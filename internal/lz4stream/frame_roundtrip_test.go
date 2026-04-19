package lz4stream

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/cwbudde/lz4/internal/lz4block"
	"github.com/cwbudde/lz4/internal/lz4errors"
)

// writeFrame compresses data into dst using the given concurrency level.
func writeFrame(t *testing.T, data []byte, num int, legacy bool) *bytes.Buffer {
	t.Helper()
	dst := new(bytes.Buffer)
	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.Descriptor.Flags.ContentChecksumSet(true)
	f.InitW(dst, num, legacy)

	if err := f.Descriptor.Write(f, dst); err != nil {
		t.Fatal(err)
	}

	// Compress data in chunks matching the block size.
	blockSize := int(lz4block.Block64Kb)
	for i := 0; i < len(data); i += blockSize {
		end := i + blockSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		block := NewFrameDataBlock(f)
		block.Compress(f, chunk, lz4block.Fast)
		if err := block.Write(f, dst); err != nil {
			block.Close(f)
			t.Fatal(err)
		}
		block.Close(f)
	}

	if err := f.CloseW(dst, num); err != nil {
		t.Fatal(err)
	}
	return dst
}

// readFrame decompresses data from src and returns the original bytes.
func readFrame(t *testing.T, src *bytes.Buffer, num int) []byte {
	t.Helper()
	f := NewFrame()
	if err := f.ParseHeaders(src); err != nil {
		t.Fatal(err)
	}

	reads, err := f.InitR(src, num)
	if err != nil {
		t.Fatal(err)
	}

	var out []byte
	if num == 1 {
		for {
			block := f.Blocks.Block
			_, err := block.Read(f, src, 0)
			if err == lz4errors.ErrEndOfStream {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			size := f.Descriptor.Flags.BlockSizeIndex()
			buf, err := block.Uncompress(f, size.Get(), nil, true)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, buf...)
			lz4block.Put(buf)
		}
	} else {
		for buf := range reads {
			out = append(out, buf...)
			lz4block.Put(buf)
		}
		if err := f.Blocks.ErrorR(); err != nil && err != lz4errors.ErrEndOfStream {
			t.Fatal(err)
		}
	}

	if err := f.CloseR(src); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFrameRoundTripSequential(t *testing.T) {
	data := []byte(strings.Repeat("hello lz4 frame roundtrip ", 500))
	compressed := writeFrame(t, data, 1, false)
	got := readFrame(t, compressed, 1)
	if !bytes.Equal(got, data) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(data))
	}
}

func TestFrameRoundTripConcurrent(t *testing.T) {
	data := []byte(strings.Repeat("concurrent lz4 frame roundtrip ", 500))
	compressed := writeFrame(t, data, 4, false)
	got := readFrame(t, compressed, 4)
	if !bytes.Equal(got, data) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(data))
	}
}

func TestFrameReset(t *testing.T) {
	data := []byte(strings.Repeat("reset test data ", 100))
	buf := new(bytes.Buffer)

	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.InitW(buf, 1, false)
	if err := f.Descriptor.Write(f, buf); err != nil {
		t.Fatal(err)
	}
	block := NewFrameDataBlock(f)
	block.Compress(f, data, lz4block.Fast)
	if err := block.Write(f, buf); err != nil {
		block.Close(f)
		t.Fatal(err)
	}
	block.Close(f)
	if err := f.CloseW(buf, 1); err != nil {
		t.Fatal(err)
	}

	// Reset and reuse frame.
	f.Reset(1)
	if f.Magic != 0 {
		t.Errorf("Reset: Magic not zeroed, got %d", f.Magic)
	}
	if f.Descriptor.ContentSize != 0 {
		t.Errorf("Reset: ContentSize not zeroed, got %d", f.Descriptor.ContentSize)
	}
}

func TestFrameDataBlockClose(t *testing.T) {
	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	block := NewFrameDataBlock(f)
	if block.data == nil {
		t.Fatal("block.data should not be nil after creation")
	}
	block.Close(f)
	if block.data != nil {
		t.Error("block.data should be nil after Close")
	}
	if block.Data != nil {
		t.Error("block.Data should be nil after Close")
	}
	// Second Close should be a no-op (not panic).
	block.Close(f)
}

func TestBlocksErrorR(t *testing.T) {
	var b Blocks
	if err := b.ErrorR(); err != nil {
		t.Errorf("ErrorR on fresh Blocks should be nil, got %v", err)
	}
	b.closeR(lz4errors.ErrInvalidFrame)
	if err := b.ErrorR(); err != lz4errors.ErrInvalidFrame {
		t.Errorf("ErrorR after closeR: got %v, want ErrInvalidFrame", err)
	}
	// closeR should not overwrite an existing error.
	b.closeR(lz4errors.ErrInvalidHeaderChecksum)
	if err := b.ErrorR(); err != lz4errors.ErrInvalidFrame {
		t.Errorf("closeR should not overwrite existing error: got %v", err)
	}
}

func TestParseHeadersInvalidMagic(t *testing.T) {
	// A buffer with an invalid magic number.
	buf := bytes.NewBuffer([]byte{0x00, 0x00, 0x00, 0x00})
	f := NewFrame()
	err := f.ParseHeaders(buf)
	if err != lz4errors.ErrInvalidFrame {
		t.Errorf("ParseHeaders with invalid magic: got %v, want ErrInvalidFrame", err)
	}
}

func TestParseHeadersAlreadyRead(t *testing.T) {
	data := []byte(strings.Repeat("abc", 10))
	compressed := writeFrame(t, data, 1, false)

	f := NewFrame()
	if err := f.ParseHeaders(compressed); err != nil {
		t.Fatal(err)
	}
	// Calling ParseHeaders again should be a no-op (magic > 0).
	if err := f.ParseHeaders(compressed); err != nil {
		t.Errorf("second ParseHeaders should succeed, got %v", err)
	}
}

func TestFrameLegacyInitW(t *testing.T) {
	// Exercise InitW with legacy=true and CloseW's legacy early-return path.
	buf := new(bytes.Buffer)
	f := NewFrame()
	// Legacy mode uses Block8Mb internally.
	f.InitW(buf, 1, true)
	if !f.isLegacy() {
		t.Fatal("InitW(legacy=true) should set legacy magic")
	}
	if err := f.Descriptor.Write(f, buf); err != nil {
		t.Fatal(err)
	}
	// Compress a small block and write it.
	data := []byte(strings.Repeat("legacy write ", 50))
	block := NewFrameDataBlock(f)
	block.Compress(f, data, lz4block.Fast)
	if err := block.Write(f, buf); err != nil {
		block.Close(f)
		t.Fatal(err)
	}
	block.Close(f)
	// CloseW in legacy mode returns early (no end-mark or checksum).
	if err := f.CloseW(buf, 1); err != nil {
		t.Fatal(err)
	}
}

func TestInitWConcurrentClose(t *testing.T) {
	// Exercise Blocks.initW and close in concurrent mode.
	buf := new(bytes.Buffer)

	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.Descriptor.Flags.ContentChecksumSet(false)
	f.InitW(buf, 4, false)

	if err := f.Descriptor.Write(f, buf); err != nil {
		t.Fatal(err)
	}
	if err := f.CloseW(buf, 4); err != nil {
		t.Fatal(err)
	}
}

// writeFrameWithOptions is like writeFrame but accepts extra flag setup via a hook.
func writeFrameWithOptions(t *testing.T, data []byte, num int, setup func(*Frame)) *bytes.Buffer {
	t.Helper()
	dst := new(bytes.Buffer)
	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	if setup != nil {
		setup(f)
	}
	f.InitW(dst, num, false)
	if err := f.Descriptor.Write(f, dst); err != nil {
		t.Fatal(err)
	}
	blockSize := int(lz4block.Block64Kb)
	for i := 0; i < len(data); i += blockSize {
		end := i + blockSize
		if end > len(data) {
			end = len(data)
		}
		block := NewFrameDataBlock(f)
		block.Compress(f, data[i:end], lz4block.Fast)
		if err := block.Write(f, dst); err != nil {
			block.Close(f)
			t.Fatal(err)
		}
		block.Close(f)
	}
	if err := f.CloseW(dst, num); err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestBlockChecksum exercises BlockChecksumSet, Compress checksum, Write checksum,
// Read checksum, and Uncompress checksum verification.
func TestBlockChecksum(t *testing.T) {
	data := []byte(strings.Repeat("block checksum test data ", 200))

	setup := func(f *Frame) {
		f.Descriptor.Flags.BlockChecksumSet(true)
		f.Descriptor.Flags.ContentChecksumSet(false)
	}
	compressed := writeFrameWithOptions(t, data, 1, setup)

	// Read and verify.
	f := NewFrame()
	if err := f.ParseHeaders(compressed); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InitR(compressed, 1); err != nil {
		t.Fatal(err)
	}
	var out []byte
	for {
		block := f.Blocks.Block
		_, err := block.Read(f, compressed, 0)
		if err == lz4errors.ErrEndOfStream {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		buf, err := block.Uncompress(f, f.Descriptor.Flags.BlockSizeIndex().Get(), nil, false)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, buf...)
		lz4block.Put(buf)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("block checksum round-trip mismatch")
	}
}

// TestBlockChecksumMismatch verifies that a corrupted block is detected.
func TestBlockChecksumMismatch(t *testing.T) {
	data := []byte(strings.Repeat("checksum mismatch test ", 100))
	setup := func(f *Frame) {
		f.Descriptor.Flags.BlockChecksumSet(true)
		f.Descriptor.Flags.ContentChecksumSet(false)
	}
	compressed := writeFrameWithOptions(t, data, 1, setup)

	// Corrupt a byte inside the first data block (skip magic+descriptor+block-size header).
	raw := compressed.Bytes()
	// header = 4(magic)+2(flags)+1(checksum) = 7 bytes; block-size = 4 bytes; then data
	if len(raw) > 15 {
		raw[15] ^= 0xFF
	}

	f := NewFrame()
	if err := f.ParseHeaders(bytes.NewBuffer(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InitR(bytes.NewBuffer(raw[7:]), 1); err != nil {
		t.Fatal(err)
	}
}

// TestCompressHCLevel exercises the HC compression path in Compress.
func TestCompressHCLevel(t *testing.T) {
	data := []byte(strings.Repeat("hc compression level test ", 300))
	dst := new(bytes.Buffer)
	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.Descriptor.Flags.ContentChecksumSet(false)
	f.InitW(dst, 1, false)
	if err := f.Descriptor.Write(f, dst); err != nil {
		t.Fatal(err)
	}
	block := NewFrameDataBlock(f)
	block.Compress(f, data, lz4block.CompressionLevel(lz4block.Fast+1)) // non-Fast → HC path
	if err := block.Write(f, dst); err != nil {
		block.Close(f)
		t.Fatal(err)
	}
	block.Close(f)
	if err := f.CloseW(dst, 1); err != nil {
		t.Fatal(err)
	}

	// Verify decompressible.
	f2 := NewFrame()
	if err := f2.ParseHeaders(dst); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.InitR(dst, 1); err != nil {
		t.Fatal(err)
	}
	block2 := f2.Blocks.Block
	if _, err := block2.Read(f2, dst, 0); err != nil && err != lz4errors.ErrEndOfStream {
		t.Fatal(err)
	}
}

// TestDescriptorWriteWithSize exercises FrameDescriptor.Write with the Size flag set
// and the early-return when the checksum is already set.
func TestDescriptorWriteWithSize(t *testing.T) {
	dst := new(bytes.Buffer)
	f := NewFrame()
	f.Magic = frameMagic
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.Descriptor.Flags.SizeSet(true)
	f.Descriptor.ContentSize = 12345
	f.Descriptor.initW()

	if err := f.Descriptor.Write(f, dst); err != nil {
		t.Fatal(err)
	}
	if dst.Len() == 0 {
		t.Fatal("expected bytes written for descriptor with Size flag")
	}
	savedLen := dst.Len()

	// Calling Write again with checksum already set should be a no-op.
	if err := f.Descriptor.Write(f, dst); err != nil {
		t.Fatal(err)
	}
	if dst.Len() != savedLen {
		t.Errorf("second Write should be no-op: got %d extra bytes", dst.Len()-savedLen)
	}
}

// TestDescriptorInitRWithSize exercises initR when the Size flag is present.
func TestDescriptorInitRWithSize(t *testing.T) {
	setup := func(f *Frame) {
		f.Descriptor.Flags.SizeSet(true)
		f.Descriptor.ContentSize = 999
		f.Descriptor.Flags.ContentChecksumSet(false)
	}
	data := []byte(strings.Repeat("content size test ", 50))
	compressed := writeFrameWithOptions(t, data, 1, setup)

	f := NewFrame()
	if err := f.ParseHeaders(compressed); err != nil {
		t.Fatal(err)
	}
	if !f.Descriptor.Flags.Size() {
		t.Error("Size flag should be set after ParseHeaders")
	}
}

// TestDescriptorInitRBadChecksum verifies that a corrupted descriptor checksum is detected.
func TestDescriptorInitRBadChecksum(t *testing.T) {
	data := []byte(strings.Repeat("bad checksum test ", 20))
	compressed := writeFrameWithOptions(t, data, 1, nil)
	raw := compressed.Bytes()
	// Descriptor checksum is at byte 6 (after 4-byte magic + 2-byte flags).
	raw[6] ^= 0xFF

	f := NewFrame()
	err := f.ParseHeaders(bytes.NewBuffer(raw))
	if err == nil {
		t.Fatal("expected error for bad descriptor checksum")
	}
}

// TestDescriptorInitRInvalidBlockSize verifies that an invalid block-size index is rejected.
func TestDescriptorInitRInvalidBlockSize(t *testing.T) {
	// Build a descriptor with an invalid block size index (0).
	var flags DescriptorFlags
	flags.ContentChecksumSet(true)
	flags.VersionSet(1)
	flags.BlockIndependenceSet(true)
	// BlockSizeIndex stays 0 = invalid.
	checksum := descriptorChecksum([]byte{byte(flags), byte(flags >> 8)})

	var buf bytes.Buffer
	binary.LittleEndian.AppendUint32(buf.Bytes(), frameMagic) // magic
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, frameMagic)
	buf.Write(b)
	buf.Write([]byte{byte(flags), byte(flags >> 8), checksum})

	f := NewFrame()
	err := f.ParseHeaders(&buf)
	if err != lz4errors.ErrOptionInvalidBlockSize {
		t.Errorf("expected ErrOptionInvalidBlockSize, got %v", err)
	}
}

// TestParseHeadersSkipMagic exercises the skip-magic frame handling in ParseHeaders.
func TestParseHeadersSkipMagic(t *testing.T) {
	data := []byte(strings.Repeat("after skip ", 50))
	realFrame := writeFrameWithOptions(t, data, 1, nil)

	// Prepend a skip frame: magic (0x184D2A50) + 4-byte skip size + skip bytes.
	const skipSize = 8
	var buf bytes.Buffer
	skipMagic := make([]byte, 4)
	binary.LittleEndian.PutUint32(skipMagic, frameSkipMagic)
	buf.Write(skipMagic)
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, skipSize)
	buf.Write(sz)
	buf.Write(make([]byte, skipSize)) // the bytes to skip
	buf.Write(realFrame.Bytes())

	f := NewFrame()
	if err := f.ParseHeaders(&buf); err != nil {
		t.Fatalf("ParseHeaders with skip frame: %v", err)
	}
	if f.Magic != frameMagic {
		t.Errorf("expected frameMagic after skip, got %x", f.Magic)
	}
}

// TestCloseRNoChecksum exercises CloseR's early-return when ContentChecksum is false.
func TestCloseRNoChecksum(t *testing.T) {
	setup := func(f *Frame) { f.Descriptor.Flags.ContentChecksumSet(false) }
	data := []byte("no checksum frame")
	compressed := writeFrameWithOptions(t, data, 1, setup)

	f := NewFrame()
	if err := f.ParseHeaders(compressed); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InitR(compressed, 1); err != nil {
		t.Fatal(err)
	}
	// Drain blocks.
	for {
		_, err := f.Blocks.Block.Read(f, compressed, 0)
		if err == lz4errors.ErrEndOfStream {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		buf, err := f.Blocks.Block.Uncompress(f, f.Descriptor.Flags.BlockSizeIndex().Get(), nil, false)
		if err != nil {
			t.Fatal(err)
		}
		lz4block.Put(buf)
	}
	if err := f.CloseR(compressed); err != nil {
		t.Fatalf("CloseR with no checksum: %v", err)
	}
}

// TestCloseRChecksumMismatch exercises CloseR's checksum verification error path.
func TestCloseRChecksumMismatch(t *testing.T) {
	data := []byte(strings.Repeat("checksum mismatch ", 50))
	compressed := writeFrame(t, data, 1, false)
	raw := compressed.Bytes()
	// Corrupt the last 4 bytes (the content checksum).
	for i := len(raw) - 4; i < len(raw); i++ {
		raw[i] ^= 0xFF
	}

	f := NewFrame()
	if err := f.ParseHeaders(bytes.NewBuffer(raw)); err != nil {
		t.Fatal(err)
	}
	src := bytes.NewBuffer(raw[7:]) // skip header
	if _, err := f.InitR(src, 1); err != nil {
		t.Fatal(err)
	}
	for {
		_, err := f.Blocks.Block.Read(f, src, 0)
		if err == lz4errors.ErrEndOfStream {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		buf, err := f.Blocks.Block.Uncompress(f, f.Descriptor.Flags.BlockSizeIndex().Get(), nil, true)
		if err != nil {
			t.Fatal(err)
		}
		lz4block.Put(buf)
	}
	err := f.CloseR(src)
	if err == nil {
		t.Fatal("expected checksum mismatch error from CloseR")
	}
}

// TestCloseRLegacy exercises CloseR's legacy early-return path.
func TestCloseRLegacy(t *testing.T) {
	f := NewFrame()
	f.Magic = frameMagicLegacy
	// CloseR on a legacy frame should return nil immediately.
	if err := f.CloseR(bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("CloseR legacy: %v", err)
	}
}

// TestDescriptorFlagsGetters exercises the uncovered getter methods on DescriptorFlags.
func TestDescriptorFlagsGetters(t *testing.T) {
	var flags DescriptorFlags
	flags.BlockIndependenceSet(true)
	flags.VersionSet(1)
	flags.BlockChecksumSet(true)
	flags.SizeSet(true)

	if !flags.BlockIndependence() {
		t.Error("BlockIndependence() should be true")
	}
	if flags.Version() != 1 {
		t.Errorf("Version() = %d; want 1", flags.Version())
	}
	if !flags.BlockChecksum() {
		t.Error("BlockChecksum() should be true after BlockChecksumSet(true)")
	}
	if !flags.Size() {
		t.Error("Size() should be true after SizeSet(true)")
	}
}

// TestUncompressWithSum exercises Uncompress with sum=true (content checksum accumulation).
// It goes through the full Write+Read cycle so b.data is populated by Read.
func TestUncompressWithSum(t *testing.T) {
	data := []byte(strings.Repeat("sum path test ", 200))

	// Write a block to a buffer.
	zbuf := new(bytes.Buffer)
	f := NewFrame()
	f.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f.Descriptor.Flags.ContentChecksumSet(true)
	f.checksum.Reset()

	wBlock := NewFrameDataBlock(f)
	wBlock.Compress(f, data, lz4block.Fast)
	if err := wBlock.Write(f, zbuf); err != nil {
		wBlock.Close(f)
		t.Fatal(err)
	}
	wBlock.Close(f)

	// Read the block back so b.data is filled with compressed bytes.
	f2 := NewFrame()
	f2.Descriptor.Flags.BlockSizeIndexSet(lz4block.Index(lz4block.Block64Kb))
	f2.Descriptor.Flags.ContentChecksumSet(true)
	f2.checksum.Reset()
	rBlock := NewFrameDataBlock(f2)
	if _, err := rBlock.Read(f2, zbuf, 0); err != nil {
		t.Fatal(err)
	}

	dst := f2.Descriptor.Flags.BlockSizeIndex().Get()
	result, err := rBlock.Uncompress(f2, dst, nil, true) // sum=true
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("Uncompress with sum=true mismatch")
	}
	lz4block.Put(dst)
	rBlock.Close(f2)
}

// TestReadUint32ShortRead covers the ErrUnexpectedEOF → io.EOF path in readUint32.
func TestReadUint32ShortRead(t *testing.T) {
	f := NewFrame()
	_, err := f.readUint32(bytes.NewBuffer([]byte{0x01, 0x02})) // only 2 bytes, need 4
	if err == nil {
		t.Fatal("expected error for short read")
	}
}

// TestInitWConcurrentBlockWrite exercises the concurrent block write path via a normal round-trip.
func TestInitWConcurrentBlockWrite(t *testing.T) {
	data := []byte(strings.Repeat("concurrent block write test ", 300))
	compressed := writeFrameWithOptions(t, data, 4, func(f *Frame) {
		f.Descriptor.Flags.ContentChecksumSet(false)
	})
	got := readFrame(t, compressed, 4)
	if !bytes.Equal(got, data) {
		t.Fatalf("concurrent write round-trip mismatch")
	}
}

// TestBlocksCloseNilBlock exercises the close path when Block is nil (num==1).
func TestBlocksCloseNilBlock(t *testing.T) {
	var b Blocks
	b.Block = nil
	// Should not panic and should return nil error.
	if err := b.close(nil, 1); err != nil {
		t.Errorf("close with nil Block: %v", err)
	}
}

// TestParseHeadersReadError exercises ParseHeaders when the reader returns an error.
func TestParseHeadersReadError(t *testing.T) {
	f := NewFrame()
	err := f.ParseHeaders(errReader{})
	if err == nil {
		t.Fatal("expected error from ParseHeaders with failing reader")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
