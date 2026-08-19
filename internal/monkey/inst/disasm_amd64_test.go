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

package inst

import (
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

// branchLen is the width of the branch sequence Disassemble is asked to make
// room for on amd64: MOV RDX, imm64 (10 bytes) + JMP RDX (2 bytes).
// It matches len(BranchInto(...)), which is what patch.go passes as `required`.
const branchLen = 12

// ret is a single-byte RET (0xc3), i.e. the shortest possible function body.
const ret = 0xc3

// TestDisassembleShortFunction covers the case where the target function ends
// before `required` bytes. Overwriting past its RET is safe only when the
// remaining bytes are linker padding rather than a neighbouring function.
func TestDisassembleShortFunction(t *testing.T) {
	// A real function prologue, used as the "neighbour" that must not be clobbered:
	// MOV RBP, RSP; SUB RSP, 0x10; TEST RAX, RAX; JNE ...
	neighbour := []byte{0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x10, 0x48, 0x85, 0xc0, 0x75, 0xcc}

	padded := func(prefix []byte) []byte {
		out := append([]byte{}, prefix...)
		for len(out) < 32 {
			out = append(out, padByte)
		}
		return out
	}

	tests := []struct {
		name    string
		code    []byte
		wantErr bool
	}{
		{
			name: "one byte function followed by padding",
			code: padded([]byte{ret}),
		},
		{
			// LEA RCX, [RAX+RAX*2]; LEA RCX, [RCX+7]; ADD RBX, RAX; RET at offset 10.
			name: "function ending just before the branch width",
			code: padded([]byte{0x48, 0x8d, 0x0c, 0x40, 0x48, 0x8d, 0x49, 0x07, 0x48, 0x01, 0xc3}),
		},
		{
			name:    "one byte function followed by a neighbour",
			code:    append([]byte{ret}, neighbour...),
			wantErr: true,
		},
		{
			// MOV RAX, RCX; RET; two bytes of padding; then a neighbour that
			// still starts inside the window we would overwrite.
			name:    "padding that ends before the branch width",
			code:    append([]byte{0x48, 0x89, 0xc8, ret, padByte, padByte}, neighbour...),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := recoverOf(func() { Disassemble(tt.code, branchLen, true) })
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected Disassemble to refuse, but it returned")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected Disassemble to accept, got %v", err)
			}
			// The cutting index must span the whole branch sequence, otherwise
			// Unpatch would restore fewer bytes than Patch overwrote.
			if got := Disassemble(tt.code, branchLen, true); got != branchLen {
				t.Fatalf("cutting index = %d, want %d", got, branchLen)
			}
		})
	}
}

// TestDisassembleShortFunctionUnsafe pins the OptUnsafe path: when the caller
// opts out of the length check, Disassemble must not refuse for any reason.
func TestDisassembleShortFunctionUnsafe(t *testing.T) {
	code := []byte{ret, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x10, 0x48, 0x85, 0xc0, 0x75, 0xcc}
	if err := recoverOf(func() { Disassemble(code, branchLen, false) }); err != nil {
		t.Fatalf("unchecked path must not refuse, got %v", err)
	}
}

// TestDisassembleReturnsInstructionBoundary is the regression test for the
// cutting index landing in the middle of an instruction.
//
// patch.go uses the return value both as the length of the prefix copied into
// the trampoline and as the address it branches back to (targetAddr+cuttingIdx),
// so a value that is not an instruction boundary produces a trampoline whose
// last instruction is truncated and whose jump lands mid-instruction. Returning
// the raw `required` from the RET branch did exactly that whenever the bytes
// after the RET were real code rather than 0xCC padding, which is reachable via
// MockUnsafe (checkLen=false).
func TestDisassembleReturnsInstructionBoundary(t *testing.T) {
	tests := []struct {
		name string
		code []byte
		want int
	}{
		{
			// The shape seen in a real service: a frameless function with an early
			// return, whose continuation is a 4-byte IMUL straddling `required`.
			//   @0 TEST RAX,RAX (3) | @3 JLE +6 (2) | @5 MOV EAX,7 (5)
			//   @10 RET (1)         | @11 IMUL RAX,RAX (4) -> covers [11,15)
			// The old code returned 12, i.e. two bytes into that IMUL.
			name: "early ret followed by a wide instruction",
			code: []byte{
				0x48, 0x85, 0xc0, // TEST RAX, RAX
				0x7e, 0x06, // JLE +6
				0xb8, 0x07, 0x00, 0x00, 0x00, // MOV EAX, 7
				ret,                    // RET   @10
				0x48, 0x0f, 0xaf, 0xc0, // IMUL RAX, RAX  @11..14
				0x48, 0x83, 0xc0, 0x03, // ADD RAX, 3
				ret,
			},
			want: 15,
		},
		{
			// Padding is INT3, one byte wide, so rounding up cannot overshoot:
			// the boundary is exactly `required`, same value the old code returned.
			name: "early ret followed by padding",
			code: []byte{0x48, 0x89, 0xc8, ret, padByte, padByte, padByte, padByte,
				padByte, padByte, padByte, padByte, padByte, padByte, padByte, padByte},
			want: branchLen,
		},
		{
			// A single 2-byte instruction straddling the branch width, so the
			// boundary sits one byte past `required` rather than on it.
			name: "early ret followed by a two byte instruction",
			code: []byte{
				0x48, 0x85, 0xc0, // TEST RAX, RAX
				0x7e, 0x06, // JLE +6
				0xb8, 0x07, 0x00, 0x00, 0x00, // MOV EAX, 7
				ret,        // RET  @10
				0x31, 0xc0, // XOR EAX, EAX  @11..12
				ret,
			},
			want: 13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// checkLen=false is the MockUnsafe path, the only one that is allowed
			// to walk past real (non-padding) code.
			got := Disassemble(tt.code, branchLen, false)
			if got != tt.want {
				t.Fatalf("cutting index = %d, want %d", got, tt.want)
			}
			if got < branchLen {
				t.Fatalf("cutting index %d is shorter than the branch sequence %d; "+
					"Unpatch would restore fewer bytes than Patch overwrote", got, branchLen)
			}
			if !onInstructionBoundary(t, tt.code, got) {
				t.Fatalf("cutting index %d is not an instruction boundary", got)
			}
		})
	}
}

// onInstructionBoundary decodes the byte stream from the top and reports whether
// idx is reachable by whole instructions, which is the property patch.go needs.
func onInstructionBoundary(t *testing.T, code []byte, idx int) bool {
	t.Helper()
	for pos := 0; pos < len(code); {
		if pos == idx {
			return true
		}
		if pos > idx {
			return false
		}
		in, err := x86asm.Decode(code[pos:], 64)
		if err != nil {
			t.Fatalf("decode at %d: %v", pos, err)
		}
		pos += in.Len
	}
	return false
}

func recoverOf(f func()) (err interface{}) {
	defer func() { err = recover() }()
	f()
	return nil
}
