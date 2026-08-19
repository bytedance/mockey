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

import "unsafe"

func BranchTo(to uintptr) (res []byte) {
	res = append(res, x26MOV(to)...)                     // MOV x26, to // fake
	res = append(res, []byte{0x40, 0x03, 0x1f, 0xd6}...) // BR x26
	return
}

// BranchInto create a branch into command
//
// Go supports passing function arguments from go 1.17 (see https://go.dev/doc/go1.17).
// We could not use x0~x18 register. As an alternative, we use R19 register (see https://go.googlesource.com/go/+/refs/heads/master/src/cmd/compile/abi-internal.md).
func BranchInto(to uintptr) (res []byte) {
	res = append(res, x26MOV(to)...)                     // MOV x26, to // fake
	res = append(res, []byte{0x53, 0x03, 0x40, 0xf9}...) // LDR x19, [x26]
	res = append(res, []byte{0x60, 0x02, 0x1f, 0xd6}...) // BR x19
	return
}

func ShortBranchSize() int {
	return 0
}

func BranchIntoShort(_, _ uintptr) ([]byte, bool) {
	return nil, false
}

// nopBytes is the arm64 NOP encoding (little-endian 0xd503201f). Zero would be
// UDF #0, so padding with it turns an unreachable tail into a SIGILL the moment
// it stops being unreachable.
var nopBytes = [4]byte{0x1f, 0x20, 0x03, 0xd5}

// PadEntry cannot currently pad anything on arm64, but it pads correctly when
// it does.
//
// The reason it is unreachable is not that instructions are four bytes wide on
// its own -- that alone would not stop a cutting point from exceeding the entry
// sequence. It is the conjunction of two facts: disassemble here advances by a
// fixed instLen of 4, and every entry sequence this file emits is a whole
// number of instructions (BranchInto is 24 bytes, ShortBranchLen is 4). So the
// loop's exit lands exactly on `required` and cuttingIdx never overshoots
// len(hookCode).
//
// Both are properties of the current implementation, not of the architecture,
// so it is worth not depending on either. Should arm64 ever round its cutting
// point up past a variable-width boundary the way amd64 does, or should an
// entry sequence stop being instruction-aligned, this fills with NOPs like its
// amd64 counterpart instead of leaving zeroes behind -- and zero is UDF #0, so
// the tail would become a SIGILL the moment it stopped being unreachable.
func PadEntry(code []byte, width int) []byte {
	if len(code) >= width {
		return code
	}
	padded := make([]byte, width)
	copy(padded, code)
	for i := len(code); i < width; i++ {
		padded[i] = nopBytes[i%4]
	}
	return padded
}

const x26 uint32 = 0b11010

// x26MOV moves the 64bit value to x26 register, using the following four instructions:
// MOVZ x26, val[0:16]
// MOVK x26, val[16:32]
// MOVK x26, val[32:48]
// MOVK x26, val[48:64]
func x26MOV(val uintptr) (res []byte) {
	res = append(res, x26MOVZ(val)...)
	res = append(res, x26MOVK(val, 1)...)
	res = append(res, x26MOVK(val, 2)...)
	res = append(res, x26MOVK(val, 3)...)
	return res
}

// x26MOVZ see https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/MOVZ--Move-wide-with-zero-
func x26MOVZ(val uintptr) []byte {
	imm := uint32(val & 0xffff)
	inst := 0b1<<31 | 0b1010010100<<21 | imm<<5 | x26
	res := make([]byte, 4)
	*(*uint32)(unsafe.Pointer(&res[0])) = inst
	return res
}

// x26MOVK see https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/MOVK--Move-wide-with-keep-
func x26MOVK(val uintptr, shift int) []byte {
	imm := uint32((val >> (shift * 0x10)) & 0xffff)
	inst := 0b1<<31 | 0b111100101<<23 | uint32(shift)&0b11<<21 | imm<<5 | 26
	res := make([]byte, 4)
	*(*uint32)(unsafe.Pointer(&res[0])) = inst
	return res
}
