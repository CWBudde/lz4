//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"
	"time"

	lz4 "github.com/cwbudde/lz4"
)

func main() {
	js.Global().Set("lz4Compress", js.FuncOf(compress))
	js.Global().Set("lz4Decompress", js.FuncOf(decompress))
	js.Global().Set("lz4Benchmark", js.FuncOf(benchmarkAll))
	select {}
}

func jsToBytes(v js.Value) []byte {
	n := v.Get("byteLength").Int()
	b := make([]byte, n)
	js.CopyBytesToGo(b, v)
	return b
}

func bytesToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

func compressBlock(src, dst []byte, level lz4.CompressionLevel) (int, error) { //nolint:unparam
	if level == lz4.Fast {
		var c lz4.Compressor
		return c.CompressBlock(src, dst)
	}
	c := lz4.CompressorHC{Level: level}
	return c.CompressBlock(src, dst)
}

// compress(data Uint8Array, level int) -> {compressed, originalSize, compressedSize, ratio, durationMs}
// The returned compressed bytes include a 4-byte LE original-size prefix so decompress is self-contained.
func compress(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return errObj("missing arguments")
	}
	data := jsToBytes(args[0])
	level := lz4.CompressionLevel(args[1].Int())
	origLen := len(data)

	bound := lz4.CompressBlockBound(origLen)
	// framed layout: [4 bytes LE orig size][compressed block]
	buf := make([]byte, 4+bound)
	buf[0] = byte(origLen)
	buf[1] = byte(origLen >> 8)
	buf[2] = byte(origLen >> 16)
	buf[3] = byte(origLen >> 24)

	start := time.Now()
	n, err := compressBlock(data, buf[4:], level)
	elapsed := time.Since(start)

	if err != nil {
		return errObj(err.Error())
	}

	ratio := 0.0
	if origLen > 0 {
		ratio = float64(n) / float64(origLen)
	}

	result := js.Global().Get("Object").New()
	result.Set("compressed", bytesToJS(buf[:4+n]))
	result.Set("originalSize", origLen)
	result.Set("compressedSize", n)
	result.Set("ratio", ratio)
	result.Set("durationMs", float64(elapsed.Nanoseconds())/1e6)
	return result
}

// decompress(data Uint8Array) -> {decompressed, originalSize, durationMs}
func decompress(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return errObj("missing arguments")
	}
	data := jsToBytes(args[0])
	if len(data) < 4 {
		return errObj("invalid data: too short")
	}

	origLen := int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
	if origLen <= 0 || origLen > 256*1024*1024 {
		return errObj("invalid data: bad size header")
	}

	dst := make([]byte, origLen)
	start := time.Now()
	n, err := lz4.UncompressBlock(data[4:], dst)
	elapsed := time.Since(start)

	if err != nil {
		return errObj("decompression failed: " + err.Error())
	}

	result := js.Global().Get("Object").New()
	result.Set("decompressed", bytesToJS(dst[:n]))
	result.Set("originalSize", n)
	result.Set("durationMs", float64(elapsed.Nanoseconds())/1e6)
	return result
}

type benchResult struct {
	Level          string  `json:"level"`
	Ratio          float64 `json:"ratio"`
	CompressMBps   float64 `json:"compressMBps"`
	DecompressMBps float64 `json:"decompressMBps"`
}

// benchmarkAll(data Uint8Array) -> JSON string []benchResult
func benchmarkAll(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return "[]"
	}
	data := jsToBytes(args[0])
	if len(data) == 0 {
		return "[]"
	}

	type levelDef struct {
		name  string
		level lz4.CompressionLevel
	}
	levels := []levelDef{
		{"Fast", lz4.Fast},
		{"L1", lz4.Level1},
		{"L2", lz4.Level2},
		{"L3", lz4.Level3},
		{"L4", lz4.Level4},
		{"L5", lz4.Level5},
		{"L6", lz4.Level6},
		{"L7", lz4.Level7},
		{"L8", lz4.Level8},
		{"L9", lz4.Level9},
	}

	const (
		warmupIters    = 3
		targetDuration = 300 * time.Millisecond
	)

	bound := lz4.CompressBlockBound(len(data))
	compBuf := make([]byte, bound)
	decompBuf := make([]byte, len(data)+1024)

	results := make([]benchResult, 0, len(levels))

	for _, l := range levels {
		for i := 0; i < warmupIters; i++ {
			compressBlock(data, compBuf, l.level) //nolint:errcheck
		}

		n, err := compressBlock(data, compBuf, l.level)
		if err != nil || n == 0 {
			continue
		}
		compressed := make([]byte, n)
		copy(compressed, compBuf[:n])
		ratio := float64(n) / float64(len(data))

		compIters := 0
		compStart := time.Now()
		for time.Since(compStart) < targetDuration {
			compressBlock(data, compBuf, l.level) //nolint:errcheck
			compIters++
		}
		compElapsed := time.Since(compStart)

		decompIters := 0
		decompStart := time.Now()
		for time.Since(decompStart) < targetDuration {
			lz4.UncompressBlock(compressed, decompBuf) //nolint:errcheck
			decompIters++
		}
		decompElapsed := time.Since(decompStart)

		compMBps := float64(compIters) * float64(len(data)) / compElapsed.Seconds() / 1e6
		decompMBps := float64(decompIters) * float64(len(data)) / decompElapsed.Seconds() / 1e6

		results = append(results, benchResult{
			Level:          l.name,
			Ratio:          ratio,
			CompressMBps:   compMBps,
			DecompressMBps: decompMBps,
		})
	}

	j, _ := json.Marshal(results)
	return string(j)
}

func errObj(msg string) js.Value {
	o := js.Global().Get("Object").New()
	o.Set("error", msg)
	return o
}
