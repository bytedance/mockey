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

	"golang.org/x/arch/x86/x86asm"
)

// copiedInst is an instruction of the copied code. For a direct JMP or Jcc,
// dest is the offset of its destination relative to the copied code.
type copiedInst struct {
	pos, len int
	branch   bool
	dest     int
}

// isExternal reports whether the branch leaves the copied code.
func (inst copiedInst) isExternal(codeLen int) bool {
	return inst.dest < 0 || inst.dest >= codeLen
}

// directBranch reports whether code starts with a JMP or Jcc with an 8-bit or
// 32-bit displacement and no prefix.
func directBranch(code []byte, inst x86asm.Inst) bool {
	if _, ok := inst.Args[0].(x86asm.Rel); !ok {
		return false
	}
	switch inst.Len {
	case 2:
		return code[0] == 0xeb || code[0]&0xf0 == 0x70
	case 5:
		return code[0] == 0xe9
	case 6:
		return code[0] == 0x0f && code[1]&0xf0 == 0x80
	}
	return false
}

// relocateBranches copies code and reports the direct branches that leave it.
// Those branches are rewritten with a 32-bit displacement, so branches inside
// the code are adjusted for the instructions that grew.
func relocateBranches(targetAddr uintptr, code []byte) ([]byte, []proxyBranch) {
	var insts []copiedInst
	for pos := 0; pos < len(code); {
		inst, err := x86asm.Decode(code[pos:], 64)
		if err != nil {
			// Keep the remaining bytes as they are, as before.
			insts = append(insts, copiedInst{pos: pos, len: len(code) - pos})
			break
		}
		ci := copiedInst{pos: pos, len: inst.Len}
		if directBranch(code[pos:], inst) {
			ci.branch = true
			ci.dest = pos + inst.Len + int(inst.Args[0].(x86asm.Rel))
		}
		insts = append(insts, ci)
		pos += inst.Len
	}

	newPos := make(map[int]int, len(insts)+1)
	size := 0
	for _, inst := range insts {
		newPos[inst.pos] = size
		size += relocatedLen(code[inst.pos:], inst, len(code))
	}
	newPos[len(code)] = size

	proxy := make([]byte, 0, size)
	var branches []proxyBranch
	for _, inst := range insts {
		src := code[inst.pos : inst.pos+inst.len]
		switch {
		case !inst.branch:
			proxy = append(proxy, src...)
		case inst.isExternal(len(code)):
			branches = append(branches, proxyBranch{pos: len(proxy), target: targetAddr + uintptr(inst.dest)})
			if src[0] == 0xeb || src[0] == 0xe9 {
				proxy = append(proxy, 0xe9, 0, 0, 0, 0) // JMP rel32
			} else {
				proxy = append(proxy, 0x0f, 0x80|jccCond(src), 0, 0, 0, 0) // Jcc rel32
			}
		default:
			dest, ok := newPos[inst.dest]
			if !ok {
				// The destination is not an instruction boundary: keep the bytes.
				proxy = append(proxy, src...)
				break
			}
			start := len(proxy)
			proxy = append(proxy, src...)
			rel := dest - (start + inst.len)
			if inst.len == 2 {
				proxy[start+1] = byte(int8(rel))
			} else {
				binary.LittleEndian.PutUint32(proxy[start+inst.len-4:], uint32(int32(rel)))
			}
		}
	}
	return proxy, branches
}

// relocatedLen returns the size of inst in the proxy.
func relocatedLen(code []byte, inst copiedInst, codeLen int) int {
	if !inst.branch || !inst.isExternal(codeLen) || inst.len != 2 {
		return inst.len
	}
	if code[0] == 0xeb {
		return 5 // JMP rel8 -> JMP rel32
	}
	return 6 // Jcc rel8 -> Jcc rel32
}

// jccCond returns the condition code of a Jcc.
func jccCond(code []byte) byte {
	if code[0] == 0x0f {
		return code[1] & 0x0f
	}
	return code[0] & 0x0f
}

// setBranchTarget makes the JMP rel32 or Jcc rel32 at proxy[pos:] jump to proxy[to:].
func setBranchTarget(proxy []byte, pos, to int) {
	n := 5
	if proxy[pos] == 0x0f {
		n = 6
	}
	binary.LittleEndian.PutUint32(proxy[pos+n-4:], uint32(int32(to-(pos+n))))
}
