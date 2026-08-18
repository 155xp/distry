---
name: distry-jobs
description: Create, review, or adapt Go programs that run as distributed Distry jobs. Use when turning a calculation, search, simulation, chess analysis, seed finder, or other independent batch workload into Distry's split/run WebAssembly job format.
---

# Write Distry jobs

Create one self-contained Go program with three commands:

- `split`: print exactly one JSON `[][]string`, where each inner array is one chunk's arguments.
- `run <args...>`: compute one chunk and print exactly one result.
- `merge`: read the ordered JSON `[]string` of chunk results from stdin and print one final answer.

Start from `examples/sum.go`. Keep chunks independent and safe to retry; workers may execute a chunk more than once when a lease expires or a worker reports failure.

## Workflow

1. Identify the search space and the smallest useful result for one chunk.
2. Divide work into balanced, non-overlapping chunks. Prefer many medium chunks over a few long ones.
3. Encode chunk inputs as strings. Parse and validate them in `run`.
4. Print no logs during `split`; extra output makes its JSON invalid.
5. Print only the chunk result during `run`. Stdout and stderr are merged.
6. Combine every chunk result in `merge`; do not assume that combining always means concatenation.
7. Make `run` deterministic and idempotent. Do not depend on execution order or shared state.
8. Build and test all three commands locally before submission.

## Required shape

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "split":
		json.NewEncoder(os.Stdout).Encode([][]string{{"0", "1000"}, {"1000", "2000"}})
	case "run":
		if len(os.Args) != 4 {
			os.Exit(2)
		}
		fmt.Println(compute(os.Args[2], os.Args[3]))
	case "merge":
		var results []string
		json.NewDecoder(os.Stdin).Decode(&results)
		fmt.Println(merge(results))
	}
}
```

Replace `compute`, `merge`, and the chunk boundaries with the requested workload. Use JSON inside each result when it needs structure. Distry preserves result order before calling `merge`.

## Runtime limits

- Target `GOOS=wasip1 GOARCH=wasm` and avoid packages unsupported by WASI.
- Do not require host files, environment secrets, sockets, or network access.
- Keep the compiled module under 10 MiB.
- Return less than 1 MiB of combined stdout/stderr per chunk.
- Finish each chunk comfortably within 10 minutes and 256 MiB of memory.
- Produce no more than 10,000 chunks.
- Treat permanent errors carefully: reported failures are retried with a new lease and incremented attempt number.

## Validate

Run:

```sh
GOOS=wasip1 GOARCH=wasm go build -o job.wasm job.go
wasmtime run job.wasm split
wasmtime run job.wasm run <first-chunk-args>
printf '["result 1","result 2"]' | wasmtime run job.wasm merge
```

Confirm that `split` is valid JSON, sample chunks do not overlap unexpectedly, edge chunks are covered, repeated `run` calls return the same result, and `merge` handles empty and representative result arrays.

Submit with:

```sh
SUBMIT_KEY="..." go run . submit <coordinator-url> job.go
```
