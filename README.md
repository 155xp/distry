# distry

A simple Go distributed computing network.

`distry` runs independent Go calculations across volunteer workers. Jobs compile to WebAssembly, workers only make outbound requests, and Wasmtime runs jobs without access to host files or the network.

## Job format

A job is a Go program with two commands:

- `split` prints a JSON array of argument arrays.
- `run <args...>` computes one chunk and prints its result.

See `examples/sum.go`.

## Run locally

Install Go and [Wasmtime](https://wasmtime.dev/), then:

```sh
export SUBMIT_KEY="choose-a-private-key"
go run . coordinator
```

In another terminal:

```sh
go run . worker http://localhost:8080
```

Submit and wait for the results:

```sh
SUBMIT_KEY="choose-a-private-key" go run . submit http://localhost:8080 examples/sum.go
```

## Railway

Deploy the included Dockerfile, set a private `SUBMIT_KEY`, and give workers the Railway HTTPS URL. Do not commit that key. Workers need no open ports and do not expose your home IP through this repository.

This is an early prototype: coordinator state is in memory, results are public to anyone with the job ID, and only trusted people should receive the submit key.
