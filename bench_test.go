package lz4_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/pierrec/lz4/v4"
	"github.com/pierrec/lz4/v4/internal/lz4block"
)

func mustLoadFile(f string) []byte {
	var b []byte
	var err error
	if strings.HasSuffix(f, ".gz") {
		b, err = loadGoldenGz(f)
	} else {
		b, err = os.ReadFile(f)
	}
	if err != nil {
		panic(err)
	}
	return b
}

func mustCompressBlock(src []byte) []byte {
	buf := make([]byte, lz4.CompressBlockBound(len(src)))
	var c lz4.Compressor
	n, err := c.CompressBlock(src, buf)
	if err != nil {
		panic(err)
	}
	if n == 0 {
		panic("benchmark fixture is not block-compressible")
	}
	return buf[:n]
}

func mustCompressFrame(src []byte, options ...lz4.Option) []byte {
	var buf bytes.Buffer
	zw := lz4.NewWriter(&buf)
	if err := zw.Apply(options...); err != nil {
		panic(err)
	}
	if _, err := io.Copy(zw, bytes.NewReader(src)); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

var (
	pg1661     = mustLoadFile("testdata/pg1661.txt.gz")
	digits     = mustLoadFile("testdata/e.txt.gz")
	twain      = mustLoadFile("testdata/Mark.Twain-Tom.Sawyer.txt.gz")
	random     = mustLoadFile("testdata/random.data.gz")
	gettysburg = mustLoadFile("testdata/gettysburg.txt.gz")

	pg1661LZ4         = mustLoadFile("testdata/pg1661.txt.lz4")
	digitsLZ4         = mustLoadFile("testdata/e.txt.lz4")
	twainLZ4          = mustLoadFile("testdata/Mark.Twain-Tom.Sawyer.txt.lz4")
	randomLZ4         = mustLoadFile("testdata/random.data.lz4")
	randomAppendedLZ4 = mustLoadFile("testdata/random_appended.data.lz4")

	pg1661Block = mustCompressBlock(pg1661)

	pg1661FrameChecksumOn  = mustCompressFrame(pg1661)
	pg1661FrameChecksumOff = mustCompressFrame(pg1661, lz4.ChecksumOption(false))
	pg1661FrameConcurrent  = mustCompressFrame(pg1661, lz4.BlockSizeOption(lz4.Block64Kb))
	digitsFrameChecksumOn  = mustCompressFrame(digits)
	twainFrameChecksumOn   = mustCompressFrame(twain)
	randomFrameChecksumOn  = mustCompressFrame(random)
)

func BenchmarkBlockCompress(b *testing.B) {
	b.Run("Pg1661Fast", func(b *testing.B) {
		buf := make([]byte, len(pg1661))
		var c lz4.Compressor

		n, err := c.CompressBlock(pg1661, buf)
		if err != nil {
			b.Fatal(err)
		}

		b.SetBytes(int64(len(pg1661)))
		b.ReportAllocs()
		b.ReportMetric(float64(n), "outbytes")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if _, err := c.CompressBlock(pg1661, buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RandomFast", func(b *testing.B) {
		buf := make([]byte, lz4.CompressBlockBound(len(random)))
		var c lz4.Compressor

		n, err := c.CompressBlock(random, buf)
		if err != nil {
			b.Fatal(err)
		}

		b.SetBytes(int64(len(random)))
		b.ReportAllocs()
		b.ReportMetric(float64(n), "outbytes")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if _, err := c.CompressBlock(random, buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pg1661HC", func(b *testing.B) {
		buf := make([]byte, len(pg1661))
		c := lz4.CompressorHC{Level: 16}

		n, err := c.CompressBlock(pg1661, buf)
		if err != nil {
			b.Fatal(err)
		}

		b.SetBytes(int64(len(pg1661)))
		b.ReportAllocs()
		b.ReportMetric(float64(n), "outbytes")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if _, err := c.CompressBlock(pg1661, buf); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkBlockDecompress(b *testing.B) {
	buf := make([]byte, len(pg1661))

	b.SetBytes(int64(len(pg1661Block)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := lz4block.UncompressBlock(pg1661Block, buf, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkFrameDecompress(b *testing.B, compressed []byte, options ...lz4.Option) {
	r := bytes.NewReader(compressed)
	zr := lz4.NewReader(r)
	if err := zr.Apply(options...); err != nil {
		b.Fatal(err)
	}

	_, err := io.Copy(io.Discard, zr)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(compressed)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(compressed)
		zr.Reset(r)
		if _, err := io.Copy(io.Discard, zr); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFrameDecompress(b *testing.B) {
	b.Run("Pg1661ChecksumOn", func(b *testing.B) {
		benchmarkFrameDecompress(b, pg1661FrameChecksumOn)
	})
	b.Run("Pg1661ChecksumOff", func(b *testing.B) {
		benchmarkFrameDecompress(b, pg1661FrameChecksumOff)
	})
	b.Run("Pg1661Concurrent", func(b *testing.B) {
		benchmarkFrameDecompress(b, pg1661FrameConcurrent, lz4.ConcurrencyOption(runtime.GOMAXPROCS(0)))
	})
	b.Run("DigitsChecksumOn", func(b *testing.B) {
		benchmarkFrameDecompress(b, digitsFrameChecksumOn)
	})
	b.Run("TwainChecksumOn", func(b *testing.B) {
		benchmarkFrameDecompress(b, twainFrameChecksumOn)
	})
	b.Run("RandChecksumOn", func(b *testing.B) {
		benchmarkFrameDecompress(b, randomFrameChecksumOn)
	})
}

func benchmarkFrameCompress(b *testing.B, uncompressed []byte, options ...lz4.Option) {
	var w bytes.Buffer
	zw := lz4.NewWriter(&w)
	if err := zw.Apply(options...); err != nil {
		b.Fatal(err)
	}
	r := bytes.NewReader(uncompressed)

	if _, err := io.Copy(zw, r); err != nil {
		b.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(uncompressed)))
	b.ReportAllocs()
	b.ReportMetric(float64(w.Len()), "outbytes")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Reset()
		r.Reset(uncompressed)
		zw.Reset(&w)
		if _, err := io.Copy(zw, r); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkFrameCompressFreshWriter(b *testing.B, uncompressed []byte, outbytes int, options ...lz4.Option) {
	b.SetBytes(int64(len(uncompressed)))
	b.ReportAllocs()
	b.ReportMetric(float64(outbytes), "outbytes")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var w bytes.Buffer
		zw := lz4.NewWriter(&w)
		if err := zw.Apply(options...); err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(zw, bytes.NewReader(uncompressed)); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFrameCompress(b *testing.B) {
	b.Run("Pg1661ChecksumOn", func(b *testing.B) {
		benchmarkFrameCompress(b, pg1661)
	})
	b.Run("Pg1661ChecksumOff", func(b *testing.B) {
		benchmarkFrameCompress(b, pg1661, lz4.ChecksumOption(false))
	})
	b.Run("Pg1661Concurrent", func(b *testing.B) {
		benchmarkFrameCompressFreshWriter(
			b,
			pg1661,
			len(pg1661FrameConcurrent),
			lz4.BlockSizeOption(lz4.Block64Kb),
			lz4.ConcurrencyOption(runtime.GOMAXPROCS(0)),
		)
	})
	b.Run("DigitsChecksumOn", func(b *testing.B) {
		benchmarkFrameCompress(b, digits)
	})
	b.Run("TwainChecksumOn", func(b *testing.B) {
		benchmarkFrameCompress(b, twain)
	})
	b.Run("RandChecksumOn", func(b *testing.B) {
		benchmarkFrameCompress(b, random)
	})
}

// Benchmark to check reallocations upon Reset().
// See issue https://github.com/pierrec/lz4/issues/52.
func BenchmarkWriterReset(b *testing.B) {
	b.ReportAllocs()

	zw := lz4.NewWriter(nil)
	var buf bytes.Buffer

	for n := 0; n < b.N; n++ {
		buf.Reset()
		zw.Reset(&buf)

		_, _ = zw.Write(gettysburg)
		_ = zw.Close()
	}
}

// Golden files are compressed with gzip.
func loadGolden(t *testing.T, fname string) []byte {
	fname = strings.Replace(fname, ".lz4", ".gz", 1)
	t.Helper()
	b, err := loadGoldenGz(fname)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadGoldenGz(fname string) ([]byte, error) {
	file, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, gzr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
