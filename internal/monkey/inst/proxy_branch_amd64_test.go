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

	"github.com/smartystreets/goconvey/convey"
)

func TestBuildProxyBranches(t *testing.T) {
	convey.Convey("TestBuildProxyBranches", t, func() {
		convey.Convey("stack check branch to morestack", func() {
			// Prologue of time.NewTicker (go1.27.1, -gcflags=all=-N -l):
			// CMPQ SP, 0x10(R14); JBE rel32; PUSHQ BP; MOVQ SP, BP
			code := []byte{0x49, 0x3b, 0x66, 0x10, 0x0f, 0x86, 0xe0, 0x00, 0x00, 0x00, 0x55, 0x48, 0x89, 0xe5}
			target := uintptr(0x10d03a0)
			proxy := BuildProxy(target, 0, code)

			// JBE to the absolute jump after the jump back to the target
			expected := []byte{0x49, 0x3b, 0x66, 0x10, 0x0f, 0x86, 0x10, 0x00, 0x00, 0x00, 0x55, 0x48, 0x89, 0xe5}
			expected = append(expected, BranchTo(target+14)...)
			expected = append(expected, BranchTo(0x10d048a)...)
			convey.So(proxy, convey.ShouldResemble, expected)
		})
		convey.Convey("short branches", func() {
			// JMP +2 (inside, over the JBE); JBE +0x30 (leaves the code); NOP x4; JMP -12 (leaves the code)
			code := []byte{0xeb, 0x02, 0x76, 0x30, 0x90, 0x90, 0x90, 0x90, 0xeb, 0xf4}
			target := uintptr(0x2000)
			proxy := BuildProxy(target, 0, code)

			expected := []byte{
				0xeb, 0x06, // JMP +6, still to the first NOP
				0x0f, 0x86, 0x15, 0x00, 0x00, 0x00, // JBE rel32 to proxy[29:]
				0x90, 0x90, 0x90, 0x90,
				0xe9, 0x18, 0x00, 0x00, 0x00, // JMP rel32 to proxy[41:]
			}
			expected = append(expected, BranchTo(target+10)...)
			expected = append(expected, BranchTo(target+4+0x30)...)
			expected = append(expected, BranchTo(target+10-12)...)
			convey.So(proxy, convey.ShouldResemble, expected)
		})
		convey.Convey("no branch", func() {
			code := []byte{0x55, 0x48, 0x89, 0xe5}
			proxy := BuildProxy(0x3000, 0, code)
			convey.So(proxy, convey.ShouldResemble, append(append([]byte(nil), code...), BranchTo(0x3004)...))
		})
	})
}
