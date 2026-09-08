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
	"fmt"
	"testing"

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
