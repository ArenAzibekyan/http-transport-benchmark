# http-transport-benchmark
Benchmark for HTTP-transport correct setup

### Install
```sh
go install github.com/ArenAzibekyan/http-transport-benchmark@latest
```

### How to use
```sh
http-transport-benchmark -test.benchtime 1s -parallelism 100 -max_idle_conns_per_host 100 -max_conns_per_host 100
```