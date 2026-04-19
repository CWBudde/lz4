package lz4block

import "testing"

func pseudoRandomBytes(n int) []byte {
	out := make([]byte, n)
	var x uint64 = 0x9e3779b97f4a7c15
	for i := range out {
		x ^= x << 7
		x ^= x >> 9
		x ^= x << 8
		out[i] = byte(x)
	}
	return out
}

func TestLikelyIncompressible(t *testing.T) {
	random := pseudoRandomBytes(16 << 10)
	if !likelyIncompressible(random) {
		t.Fatal("expected random data to look incompressible")
	}

	repeated := make([]byte, 16<<10)
	for i := range repeated {
		repeated[i] = "abcd"[i&3]
	}
	if likelyIncompressible(repeated) {
		t.Fatal("expected repeated data to look compressible")
	}

	delayedRepeat := pseudoRandomBytes(8 << 10)
	delayedRepeat = append(delayedRepeat, delayedRepeat...)
	if likelyIncompressible(delayedRepeat) {
		t.Fatal("expected delayed repeat data to look compressible")
	}
}
