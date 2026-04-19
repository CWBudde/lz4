package lz4_test

import (
	"bytes"
	"testing"

	"github.com/cwbudde/lz4"
)

var testPayload = bytes.Repeat([]byte("hello lz4 world! "), 100)

func TestCompressBlockFreeFunc(t *testing.T) {
	dst := make([]byte, lz4.CompressBlockBound(len(testPayload)))
	n, err := lz4.CompressBlock(testPayload, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected non-zero compressed size")
	}
	out := make([]byte, len(testPayload))
	m, err := lz4.UncompressBlock(dst[:n], out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[:m], testPayload) {
		t.Fatal("round-trip mismatch")
	}
}

func TestUncompressBlockWithDict(t *testing.T) {
	dict := bytes.Repeat([]byte("dictionary prefix "), 10)
	payload := append(dict, testPayload...)

	var c lz4.Compressor
	dst := make([]byte, lz4.CompressBlockBound(len(payload)))
	n, err := c.CompressBlock(payload, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected non-zero compressed size")
	}

	out := make([]byte, len(payload))
	m, err := lz4.UncompressBlockWithDict(dst[:n], out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[:m], payload) {
		t.Fatal("round-trip mismatch without dict")
	}
}

func TestCompressorHCCompressBlock(t *testing.T) {
	var c lz4.CompressorHC
	c.Level = lz4.Level5

	dst := make([]byte, lz4.CompressBlockBound(len(testPayload)))
	n, err := c.CompressBlock(testPayload, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected non-zero compressed size")
	}
	out := make([]byte, len(testPayload))
	m, err := lz4.UncompressBlock(dst[:n], out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[:m], testPayload) {
		t.Fatal("round-trip mismatch")
	}
}

func TestCompressBlockHCFreeFunc(t *testing.T) {
	dst := make([]byte, lz4.CompressBlockBound(len(testPayload)))
	n, err := lz4.CompressBlockHC(testPayload, dst, lz4.Level3, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected non-zero compressed size")
	}
	out := make([]byte, len(testPayload))
	m, err := lz4.UncompressBlock(dst[:n], out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[:m], testPayload) {
		t.Fatal("round-trip mismatch")
	}
}
