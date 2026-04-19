# Task 1 Benchmark Note

## Pre-change baseline

Command:

```bash
go test -run '^$' -bench . -benchmem
```

Status:

- Failed in `BenchmarkWriterReset` with `panic: open testdata/gettysburg.txt: no such file or directory`.
- The old `BenchmarkUncompress` was not a valid raw-block benchmark because it passed framed `pg1661.txt.lz4` data to `lz4block.UncompressBlock`.

Partial results captured before the panic:

- `BenchmarkUncompress-12`: `4.741 ns/op` (invalid benchmark)
- `BenchmarkUncompressPg1661-12`: `252546 ns/op`
- `BenchmarkCompressPg1661-12`: `18651 ns/op`

Command:

```bash
go test -run '^$' -bench . -benchmem -tags noasm
```

Status:

- Failed in the same `BenchmarkWriterReset` path.
- The old `BenchmarkUncompress-12` remained invalid here too.

Partial results captured before the panic:

- `BenchmarkUncompress-12`: `139.7 ns/op` (invalid benchmark)
- `BenchmarkUncompressPg1661-12`: `527959 ns/op`
- `BenchmarkCompressPg1661-12`: `20380 ns/op`

## Post-change verification

Command:

```bash
go test -run '^$' -bench . -benchmem -count=1 -timeout 120s
```

Status:

- Passed.

Selected results:

- `BenchmarkBlockDecompress-12`: `159068 ns/op`
- `BenchmarkFrameDecompress/Pg1661ChecksumOn-12`: `365723 ns/op`
- `BenchmarkFrameDecompress/Pg1661ChecksumOff-12`: `188143 ns/op`
- `BenchmarkFrameCompress/Pg1661Concurrent-12`: `1377165 ns/op`
- `BenchmarkWriterReset-12`: `7234 ns/op`

Command:

```bash
go test -run '^$' -bench . -benchmem -count=1 -timeout 120s -tags noasm
```

Status:

- Passed.

Selected results:

- `BenchmarkBlockDecompress-12`: `5787495 ns/op`
- `BenchmarkFrameDecompress/Pg1661ChecksumOn-12`: `6185727 ns/op`
- `BenchmarkFrameCompress/Pg1661Concurrent-12`: `1386778 ns/op`
- `BenchmarkWriterReset-12`: `3483 ns/op`
