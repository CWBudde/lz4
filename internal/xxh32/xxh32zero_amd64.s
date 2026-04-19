//go:build amd64 && !appengine && gc && !noasm
// +build amd64,!appengine,gc,!noasm

#include "go_asm.h"
#include "textflag.h"

#define round16(ptr, v1, v2, v3, v4, x, prime1, prime2) \
	MOVL 0(ptr), x; \
	IMULL prime2, x; \
	ADDL x, v1; \
	ROLL $13, v1; \
	IMULL prime1, v1; \
	MOVL 4(ptr), x; \
	IMULL prime2, x; \
	ADDL x, v2; \
	ROLL $13, v2; \
	IMULL prime1, v2; \
	MOVL 8(ptr), x; \
	IMULL prime2, x; \
	ADDL x, v3; \
	ROLL $13, v3; \
	IMULL prime1, v3; \
	MOVL 12(ptr), x; \
	IMULL prime2, x; \
	ADDL x, v4; \
	ROLL $13, v4; \
	IMULL prime1, v4

// func ChecksumZero(input []byte) uint32
TEXT ·ChecksumZero(SB), NOSPLIT, $0-32
	MOVQ input_base+0(FP), SI
	MOVQ input_len+8(FP), BX
	MOVL BX, AX

	CMPQ BX, $16
	JAE  bulk

	MOVL $const_prime5, R14
	ADDL R14, AX
	JMP  tail4

bulk:
	MOVL $const_prime1plus2, R8
	MOVL $const_prime2, R9
	XORL R10, R10
	MOVL $const_prime1minus, R11
	MOVL $const_prime2, R14
	MOVL $const_prime1, R15

bulkLoop:
	round16(SI, R8, R9, R10, R11, DX, R15, R14)
	ADDQ $16, SI
	SUBQ $16, BX
	CMPQ BX, $16
	JAE  bulkLoop

	MOVL input_len+8(FP), AX
	MOVL R8, DX
	ROLL $1, DX
	ADDL DX, AX
	MOVL R9, DX
	ROLL $7, DX
	ADDL DX, AX
	MOVL R10, DX
	ROLL $12, DX
	ADDL DX, AX
	MOVL R11, DX
	ROLL $18, DX
	ADDL DX, AX

tail4:
	CMPQ BX, $4
	JB   tail1

	MOVL $const_prime3, R14
	MOVL $const_prime4, R15

tail4Loop:
	MOVL (SI), DX
	IMULL R14, DX
	ADDL DX, AX
	ROLL $17, AX
	IMULL R15, AX
	ADDQ $4, SI
	SUBQ $4, BX
	CMPQ BX, $4
	JAE  tail4Loop

tail1:
	CMPQ BX, $0
	JE   avalanche

	MOVL $const_prime5, R14
	MOVL $const_prime1, R15

tail1Loop:
	MOVBLZX (SI), DX
	IMULL R14, DX
	ADDL DX, AX
	ROLL $11, AX
	IMULL R15, AX
	INCQ SI
	DECQ BX
	JNE  tail1Loop

avalanche:
	MOVL AX, DX
	SHRL $15, DX
	XORL DX, AX
	MOVL $const_prime2, R14
	IMULL R14, AX
	MOVL AX, DX
	SHRL $13, DX
	XORL DX, AX
	MOVL $const_prime3, R14
	IMULL R14, AX
	MOVL AX, DX
	SHRL $16, DX
	XORL DX, AX
	MOVL AX, ret+24(FP)
	RET

// func update(v *[4]uint32, buf *[16]byte, input []byte)
TEXT ·update(SB), NOSPLIT, $0-40
	MOVQ v+0(FP), DI
	MOVL 0(DI), R8
	MOVL 4(DI), R9
	MOVL 8(DI), R10
	MOVL 12(DI), R11
	MOVL $const_prime2, R14
	MOVL $const_prime1, R15

	MOVQ buf+8(FP), SI
	CMPQ SI, $0
	JE   updateInput
	round16(SI, R8, R9, R10, R11, DX, R15, R14)

updateInput:
	MOVQ input_base+16(FP), SI
	MOVQ input_len+24(FP), BX
	CMPQ BX, $16
	JB   updateStore

updateLoop:
	round16(SI, R8, R9, R10, R11, DX, R15, R14)
	ADDQ $16, SI
	SUBQ $16, BX
	CMPQ BX, $16
	JAE  updateLoop

updateStore:
	MOVQ v+0(FP), DI
	MOVL R8, 0(DI)
	MOVL R9, 4(DI)
	MOVL R10, 8(DI)
	MOVL R11, 12(DI)
	RET
