//go:build arm64

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
	"unsafe"

	"golang.org/x/arch/arm64/arm64asm"
)

// ShortBranchLen is the size of the entry stub used by the near-trampoline
// strategy: a single unconditional ARM64 `B` instruction.
const ShortBranchLen = 4

// ShortBranchRange is the reach of `B` (+/-128MB).
const ShortBranchRange = 1 << 27

// TooShortToPatch reports whether `code` reaches a RET within `required` bytes,
// i.e. whether the function is too small to hold the long-jump entry sequence.
//
// This is the same condition Disassemble enforces, but as a predicate: it
// answers the question instead of aborting, so PatchValue can consult it
// before choosing a strategy. Undecodable bytes yield false, which leaves the
// diagnosis to Disassemble on the default path rather than silently rerouting
// a target we do not understand.
func TooShortToPatch(code []byte, required int) bool {
	for pos := 0; pos < required; pos += instLen {
		if pos+instLen > len(code) {
			return false
		}
		in, err := arm64asm.Decode(code[pos:])
		if err != nil {
			return false
		}
		if in.Op == arm64asm.RET {
			return true
		}
	}
	return false
}

// ShortBranchTo encodes `B <to>` placed at address `from`. ok is false when the
// displacement does not fit in the 26-bit signed word offset.
func ShortBranchTo(from, to uintptr) (res []byte, ok bool) {
	delta := int64(to) - int64(from)
	if delta%4 != 0 || delta <= -ShortBranchRange || delta >= ShortBranchRange {
		return nil, false
	}
	imm26 := uint32((delta >> 2) & 0x03ffffff)
	enc := uint32(0b000101)<<26 | imm26
	res = make([]byte, 4)
	*(*uint32)(unsafe.Pointer(&res[0])) = enc
	return res, true
}
