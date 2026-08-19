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

func TestRelocate(t *testing.T) {
	t.Run("short conditional branch", func(t *testing.T) {
		oldAddr := uintptr(0x100000)
		newAddr := uintptr(0x200000)
		code := []byte{
			0x48, 0x85, 0xc0, // TEST RAX, RAX
			0x74, 0x04, // JE oldAddr+9
		}
		relocated, err := Relocate(code, oldAddr, newAddr)
		if err != nil {
			t.Fatal(err)
		}
		if len(relocated) != 9 {
			t.Fatalf("relocated length = %d, want 9", len(relocated))
		}
		jump, err := x86asm.Decode(relocated[3:], 64)
		if err != nil {
			t.Fatal(err)
		}
		if jump.Op != x86asm.JE || jump.PCRel != 4 {
			t.Fatalf("relocated jump = %v", jump)
		}
		got := branchTarget(t, newAddr+3, jump)
		if want := oldAddr + 9; got != want {
			t.Fatalf("relocated jump target = 0x%x, want 0x%x", got, want)
		}
	})

	t.Run("prefixed short conditional branch", func(t *testing.T) {
		oldAddr := uintptr(0x100000)
		newAddr := uintptr(0x200000)
		// CS is a branch-prediction prefix in 64-bit mode. It remains part of
		// the encoding when JE rel8 expands to JE rel32.
		code := []byte{0x2e, 0x74, 0x00}
		relocated, err := Relocate(code, oldAddr, newAddr)
		if err != nil {
			t.Fatal(err)
		}
		want := []byte{0x2e, 0x0f, 0x84, 0xfc, 0xff, 0xef, 0xff}
		if string(relocated) != string(want) {
			t.Fatalf("relocated = % x, want % x", relocated, want)
		}
		jump, err := x86asm.Decode(relocated, 64)
		if err != nil {
			t.Fatal(err)
		}
		if got := branchTarget(t, newAddr, jump); got != oldAddr+3 {
			t.Fatalf("relocated jump target = 0x%x, want 0x%x", got, oldAddr+3)
		}
	})

	t.Run("RIP relative memory", func(t *testing.T) {
		oldAddr := uintptr(0x300000)
		newAddr := uintptr(0x380000)
		code := []byte{0x48, 0x8b, 0x05, 0x98, 0xff, 0x0b, 0x00}
		original, err := x86asm.Decode(code, 64)
		if err != nil {
			t.Fatal(err)
		}
		want := pcRelativeTarget(t, oldAddr, original, code)

		relocated, err := Relocate(code, oldAddr, newAddr)
		if err != nil {
			t.Fatal(err)
		}
		moved, err := x86asm.Decode(relocated, 64)
		if err != nil {
			t.Fatal(err)
		}
		if got := pcRelativeTarget(t, newAddr, moved, relocated); got != want {
			t.Fatalf("relocated memory target = 0x%x, want 0x%x", got, want)
		}
	})

	t.Run("unsupported short loop", func(t *testing.T) {
		if _, err := Relocate([]byte{0xe2, 0xfe}, 0x100000, 0x200000); err == nil {
			t.Fatal("expected LOOP relocation to be rejected")
		}
	})
}

func TestHasPCRelative(t *testing.T) {
	if HasPCRelative([]byte{0x48, 0x85, 0xc0}) {
		t.Fatal("TEST must be position independent")
	}
	if !HasPCRelative([]byte{0x74, 0x04}) {
		t.Fatal("short conditional jump must be position relative")
	}
	if !HasPCRelative([]byte{0x48, 0x8b, 0x05, 0x98, 0xff, 0x0b, 0x00}) {
		t.Fatal("RIP-relative MOV must be position relative")
	}
}

func branchTarget(t *testing.T, address uintptr, inst x86asm.Inst) uintptr {
	t.Helper()
	for _, arg := range inst.Args {
		if rel, ok := arg.(x86asm.Rel); ok {
			target, valid := addSigned(address+uintptr(inst.Len), int64(rel))
			if !valid {
				t.Fatal("branch target overflow")
			}
			return target
		}
	}
	t.Fatal("instruction has no relative target")
	return 0
}

func pcRelativeTarget(t *testing.T, address uintptr, inst x86asm.Inst, code []byte) uintptr {
	t.Helper()
	displacement, err := readSigned(code[inst.PCRelOff:], inst.PCRel)
	if err != nil {
		t.Fatal(err)
	}
	target, valid := addSigned(address+uintptr(inst.Len), displacement)
	if !valid {
		t.Fatal("pc-relative target overflow")
	}
	return target
}
