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

import "encoding/binary"

// branchImm returns the position and width of the immediate field of a direct
// branch (B, B.cond, CBZ/CBNZ, TBZ/TBNZ). The immediate counts instructions.
// See https://developer.arm.com/documentation/ddi0596/2021-12/Index-by-Encoding/Branches--Exception-Generating-and-System-instructions
func branchImm(inst uint32) (shift, width uint, ok bool) {
	switch {
	case inst&0xfc000000 == 0x14000000: // B
		return 0, 26, true
	case inst&0xff000010 == 0x54000000: // B.cond
		return 5, 19, true
	case inst&0x7e000000 == 0x34000000: // CBZ, CBNZ
		return 5, 19, true
	case inst&0x7e000000 == 0x36000000: // TBZ, TBNZ
		return 5, 14, true
	}
	return 0, 0, false
}

// relocateBranches copies code and reports the direct branches that leave it.
// Instructions keep their size, so branches inside the code stay valid.
func relocateBranches(targetAddr uintptr, code []byte) ([]byte, []proxyBranch) {
	proxy := append([]byte(nil), code...)
	var branches []proxyBranch
	for pos := 0; pos+instLen <= len(code); pos += instLen {
		inst := binary.LittleEndian.Uint32(code[pos:])
		shift, width, ok := branchImm(inst)
		if !ok {
			continue
		}
		imm := int64(inst>>shift) << (64 - width) >> (64 - width)
		dest := int64(pos) + imm*instLen
		if dest >= 0 && dest < int64(len(code)) {
			continue
		}
		branches = append(branches, proxyBranch{pos: pos, target: targetAddr + uintptr(dest)})
	}
	return proxy, branches
}

// setBranchTarget makes the direct branch at proxy[pos:] jump to proxy[to:].
func setBranchTarget(proxy []byte, pos, to int) {
	inst := binary.LittleEndian.Uint32(proxy[pos:])
	shift, width, _ := branchImm(inst)
	mask := uint32(1)<<width - 1
	imm := uint32((to-pos)/instLen) & mask
	binary.LittleEndian.PutUint32(proxy[pos:], inst&^(mask<<shift)|imm<<shift)
}
