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
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func Test_rdxMOV(t *testing.T) {
	convey.Convey("Test_rdxMOV", t, func() {
		inst := fmt.Sprintf("%x", rdxMOV(0x123456789abcef01))
		convey.So(inst, convey.ShouldEqual, "48ba01efbc9a78563412")
	})
}

func TestBranchIntoShort(t *testing.T) {
	from := uintptr(0x100000)
	to := uintptr(0x200123)
	code, ok := BranchIntoShort(from, to)
	if !ok {
		t.Fatal("expected target to be reachable")
	}
	if len(code) != ShortBranchSize() || code[0] != 0xe9 {
		t.Fatalf("short branch = %x", code)
	}
	delta := int64(int32(binary.LittleEndian.Uint32(code[1:])))
	got, valid := addSigned(from+uintptr(len(code)), delta)
	if !valid || got != to {
		t.Fatalf("short branch target = 0x%x, want 0x%x", got, to)
	}

	if _, ok := BranchIntoShort(from, from+1<<31+uintptr(ShortBranchSize())); ok {
		t.Fatal("expected out-of-range target to be rejected")
	}
}
