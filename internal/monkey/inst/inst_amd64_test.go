/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to verify trampoline branch relocation.
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
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
	"golang.org/x/arch/x86/x86asm"

	"github.com/smartystreets/goconvey/convey"
)

func Test_rdxMOV(t *testing.T) {
	convey.Convey("Test_rdxMOV", t, func() {
		inst := fmt.Sprintf("%x", rdxMOV(0x123456789abcef01))
		convey.So(inst, convey.ShouldEqual, "48ba01efbc9a78563412")
	})
}

func TestRelocateStackGuardBranch(t *testing.T) {
	code := make([]byte, 128)
	for i := 0; i < 16; i++ {
		code[i] = 0x90
	}
	copy(code, []byte{0x0f, 0x86, 0xfa, 0x01, 0, 0}) // JBE original+0x200
	RelocateBranches(code, 0x1000, 16, 28)
	instruction, err := x86asm.Decode(code, 64)
	if err != nil {
		t.Fatal(err)
	}
	if got := instruction.Len + int(instruction.Args[0].(x86asm.Rel)); got != 28 {
		t.Fatalf("guard branch goes to %d, want local stub at 28", got)
	}
	if want := BranchToOriginal(0x1200); !bytes.Equal(code[28:28+len(want)], want) {
		t.Fatal("branch stub lost original stack-growth destination")
	}
}

func TestRelocateDistantLEA(t *testing.T) {
	for _, test := range []struct {
		name string
		code []byte
	}{
		{"AX", []byte{0x66, 0x8d, 0x05, 0x10, 0, 0, 0}},
		{"EAX", []byte{0x8d, 0x05, 0x10, 0, 0, 0}},
		{"RAX", []byte{0x48, 0x8d, 0x05, 0x10, 0, 0, 0}},
		{"R9W", []byte{0x66, 0x44, 0x8d, 0x0d, 0xf0, 0xff, 0xff, 0xff}},
		{"R9D", []byte{0x44, 0x8d, 0x0d, 0xf0, 0xff, 0xff, 0xff}},
		{"R9", []byte{0x4c, 0x8d, 0x0d, 0xf0, 0xff, 0xff, 0xff}},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalInstruction, err := x86asm.Decode(test.code, 64)
			if err != nil {
				t.Fatal(err)
			}
			code := make([]byte, 128)
			copy(code, test.code)
			original := common.PtrOf(code) + 1<<33
			RelocateBranches(code, original, len(test.code), 32)
			instruction, err := x86asm.Decode(code, 64)
			if err != nil {
				t.Fatal(err)
			}
			if instruction.Op != x86asm.MOV || instruction.Len != originalInstruction.Len ||
				instruction.Args[0] != originalInstruction.Args[0] || instruction.DataSize != originalInstruction.DataSize {
				t.Fatalf("relocation changed the destination or instruction width: %v -> %v", originalInstruction, instruction)
			}
			address := instruction.Args[1].(x86asm.Mem)
			literal := instruction.Len + int(address.Disp)
			if address.Base != x86asm.RIP || literal != 32 {
				t.Fatalf("relocated instruction does not reference its local literal: %v", instruction)
			}
			want := original + uintptr(originalInstruction.Len) + uintptr(int32(originalInstruction.Args[1].(x86asm.Mem).Disp))
			if got := binary.LittleEndian.Uint64(code[literal:]); got != uint64(want) {
				t.Fatalf("literal contains %#x, want original effective address %#x", got, want)
			}
		})
	}
}

func TestRelocateNearbyLEA(t *testing.T) {
	for _, prefix := range [][]byte{nil, {0x64}, {0x65}} {
		code := common.AllocatePage()
		defer common.ReleasePage(code)
		encoding := append(prefix, 0x48, 0x8d, 0x05, 0xf0, 0xff, 0xff, 0xff)
		copy(code, encoding)
		original := common.PtrOf(code) + 0x1000
		RelocateBranches(code, original, len(encoding), 32)
		instruction, err := x86asm.Decode(code, 64)
		if err != nil {
			t.Fatal(err)
		}
		if instruction.Op != x86asm.LEA || int32(instruction.Args[1].(x86asm.Mem).Disp) != 0xff0 ||
			!bytes.Equal(code[:len(encoding)-4], encoding[:len(encoding)-4]) {
			t.Fatalf("nearby LEA changed its effective address or prefixes: %v", instruction)
		}
		if !bytes.Equal(code[32:64], make([]byte, 32)) {
			t.Fatal("nearby LEA unexpectedly allocated a literal")
		}
	}
}

func TestRelocateLEALiteralsAndBranch(t *testing.T) {
	code := make([]byte, 128)
	copy(code, []byte{
		0x48, 0x8d, 0x05, 0x10, 0, 0, 0, // LEA RAX, [RIP+0x10]
		0x0f, 0x86, 0xf3, 0x01, 0, 0, // JBE original+0x200
		0x4c, 0x8d, 0x0d, 0xf0, 0xff, 0xff, 0xff, // LEA R9, [RIP-0x10]
	})
	original := common.PtrOf(code) + 1<<33
	branchBack := BranchToOriginal(original + 20)
	copy(code[20:], branchBack)
	RelocateBranches(code, original, 20, 34)
	if !bytes.Equal(code[20:34], branchBack) {
		t.Fatal("relocation overwrote the branch back to the original function")
	}
	if got := binary.LittleEndian.Uint64(code[34:]); got != uint64(original+23) {
		t.Fatalf("first literal contains %#x, want %#x", got, original+23)
	}
	if want := BranchToOriginal(original + 0x200); !bytes.Equal(code[42:56], want) {
		t.Fatal("LEA literal overwrote the stack-growth branch stub")
	}
	if got := binary.LittleEndian.Uint64(code[56:]); got != uint64(original+4) {
		t.Fatalf("second literal contains %#x, want %#x", got, original+4)
	}
}

func TestRelocateLEARejectsUnsupportedForms(t *testing.T) {
	for _, test := range []struct {
		name       string
		code       []byte
		stubOffset int
		message    string
	}{
		{"literal bounds", []byte{0x48, 0x8d, 0x05, 0, 0, 0, 0}, 57, "trampoline literals exceed page size"},
		{"literal overlap", []byte{0x48, 0x8d, 0x05, 0, 0, 0, 0}, 0, "trampoline literals exceed page size"},
		{"segment prefix", []byte{0x64, 0x48, 0x8d, 0x05, 0, 0, 0, 0}, 32, "cannot relocate PC-relative LEA"},
		{"address size prefix", []byte{0x67, 0x48, 0x8d, 0x05, 0, 0, 0, 0}, 32, "cannot relocate EIP-relative LEA"},
		{"data load", []byte{0x48, 0x8b, 0x05, 0, 0, 0, 0}, 32, "PC-relative data reference is out of trampoline range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				failure := recover()
				if failure == nil || !strings.Contains(fmt.Sprint(failure), test.message) {
					t.Fatalf("got panic %v, want %q", failure, test.message)
				}
			}()
			code := make([]byte, 64)
			copy(code, test.code)
			RelocateBranches(code, common.PtrOf(code)+1<<33, len(test.code), test.stubOffset)
		})
	}
}
