package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type counter struct {
	reused    uint64
	notReused uint64
	wasIdle   uint64
	putIdle   uint64
	errors    map[string]uint64
	ch        chan error
	wg        *sync.WaitGroup
}

func newCounter() *counter {
	cnt := &counter{
		errors: make(map[string]uint64),
		ch:     make(chan error),
		wg:     &sync.WaitGroup{},
	}

	cnt.wg.Add(1)
	go func() {
		defer cnt.wg.Done()

		cnt.gatherErrors()
	}()

	return cnt
}

func (cnt *counter) gatherErrors() {
	for err := range cnt.ch {
		cnt.errors[err.Error()]++
	}
}

func (cnt *counter) close() {
	close(cnt.ch)
	cnt.wg.Wait()
}

func (cnt *counter) addError(err error) {
	cnt.ch <- err
}

func (cnt *counter) gotConn(info httptrace.GotConnInfo) {
	if info.Reused {
		atomic.AddUint64(&cnt.reused, 1)
	} else {
		atomic.AddUint64(&cnt.notReused, 1)
	}

	if info.WasIdle {
		atomic.AddUint64(&cnt.wasIdle, 1)
	}
}

func (cnt *counter) putIdleConn(err error) {
	if err != nil {
		cnt.addError(err)
		return
	}

	atomic.AddUint64(&cnt.putIdle, 1)
}

// go run . -test.benchtime 1s -parallelism 100 -max_idle_conns_per_host 100 -max_conns_per_host 100
func main() {
	testing.Init()

	var parallelism int
	flag.IntVar(&parallelism, "parallelism", 1, "bench parallelism factor")

	var maxIdleConnsPerHost int
	flag.IntVar(&maxIdleConnsPerHost, "max_idle_conns_per_host", 100, "max idle conns per host")

	var maxConnsPerHost int
	flag.IntVar(&maxConnsPerHost, "max_conns_per_host", 100, "max conns per host")

	flag.Parse()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/ping", func(gctx *gin.Context) {
		gctx.String(http.StatusOK, "pong")
	})

	srv := httptest.NewServer(router.Handler())
	defer srv.Close()

	cli := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			MaxConnsPerHost:       maxConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	defer cli.CloseIdleConnections()

	cnt := newCounter()
	trace := &httptrace.ClientTrace{
		GotConn:     cnt.gotConn,
		PutIdleConn: cnt.putIdleConn,
	}

	res := testing.Benchmark(func(b *testing.B) {
		b.SetParallelism(parallelism)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx := httptrace.WithClientTrace(context.Background(), trace)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/ping", nil)
				if err != nil {
					cnt.addError(err)
					continue
				}

				resp, err := cli.Do(req)
				if err != nil {
					cnt.addError(err)
					continue
				}

				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		})
	})

	cnt.close()

	fmt.Printf("%d %d ns/op %s\n", res.N, res.NsPerOp(), res.MemString())
	fmt.Printf("not reused: %d\n", cnt.notReused)
	fmt.Printf("reused: %d\n", cnt.reused)
	fmt.Printf("was idle: %d\n", cnt.wasIdle)
	fmt.Printf("put idle: %d\n", cnt.putIdle)
	fmt.Printf("errors:\n")
	for k, v := range cnt.errors {
		fmt.Printf("\t%d times: %s\n", v, k)
	}
}
