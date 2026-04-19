# LZ4 Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Increase real-world compression and decompression throughput in this repo without breaking the LZ4 block/frame format or correctness guarantees.

**Architecture:** Treat performance work as three separate layers: benchmark fidelity, core block codec speed, and stream/frame overhead. Fix the measurement harness first, then optimize the fast block compressor in Go, then add targeted `asm` only where profiles show stable wins, especially for checksum-heavy frame workloads.

**Tech Stack:** Go, Go testing/benchmarking, `pprof`, Plan 9 assembly, existing `internal/lz4block`, `internal/lz4stream`, and `internal/xxh32` packages.

---

## Bottleneck Analysis

### What was measured

- `go test -run '^$' -bench 'Benchmark(Compress|Uncompress)' -benchmem`
- `go test ./internal/lz4block -run '^$' -bench '.' -benchmem`
- `go test -run '^$' -bench '^BenchmarkCompressPg1661$' -cpuprofile /tmp/lz4-compress.prof`
- `go test -run '^$' -bench '^BenchmarkUncompressPg1661$' -cpuprofile /tmp/lz4-uncompress.prof`
- `go tool pprof -top /tmp/lz4-compress.prof`
- `go tool pprof -top /tmp/lz4-uncompress.prof`
- `go test -tags noasm -run '^$' -bench '^BenchmarkUncompressPg1661$' -benchmem`

### Current findings

1. The fast block compressor is the main Go-side hotspot.
   - `BenchmarkCompress`: about `5.36 ms/op` on `pg1661`.
   - Block-only CPU profile is almost entirely in `internal/lz4block.(*Compressor).CompressBlock`.
   - Within that profile:
     - `(*Compressor).get`: about `26%`
     - `(*Compressor).put`: about `10%`
     - `binary.LittleEndian.Uint32/Uint64`: about `12%`
     - `blockHash`: about `5%`
   - Interpretation: the compression hot loop is dominated by hash-table probing, position bookkeeping, and repeated little-endian loads. That is where most Go-side wins are likely to come from.

2. Existing `amd64` decoder assembly is already valuable.
   - `BenchmarkUncompressPg1661`: about `291,679 ns/op`
   - `BenchmarkUncompressPg1661` with `-tags noasm`: about `856,407 ns/op`
   - Interpretation: current decode assembly is worth roughly a `2.9x` speedup on this stream workload. Decoder `asm` is already paying for itself.

3. Frame checksums are a major secondary cost on stream decompression.
   - Stream decode profile:
     - `internal/lz4block.decodeBlock`: about `59%`
     - `internal/xxh32.updateGo`: about `27.5%`
     - `runtime.memmove`: about `10.6%`
   - Interpretation: once decode is assembly-backed, checksum work becomes one of the next largest frame-level costs, especially on `amd64` where `xxh32` is still Go code.

4. Some existing benchmarks are not trustworthy enough to guide optimization work.
   - `bench_test.go:61-69` calls `lz4block.UncompressBlock(pg1661LZ4, buf, nil)`, but `testdata/pg1661.txt.lz4` starts with `0x184D2204`, which is the frame magic, not a raw block.
   - `bench_test.go:143-147` resets the `Writer` but does not reset the underlying `bytes.Buffer`, so stream-compress benchmarks are polluted by buffer growth and append behavior.
   - Interpretation: measurement cleanup is the first task, otherwise later wins will be hard to trust.

5. Stream concurrency likely has overhead worth revisiting, but it is not the first thing to optimize.
   - `internal/lz4stream/block.go:21-57` and `:95-180` create goroutines and per-block channels in the concurrent path.
   - This is likely fine for large blocks and high latency I/O, but it adds scheduling and allocation pressure for CPU-bound microbenchmarks.

## Brainstorming: Where Speedups Are Most Likely

### High-confidence wins

- Fix the benchmark harness first so the numbers reflect actual work.
- Add `amd64` and `arm64` `xxh32` assembly. The profile already shows checksum cost is material on stream decode.
- Tune the fast compressor hot loop in Go before attempting compressor assembly.
- Add explicit benchmarks for checksum-enabled vs checksum-disabled frame workloads.

### Plausible wins

- Rework the fast compressor hash-table layout to reduce `get`/`put` overhead.
- Use lower-overhead loads and fewer repeated bounds checks in `internal/lz4block/block.go`.
- Add an earlier incompressible-data bailout to avoid spending too much time hashing data that will be emitted raw anyway.
- Replace per-block goroutine/channel orchestration with a worker pool for concurrent frame encode/decode.

### Low-confidence or high-risk ideas

- Write Plan 9 assembly for the fast compressor.
- Further tune decoder assembly thresholds in `internal/lz4block/decode_amd64.s`.
- Use wider SIMD-heavy copy/match logic in the encoder.

These are real options, but they should come after the measurement cleanup and Go-level compressor work. The encoder profile says the current bottleneck is mostly table/index logic, not something obviously fixed by hand-written copy loops alone.

## Recommended Optimization Order

1. Repair and expand the benchmark/profiling harness.
2. Optimize the fast block compressor in pure Go.
3. Add missing checksum assembly for `amd64` and possibly `arm64`.
4. Re-measure frame encode/decode with checksums on and off.
5. Revisit stream concurrency overhead.
6. Only then decide whether encoder `asm` is justified.

## Where Plan 9 Assembly Can Help

### Good assembly candidates

- `internal/xxh32`
  - There is ARM assembly already, but no `amd64` or `arm64` fast path.
  - This is a clean, self-contained target with stable semantics and a clear profile signal.
  - Expected impact: medium to high for checksum-enabled frame workloads.

- `internal/lz4block/decode_amd64.s`
  - Decoder assembly already exists and is effective.
  - The `TODO` threshold comments around `decode_amd64.s:365` and `:412` suggest some tuning work is still unfinished.
  - Expected impact: low to medium unless new measurements show a specific missed fast path.

### Poor early assembly candidates

- `internal/lz4block/block.go` fast compressor
  - The hot costs are mostly table lookups, hashing, and branchy match search logic.
  - Assembly here would be harder to maintain, harder to fuzz, and harder to port than an `xxh32` port.
  - Recommendation: optimize the Go implementation first and only consider `asm` if the post-Go profile still points at a tiny stable inner loop.

## Other Tricks Worth Trying

- Table-layout experiments
  - Replace the split `table` plus `inUse` layout with a single entry format if that reduces lookup overhead.
  - Try storing absolute or tagged positions instead of reconstructing from 16-bit values plus a bitmap.
  - Guard this with benchmarks because higher memory traffic could erase the win.

- Load-path experiments
  - Reduce repeated `binary.LittleEndian.Uint32/Uint64` calls in the hot loop.
  - Evaluate `unsafe` loads only if they produce a clear win and keep the code easy to reason about.

- Incompressible fast path
  - Add a cheap sampling heuristic so obviously incompressible blocks bail out earlier.
  - This is most relevant when the frame layer can emit raw blocks directly.

- Stream write/read overhead
  - Collapse small writes in frame encode if write aggregation reduces syscall or buffer churn.
  - Replace per-block goroutines with a bounded worker pool and reusable job/result structs.

- Benchmark matrix
  - Add separate benchmarks for:
    - raw block compress
    - raw block decompress
    - frame compress with checksum on
    - frame compress with checksum off
    - frame decompress with checksum on
    - frame decompress with checksum off
    - concurrent frame encode/decode
    - `noasm` comparisons for decode and checksums

## Task 1: Fix Measurement Fidelity

**Files:**
- Modify: `bench_test.go`
- Create: `internal/lz4block/bench_test.go` or similar block-only benchmark file if needed

- [x] Replace `BenchmarkUncompress` so it benchmarks a real raw block, not a framed `.lz4` test file.
- [x] Reset the underlying `bytes.Buffer` in `benchmarkCompress` before each iteration.
- [x] Add checksum-on and checksum-off stream benchmarks for both reader and writer paths.
- [x] Add a benchmark that exercises concurrent frame compression and decompression with `ConcurrencyOption(runtime.GOMAXPROCS(0))`.
- [x] Add benchmark names that clearly distinguish block-vs-frame and checksum-vs-no-checksum paths.
- [x] Capture baseline results with:
  - `go test -run '^$' -bench . -benchmem`
  - `go test -run '^$' -bench . -benchmem -tags noasm`
- [x] Save the baseline numbers in the commit message or a short benchmark note before changing hot code.

**Why this task comes first:** the repo currently has at least one invalid decode benchmark and one skewed stream-compress benchmark. Optimization work without fixing those will create false positives.

## Task 2: Optimize the Fast Go Compressor

**Files:**
- Modify: `internal/lz4block/block.go`
- Test: `internal/lz4block/block_test.go`
- Benchmark: `bench_test.go`, `internal/lz4block/bench_test.go`

- [x] Profile the repaired block compressor benchmark with `pprof` and confirm the hot symbols still match the current profile.
- [x] Experiment with simplifying `get`/`put` overhead:
  - inline more aggressively if the compiler misses something
  - reduce repeated mask/div/mod work
  - try alternate entry layouts
- [x] Benchmark whether a single-entry layout beats the current `table` plus `inUse` bitmap.
- [x] Evaluate reducing little-endian load overhead in the match scan.
- [x] Test whether a cheaper hash or a different `hashLog` improves throughput without unacceptable ratio loss.
- [x] Add an early incompressible-block bailout heuristic and measure it on `random.data` plus mixed corpora.
- [x] Keep output-format compatibility exact and verify with block round-trip tests and fuzz inputs.

**Success criteria:**
- At least one repaired block-compress benchmark improves measurably.
- No regressions in `TestCompressUncompressBlock`, fuzz-style corpora, or ratio on the main text fixtures.

## Task 3: Add Missing Checksum Assembly

**Files:**
- Modify/Create: `internal/xxh32/xxh32zero_amd64.s`
- Modify/Create: `internal/xxh32/xxh32zero_amd64.go`
- Consider: `internal/xxh32/xxh32zero_arm64.s`, `internal/xxh32/xxh32zero_arm64.go`
- Test: `internal/xxh32/xxh32zero_test.go`

- [ ] Implement `ChecksumZero` and `update` for `amd64` in Plan 9 assembly.
- [ ] Keep the Go fallback untouched for correctness and portability.
- [ ] Add focused benchmarks for `ChecksumZero` and streaming `update`.
- [ ] Re-run frame decode benchmarks with checksums enabled and disabled to isolate the win.
- [ ] If the `amd64` path is successful, decide whether an `arm64` port is worth doing immediately.

**Why this is a good `asm` target:** it is small, isolated, already partially assembly-backed on ARM, and the stream profile already proves it matters.

## Task 4: Revisit Stream Concurrency

**Files:**
- Modify: `internal/lz4stream/block.go`
- Modify: `writer.go`
- Modify: `reader.go`
- Benchmark: `bench_test.go`

- [ ] Measure concurrent encode/decode with large blocks and multiple cores after Tasks 1-3.
- [ ] Replace per-block goroutine creation with a reusable worker pool if scheduler overhead shows up.
- [ ] Replace `chan chan *FrameDataBlock` and `chan chan []byte` orchestration with cheaper job/result queues if profiles justify it.
- [ ] Preserve ordered output and first-error semantics.
- [ ] Re-check memory retention and buffer-pool behavior after the refactor.

**Success criteria:**
- Better throughput at `ConcurrencyOption(n>1)` without increasing allocations or regressing single-threaded performance.

## Task 5: Decide Whether More Assembly Is Worth It

**Files:**
- Inspect: `internal/lz4block/block.go`
- Inspect: `internal/lz4block/decode_amd64.s`

- [ ] Re-profile after Tasks 1-4.
- [ ] If decoder assembly is no longer the bottleneck, leave it alone except for obvious threshold cleanups.
- [ ] If compressor hot time is still concentrated in a tiny stable loop, prototype a narrow `amd64` assembly helper rather than rewriting the whole encoder.
- [ ] Reject broad encoder assembly if the win is small or the maintenance cost is too high.

**Decision rule:** do not write large compressor assembly until the repaired benchmarks and post-Go profiles prove that Go-level structural changes are exhausted.

## Verification

- [ ] Run:
  - `go test ./...`
  - `go test -run '^$' -bench . -benchmem`
  - `go test -run '^$' -bench . -benchmem -tags noasm`
- [ ] Compare compression ratio and throughput before vs after for:
  - `pg1661`
  - `Mark.Twain-Tom.Sawyer`
  - `e.txt`
  - `random.data`
- [ ] Verify checksum-enabled frame decode still validates corrupted data correctly.
- [ ] Verify block and frame fuzz/regression tests still pass.

## Exit Criteria

- The benchmark suite measures the real codec paths correctly.
- Fast block compression is measurably faster on representative corpora.
- Checksum-enabled frame decode is faster on `amd64`.
- Any new assembly has tests, `noasm` fallbacks, and clear benchmark justification.
- The repo ends up with a repeatable measurement workflow, not just a one-off speed hack.
