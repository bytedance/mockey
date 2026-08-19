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

// RelocateFirstInst returns the encoding of the instruction at `code` rewritten
// so that, when placed at address `to`, it behaves as it did at address `from`.
//
// Supported today:
//   - non-PC-relative instructions: copied verbatim
//   - ADRP Xd, <label>: the 21-bit page offset is recomputed for the new page.
//     ADR is not rewritten (its +/-1MB reach is too small to survive the move).
//
// ok is false when the instruction cannot be safely relocated.
func RelocateFirstInst(code []byte, from, to uintptr) (out []byte, ok bool, why string) {
	if len(code) < 4 {
		return nil, false, "truncated"
	}
	enc := *(*uint32)(unsafe.Pointer(&code[0]))

	isAdrFamily := enc&0x1f000000 == 0x10000000
	if isAdrFamily {
		isAdrp := enc&0x80000000 != 0
		if !isAdrp {
			return nil, false, "ADR (+/-1MB reach too small to relocate)"
		}
		// ADRP computes: (PC & ~0xfff) + (imm << 12)
		immLo := (enc >> 29) & 0x3
		immHi := (enc >> 5) & 0x7ffff
		imm := int64(immHi<<2 | immLo)
		if imm&(1<<20) != 0 { // sign extend 21 bits
			imm -= 1 << 21
		}
		target := (int64(from) &^ 0xfff) + (imm << 12)
		newImm := (target - (int64(to) &^ 0xfff)) >> 12
		if newImm < -(1<<20) || newImm >= (1<<20) {
			return nil, false, "ADRP target out of range from the new page"
		}
		u := uint32(newImm) & 0x1fffff
		enc = enc&^(0x3<<29) | (u&0x3)<<29
		enc = enc&^(0x7ffff<<5) | ((u>>2)&0x7ffff)<<5
		res := make([]byte, 4)
		*(*uint32)(unsafe.Pointer(&res[0])) = enc
		return res, true, "ADRP relocated"
	}

	if relOK, why := RelocatableFirstInst(code); !relOK {
		return nil, false, why
	}
	res := make([]byte, 4)
	copy(res, code[:4])
	return res, true, "verbatim"
}

// RelocatableFirstInst reports whether the first instruction at `code` can be
// copied verbatim to another address. PC-relative instructions cannot: moving
// them changes what they compute.
//
// Rather than decode operand semantics, this checks the encoding directly for
// the PC-relative families on arm64:
//
//	ADR/ADRP        op[31] x 1_0000 ...      -> bits 28..24 == 10000
//	B/BL <label>    000101 / 100101 imm26
//	B.cond / CBZ / CBNZ / TBZ / TBNZ         -> bits 30..25 == 011010 / 011011
//	LDR literal     bits 29..24 == 011000
func RelocatableFirstInst(code []byte) (ok bool, op string) {
	if len(code) < 4 {
		return false, "truncated"
	}
	enc := *(*uint32)(unsafe.Pointer(&code[0]))

	if enc&0x1f000000 == 0x10000000 { // ADR / ADRP
		return false, "ADR/ADRP"
	}
	if enc&0x7c000000 == 0x14000000 { // B / BL <label>
		return false, "B/BL"
	}
	if enc&0x7e000000 == 0x34000000 { // CBZ / CBNZ
		return false, "CBZ/CBNZ"
	}
	if enc&0x7e000000 == 0x36000000 { // TBZ / TBNZ
		return false, "TBZ/TBNZ"
	}
	if enc&0xff000010 == 0x54000000 { // B.cond
		return false, "B.cond"
	}
	if enc&0x3b000000 == 0x18000000 { // LDR (literal)
		return false, "LDR-literal"
	}

	// decode for a readable name; failure to decode is itself disqualifying
	inst, err := arm64asm.Decode(code)
	if err != nil {
		return false, "undecodable"
	}
	return true, inst.Op.String()
}
