//go:build amd64 && !noasm
// +build amd64,!noasm

package xxh32

import "testing"

func TestImplementationIsAsmOnAMD64(t *testing.T) {
	if implementation != "asm" {
		t.Fatalf("implementation = %q; want asm", implementation)
	}
}
