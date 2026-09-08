/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to verify initialization-loop and branch trampolines.
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
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"golang.org/x/arch/arm64/arm64asm"
)

func Test_x26MOVZ(t *testing.T) {
	convey.Convey("Test_x26MOVZ", t, func() {
		inst := fmt.Sprintf("%x", x26MOVZ(0x123456789abcef01))
		convey.So(inst, convey.ShouldEqual, "3ae09dd2")
	})
}

func Test_x26MOVK(t *testing.T) {
	convey.Convey("Test_x26MOVK", t, func() {
		inst := fmt.Sprintf("%x", x26MOVK(0x123456789abcef01, 1))
		convey.So(inst, convey.ShouldEqual, "9a57b3f2")
	})
}

func TestDisassembleInitializationLoop(t *testing.T) {
	// The back-edge lies past the former 64-byte read limit and returns
	// into the 24-byte prefix replaced by the hook.
	code := make([]byte, 88)
	for offset := 0; offset < len(code); offset += 4 {
		binary.LittleEndian.PutUint32(code[offset:], 0xd503201f)
	}
	relative := int32((16 - 80) / 4)
	binary.LittleEndian.PutUint32(code[80:], 0xb5000001|(uint32(relative)&0x7ffff)<<5)
	binary.LittleEndian.PutUint32(code[84:], 0xd65f03c0)
	if got := Disassemble(code, 24, true); got != 84 {
		t.Fatalf("copied prefix length = %d, want 84", got)
	}
	t.Run("reject relative address in extended prefix", func(t *testing.T) {
		binary.LittleEndian.PutUint32(code[32:], 0x90000000)
		defer func() {
			if recover() == nil {
				t.Fatal("expected rejection of an instruction requiring relocation")
			}
		}()
		Disassemble(code, 24, true)
	})
}

func TestDisassembleShortUnsafeFunction(t *testing.T) {
	code := make([]byte, 16)
	for offset := 0; offset < len(code); offset += 4 {
		binary.LittleEndian.PutUint32(code[offset:], 0xd503201f)
	}
	binary.LittleEndian.PutUint32(code[12:], 0xd65f03c0)
	if got := Disassemble(code, 24, false); got != 24 {
		t.Fatalf("unsafe overwrite width = %d, want 24", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("safe patch must reject a short function")
		}
	}()
	Disassemble(code, 24, true)
}

func TestRelocateStackGuardBranch(t *testing.T) {
	code := make([]byte, 128)
	for offset := 0; offset < 24; offset += 4 {
		binary.LittleEndian.PutUint32(code[offset:], 0xd503201f)
	}
	binary.LittleEndian.PutUint32(code, 0x54001009)
	binary.LittleEndian.PutUint32(code[4:], 0xb5ffffe1)
	RelocateBranches(code, 0x1000, 24, 44)
	instruction, err := arm64asm.Decode(code)
	if err != nil {
		t.Fatal(err)
	}
	if got := int(instruction.Args[1].(arm64asm.PCRel)); got != 44 {
		t.Fatalf("guard branch goes to %d, want local stub at 44", got)
	}
	if got := binary.LittleEndian.Uint32(code[4:]); got != 0xb5ffffe1 {
		t.Fatal("branch within copied prefix was changed")
	}
	if want := BranchToOriginal(0x1200); !bytes.Equal(code[44:44+len(want)], want) {
		t.Fatal("branch stub lost original stack-growth destination")
	}
}

func TestRelocatePageAddress(t *testing.T) {
	code := make([]byte, 128)
	for offset := 0; offset < 24; offset += 4 {
		binary.LittleEndian.PutUint32(code[offset:], 0xd503201f)
	}
	binary.LittleEndian.PutUint32(code, 0x90000000)
	RelocateBranches(code, 0x12345678, 24, 44)
	instruction, err := arm64asm.Decode(code)
	if err != nil {
		t.Fatal(err)
	}
	if instruction.Op != arm64asm.LDR || instruction.Args[0] != arm64asm.X0 {
		t.Fatalf("address load = %v", instruction)
	}
	offset := int(instruction.Args[1].(arm64asm.PCRel))
	if address := binary.LittleEndian.Uint64(code[offset:]); address != 0x12345000 {
		t.Fatalf("relocated page address = %#x", address)
	}
}
