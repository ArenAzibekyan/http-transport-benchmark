# http-transport-benchmark
Benchmark for HTTP-transport correct setup

### How to use
```sh
go run . -test.benchtime 1s -parallelism 100 -max_idle_conns_per_host 100 -max_conns_per_host 100
```