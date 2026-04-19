package lz4block

import "testing"

func TestCompressorGetPutCurrentWindow(t *testing.T) {
	var c Compressor
	const h = 12345

	c.reset()
	if got := c.get(h, 32); got != 32-winSize {
		t.Fatalf("empty get = %d; want %d", got, 32-winSize)
	}

	c.put(h, winSize-1)
	if got := c.get(h, winSize); got != winSize-1 {
		t.Fatalf("get after put across boundary = %d; want %d", got, winSize-1)
	}

	c.reset()
	if got := c.get(h, winSize); got != 0 {
		t.Fatalf("stale entry after reset = %d; want 0", got)
	}
}

func TestCompressorResetClearsInUse(t *testing.T) {
	var c Compressor
	const h = 321

	c.put(h, 123)

	c.reset()

	if c.inUse[h/32] != 0 {
		t.Fatalf("inUse word after reset = %032b; want 0", c.inUse[h/32])
	}
	if got := c.get(h, 200); got != 200-winSize {
		t.Fatalf("stale entry after reset = %d; want %d", got, 200-winSize)
	}
}
