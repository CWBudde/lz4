package lz4block

import (
	"testing"
)

func TestBlockGetPut(t *testing.T) {
	expect := map[BlockSizeIndex]uint32{ // block index -> expected size
		4: Block64Kb,
		5: Block256Kb,
		6: Block1Mb,
		7: Block4Mb,
		3: Block8Mb,
	}
	for idx, size := range expect {
		buf := idx.Get()
		if uint32(cap(buf)) != size {
			t.Errorf("expected size %d for index %d, got %d", size, idx, cap(buf))
		}
		Put(buf) // ensure no panic
	}
}

func TestBlockGetInvalid(t *testing.T) {
	defer func() { recover() }() // swallow panic
	_ = BlockSizeIndex(123).Get()
	t.Fatalf("expected panic on bad Get")
}

func TestBlockPutInvalid(t *testing.T) {
	defer func() { recover() }() // swallow panic
	Put(make([]byte, 123))
	t.Fatalf("expected panic on bad Put")
}

func TestIndex(t *testing.T) {
	tests := []struct {
		size uint32
		want BlockSizeIndex
	}{
		{Block64Kb, 4},
		{Block256Kb, 5},
		{Block1Mb, 6},
		{Block4Mb, 7},
		{Block8Mb, 3},
		{0, 0},
		{12345, 0},
	}
	for _, tt := range tests {
		if got := Index(tt.size); got != tt.want {
			t.Errorf("Index(%d) = %d; want %d", tt.size, got, tt.want)
		}
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		size  uint32
		valid bool
	}{
		{Block64Kb, true},
		{Block256Kb, true},
		{Block1Mb, true},
		{Block4Mb, true},
		{Block8Mb, true},
		{0, false},
		{12345, false},
	}
	for _, tt := range tests {
		if got := IsValid(tt.size); got != tt.valid {
			t.Errorf("IsValid(%d) = %v; want %v", tt.size, got, tt.valid)
		}
	}
}

func TestBlockSizeIndexIsValid(t *testing.T) {
	tests := []struct {
		idx   BlockSizeIndex
		valid bool
	}{
		{4, true},
		{5, true},
		{6, true},
		{7, true},
		{0, false},
		{3, false},
		{8, false},
		{123, false},
	}
	for _, tt := range tests {
		if got := tt.idx.IsValid(); got != tt.valid {
			t.Errorf("BlockSizeIndex(%d).IsValid() = %v; want %v", tt.idx, got, tt.valid)
		}
	}
}

func BenchmarkGetPut(b *testing.B) {
	const idx = BlockSizeIndex(4)
	for i := 0; i < b.N; i++ {
		buf := idx.Get()
		Put(buf)
	}
}
