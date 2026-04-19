package lz4_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/pierrec/lz4/v4"
)

func TestSizeOptionWriter(t *testing.T) {
	data := bytes.Repeat([]byte("size option test "), 50)
	out := new(bytes.Buffer)
	zw := lz4.NewWriter(out)
	if err := zw.Apply(lz4.SizeOption(uint64(len(data)))); err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zr := lz4.NewReader(out)
	result, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("round-trip mismatch with SizeOption")
	}
}

func TestSizeOptionCompressingReader(t *testing.T) {
	data := bytes.Repeat([]byte("compressing reader size "), 50)
	cr := lz4.NewCompressingReader(io.NopCloser(bytes.NewReader(data)))
	if err := cr.Apply(lz4.SizeOption(uint64(len(data)))); err != nil {
		t.Fatal(err)
	}
	compressed, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(lz4.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("round-trip mismatch")
	}
}

func TestSizeOptionNotApplicable(t *testing.T) {
	zr := lz4.NewReader(bytes.NewReader(nil))
	err := zr.Apply(lz4.SizeOption(42))
	if !errors.Is(err, lz4.ErrOptionNotApplicable) {
		t.Fatalf("expected ErrOptionNotApplicable, got %v", err)
	}
}

func TestLegacyOptionNotApplicable(t *testing.T) {
	zr := lz4.NewReader(bytes.NewReader(nil))
	err := zr.Apply(lz4.LegacyOption(true))
	if !errors.Is(err, lz4.ErrOptionNotApplicable) {
		t.Fatalf("expected ErrOptionNotApplicable, got %v", err)
	}
}

func TestCompressionLevelOptionInvalid(t *testing.T) {
	zw := lz4.NewWriter(new(bytes.Buffer))
	err := zw.Apply(lz4.CompressionLevelOption(lz4.CompressionLevel(99999)))
	if !errors.Is(err, lz4.ErrOptionInvalidCompressionLevel) {
		t.Fatalf("expected ErrOptionInvalidCompressionLevel, got %v", err)
	}
}

func TestCompressionLevelOptionCompressingReaderInvalid(t *testing.T) {
	cr := lz4.NewCompressingReader(io.NopCloser(bytes.NewReader(nil)))
	err := cr.Apply(lz4.CompressionLevelOption(lz4.CompressionLevel(99999)))
	if !errors.Is(err, lz4.ErrOptionInvalidCompressionLevel) {
		t.Fatalf("expected ErrOptionInvalidCompressionLevel, got %v", err)
	}
}

func TestBlockSizeOptionInvalidSize(t *testing.T) {
	zw := lz4.NewWriter(new(bytes.Buffer))
	err := zw.Apply(lz4.BlockSizeOption(lz4.BlockSize(12345)))
	if !errors.Is(err, lz4.ErrOptionInvalidBlockSize) {
		t.Fatalf("expected ErrOptionInvalidBlockSize, got %v", err)
	}
}

func TestBlockSizeOptionCompressingReader(t *testing.T) {
	data := bytes.Repeat([]byte("block size option "), 50)
	cr := lz4.NewCompressingReader(io.NopCloser(bytes.NewReader(data)))
	if err := cr.Apply(lz4.BlockSizeOption(lz4.Block64Kb)); err != nil {
		t.Fatal(err)
	}
	compressed, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(lz4.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("round-trip mismatch")
	}
}

func TestBlockSizeOptionInvalidCompressingReader(t *testing.T) {
	cr := lz4.NewCompressingReader(io.NopCloser(bytes.NewReader(nil)))
	err := cr.Apply(lz4.BlockSizeOption(lz4.BlockSize(12345)))
	if !errors.Is(err, lz4.ErrOptionInvalidBlockSize) {
		t.Fatalf("expected ErrOptionInvalidBlockSize, got %v", err)
	}
}

func TestChecksumOptionCompressingReader(t *testing.T) {
	data := bytes.Repeat([]byte("checksum option test "), 50)
	cr := lz4.NewCompressingReader(io.NopCloser(bytes.NewReader(data)))
	if err := cr.Apply(lz4.ChecksumOption(false)); err != nil {
		t.Fatal(err)
	}
	compressed, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(lz4.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("round-trip mismatch")
	}
}

func TestOptionString(t *testing.T) {
	tests := []struct {
		opt  lz4.Option
		want string
	}{
		{lz4.BlockSizeOption(lz4.Block4Mb), "BlockSizeOption(Block4Mb)"},
		{lz4.ChecksumOption(true), "ChecksumOption(true)"},
		{lz4.SizeOption(0), "SizeOption(0)"},
		{lz4.LegacyOption(false), "LegacyOption(false)"},
	}
	for _, tt := range tests {
		if got := tt.opt.String(); got != tt.want {
			t.Errorf("Option.String() = %q; want %q", got, tt.want)
		}
	}
}
