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
	"unsafe"
)

const shortBranchSize = 5

func BranchTo(to uintptr) (res []byte) {
	res = append(res, rdxMOV(to)...)         // MOVABS RDX, to
	res = append(res, []byte{0xff, 0xe2}...) // JMP RDX
	return
}

func BranchInto(to uintptr) (res []byte) {
	res = append(res, rdxMOV(to)...)         // MOVABS RDX, to
	res = append(res, []byte{0xff, 0x22}...) // JMP [RDX]
	return
}

func ShortBranchSize() int {
	return shortBranchSize
}

func BranchIntoShort(from, to uintptr) ([]byte, bool) {
	delta, ok := subtractAddress(to, from+shortBranchSize)
	if !ok || delta < -1<<31 || delta > 1<<31-1 {
		return nil, false
	}
	result := make([]byte, shortBranchSize)
	result[0] = 0xe9
	binary.LittleEndian.PutUint32(result[1:], uint32(int32(delta)))
	return result, true
}

// nopByte is a one-byte NOP (XCHG EAX, EAX in 64-bit mode). One byte is all we
// need: PadEntry only ever fills a gap it never intends to execute, so the
// shortest encoding is the simplest correct one.
const nopByte = 0x90

// PadEntry extends an entry sequence to exactly width bytes with NOPs.
//
// Why this is necessary, rather than cosmetic: the cutting point is rounded up
// to an instruction boundary, so it is frequently WIDER than the branch we
// write over it (a five-byte E9 over a six-byte CMP/JBE prologue is the common
// case). Whatever we do not overwrite is the tail of an instruction whose head
// is gone -- an orphan. The CPU never sees it, because the E9 leaves before
// reaching it, so the patch works. But mockey itself reads those bytes back:
// every later Patch of the same function disassembles the live entry, and an
// orphan tail decodes as garbage or not at all ("unrecognized instruction").
//
// Unpatch restores the full cuttingIdx and so was never the problem; the entry
// only has to stay decodable WHILE patched, which is exactly the state a leaked
// (never-unpatched) mock leaves behind.
//
// The legacy path gets this for free: its twelve-byte MOVABS+JMP is written
// over a cut that Disassemble already grew to >= 12, and any excess is padded
// here too, so both paths now maintain the same invariant -- a patched entry is
// always a valid instruction stream.
func PadEntry(code []byte, width int) []byte {
	if len(code) >= width {
		return code
	}
	padded := make([]byte, width)
	copy(padded, code)
	for i := len(code); i < width; i++ {
		padded[i] = nopByte
	}
	return padded
}

// rdxMOV moves the 64bit value to rdx register, using the following instruction:
// MOVABS RDX, val
func rdxMOV(val uintptr) []byte {
	res := make([]byte, unsafe.Sizeof(val))
	*(*uintptr)(unsafe.Pointer(&res[0])) = val
	res = append([]byte{0x48, 0xba}, res...)
	return res
}
