/*
 * Copyright 2022 ByteDance Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include "textflag.h"

#define NOP4 WORD $0x00000013;
#define NOP16 NOP4; NOP4; NOP4; NOP4;
#define NOP64 NOP16; NOP16; NOP16; NOP16;
#define NOP512 NOP64; NOP64; NOP64; NOP64; NOP64; NOP64; NOP64; NOP64;
#define NOP4096 NOP512; NOP512; NOP512; NOP512; NOP512; NOP512; NOP512; NOP512;
#define NOP16384 NOP4096; NOP4096; NOP4096; NOP4096;
#define NOP65536 NOP16384; NOP16384; NOP16384; NOP16384;

#define protRW $(0x1|0x2)
#define protRX $(0x1|0x4)
#define mProtect $226
#define flushICache $259

TEXT ·write(SB),NOSPLIT|NOFRAME,$0-56
// Keep the executable critical section at least one maximum supported base
// page away from the ABI wrapper and every other function. It must remain a
// leaf while the target page is writable and non-executable.

	JMP	START
	NOP65536
START:
	MOV	mProtect, A7
	MOV	page+24(FP), A0
	MOV	pageSize+32(FP), A1
	MOV	protRW, A2
	ECALL
	BEQZ	A0, MPROTECT_OK
	MOV	A0, ret+48(FP)
	JMP	FINISH

MPROTECT_OK:
	MOV	target+0(FP), A0
	MOV	data+8(FP), A1
	MOV	len+16(FP), A2
	BEQZ	A2, COPY_DONE
	BEQ	A0, A1, COPY_DONE
	BGTU	A0, A1, COPY_BACKWARD
COPY_FORWARD:
	MOVBU	0(A1), T1
	MOVB	T1, 0(A0)
	ADD	$1, A1
	ADD	$1, A0
	SUB	$1, A2
	BNEZ	A2, COPY_FORWARD
	JMP	COPY_DONE
COPY_BACKWARD:
	ADD	A0, A2, A0
	ADD	A1, A2, A1
COPY_BACKWARD_LOOP:
	SUB	$1, A0
	SUB	$1, A1
	MOVBU	0(A1), T1
	MOVB	T1, 0(A0)
	SUB	$1, A2
	BNEZ	A2, COPY_BACKWARD_LOOP

COPY_DONE:
	MOV	flushICache, A7
	MOV	target+0(FP), A0
	MOV	len+16(FP), A1
	ADD	A0, A1, A1
	MOV	$0, A2
	ECALL
	MOV	A0, ret+48(FP)

	MOV	mProtect, A7
	MOV	page+24(FP), A0
	MOV	pageSize+32(FP), A1
	MOV	oriProt+40(FP), A2
	ECALL
	BEQZ	A0, RESTORED

	// Preserve the original restore error, then make one final RX attempt so
	// the ABI wrapper remains executable even when it shares the target page.
	MOV	A0, ret+48(FP)
	MOV	mProtect, A7
	MOV	page+24(FP), A0
	MOV	pageSize+32(FP), A1
	MOV	protRX, A2
	ECALL
	BEQZ	A0, RESTORED

	// Returning through a wrapper on an NX page is impossible. Treat a failed
	// emergency RX restore as unrecoverable instead of continuing corrupted.
	WORD	$0x00000000

RESTORED:
	JMP	FINISH
	NOP65536
FINISH:
	RET
