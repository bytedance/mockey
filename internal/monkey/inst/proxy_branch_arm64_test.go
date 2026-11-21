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
	"encoding/binary"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func arm64Code(insts ...uint32) []byte {
	code := make([]byte, 4*len(insts))
	for i, inst := range insts {
		binary.LittleEndian.PutUint32(code[4*i:], inst)
	}
	return code
}

func TestBuildProxyBranches(t *testing.T) {
	convey.Convey("TestBuildProxyBranches", t, func() {
		convey.Convey("stack check branch to morestack", func() {
			// Prologue of time.NewTicker (go1.27.1, -gcflags=all=-N -l):
			// MOVD 16(R28), R16; CMP R16, RSP; BLS 59(PC); MOVD.W R30, -128(RSP); MOVD R29, -8(RSP); SUB $8, RSP, R29
			code := arm64Code(0xf9400b90, 0xeb3063ff, 0x54000769, 0xf8180ffe, 0xf81f83fd, 0xd10023fd)
			target := uintptr(0x1000b8d40)
			proxy := BuildProxy(target, 0, code)

			expected := arm64Code(0xf9400b90, 0xeb3063ff, 0x54000129, 0xf8180ffe, 0xf81f83fd, 0xd10023fd) // BLS 9(PC)
			expected = append(expected, BranchTo(target+24)...)
			expected = append(expected, BranchTo(target+8+59*4)...)
			convey.So(proxy, convey.ShouldResemble, expected)
		})
		convey.Convey("branch inside the copied code", func() {
			// CBZ R0, 2(PC); NOP; NOP
			code := arm64Code(0xb4000040, 0xd503201f, 0xd503201f)
			proxy := BuildProxy(0x1000, 0, code)
			convey.So(proxy, convey.ShouldResemble, append(append([]byte(nil), code...), BranchTo(0x100c)...))
		})
		convey.Convey("backward TBZ and B leaving the copied code", func() {
			// TBZ W1, #3, -2(PC); B 100(PC)
			code := arm64Code(0x361fffc1, 0x14000064)
			target := uintptr(0x10000)
			proxy := BuildProxy(target, 0, code)

			expected := arm64Code(0x361800e1, 0x1400000b) // TBZ W1, #3, 7(PC); B 11(PC)
			expected = append(expected, BranchTo(target+8)...)
			expected = append(expected, BranchTo(target-8)...)
			expected = append(expected, BranchTo(target+4+100*4)...)
			convey.So(proxy, convey.ShouldResemble, expected)
		})
	})
}
