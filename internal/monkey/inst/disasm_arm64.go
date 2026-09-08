/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to analyze Go 1.27 generic method closures.
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
	"reflect"
	"unsafe"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/arm64/arm64asm"
)

const (
	instLen = 4 // arm64 instruction length is 4 bytes
)

func calcFnAddrRange(name string, fn func()) (uintptr, uintptr) {
	v := reflect.ValueOf(fn)
	var start, end uintptr
	start = v.Pointer()
	maxScan := 2000
	code := common.BytesOf(start, 2000)
	pos := 0
	for pos < maxScan {
		inst, err := arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)

		args := []interface{}{name, inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}

		if inst.Op == arm64asm.RET {
			end = start + uintptr(pos)
			return start, end
		}

		pos += int(unsafe.Sizeof(inst.Enc))
	}
	tool.Assert(false, "%v end not found", name)
	return 0, 0
}

func Disassemble(code []byte, required int, checkLen bool) int {
	var pos int
	var err error
	var inst arm64asm.Inst

	for pos < required {
		if pos+instLen > len(code) {
			tool.Assert(!checkLen, "function is too short to patch")
			// MockUnsafe explicitly permits overwriting past a short function.
			// Its caller will save those overwritten bytes, but instruction
			// analysis must stay within this function's metadata boundary.
			return (required + instLen - 1) / instLen * instLen
		}
		inst, err = arm64asm.Decode(code[pos:])
		tool.Assert(err == nil || !checkLen, err)
		tool.DebugPrintf("Disassemble: %3d\t0x%x\t%v\n", pos, common.PtrOf(code)+uintptr(pos), inst)
		tool.Assert(inst.Op != arm64asm.RET || !checkLen, "function is too short to patch")
		pos += instLen
	}
	prefixEnd := pos
	// Go 1.27 may start a stack-only leaf with a zeroing/copy loop rather
	// than a stack-split prologue. Include any backward branch into the
	// copied prefix, otherwise Origin jumps back into the patched entry.
	// The unconditional retry after morestack is not an initialization loop.
	for scan := pos; scan+instLen <= len(code); scan += instLen {
		instruction, decodeErr := arm64asm.Decode(code[scan:])
		if decodeErr != nil || instruction.Op == arm64asm.RET {
			break
		}
		conditional := instruction.Op == arm64asm.CBNZ || instruction.Op == arm64asm.CBZ || instruction.Op == arm64asm.TBNZ || instruction.Op == arm64asm.TBZ
		if instruction.Op == arm64asm.B {
			_, conditional = instruction.Args[0].(arm64asm.Cond)
		}
		if !conditional {
			continue
		}
		for _, arg := range instruction.Args {
			if relative, ok := arg.(arm64asm.PCRel); ok {
				destination := scan + int(relative)
				if destination >= 0 && destination < pos {
					pos = scan + instLen
				}
			}
		}
	}
	// Relative references in newly copied instructions must stay within the
	// copied block. Relocating calls and literal/address loads is not supported
	// by the trampoline; reject them rather than emitting an invalid Origin.
	for scan := prefixEnd; scan < pos; scan += instLen {
		instruction, decodeErr := arm64asm.Decode(code[scan:])
		tool.Assert(decodeErr == nil, decodeErr)
		for _, arg := range instruction.Args {
			if relative, ok := arg.(arm64asm.PCRel); ok {
				destination := scan + int(relative)
				branch := instruction.Op == arm64asm.B || instruction.Op == arm64asm.CBNZ || instruction.Op == arm64asm.CBZ || instruction.Op == arm64asm.TBNZ || instruction.Op == arm64asm.TBZ
				tool.Assert(branch && destination >= 0 && destination < pos,
					"cannot relocate instruction in initialization loop: %v", instruction)
			}
		}
	}
	return pos
}

func GetGenericAddr(addr uintptr, maxScan int) (jumpAddr, genericInfoAddr uintptr) {
	code := common.BytesOf(addr, maxScan)
	var (
		allJumpInsts []*genericJmpInst
		allInfoInsts []*genericInfoInst
		pos          int
	)
loop:
	for pos < maxScan {
		inst, err := arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)
		args := []interface{}{inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("GetGenericAddr: %3d\t0x%x\t%v\n", pos, addr+uintptr(pos), inst)

		switch inst.Op {
		case arm64asm.BL:
			allJumpInsts = append(allJumpInsts, newGenericJmpInst(addr, pos, inst))
		case arm64asm.ADRP:
			allInfoInsts = append(allInfoInsts, newGenericInfoInst(addr, pos, inst))
		case arm64asm.ADD:
			if len(allInfoInsts) > 0 {
				allInfoInsts[len(allInfoInsts)-1].putAddInst(pos, inst)
			}
		case arm64asm.RET:
			break loop
		}
		pos += instLen
	}

	var (
		jumpInst *genericJmpInst
		infoInst *genericInfoInst
	)
	// Find the only jumpInst and filter the extra call
	for _, cur := range allJumpInsts {
		if cur.isExtraCall {
			continue
		}
		tool.Assert(jumpInst == nil, "invalid jumpInsts: %v", allJumpInsts)
		jumpInst = cur
	}
	tool.Assert(jumpInst != nil, "invalid jumpInsts: %v", allJumpInsts)
	tool.DebugPrintf("jumpInst found: %v\n", jumpInst)

	// Find the latest infoInst before the jumpInst
	for _, cur := range allInfoInsts {
		if cur.matchJumpInst(jumpInst) {
			infoInst = cur
		}
	}
	// In some cases, genericInfoAddr needs to be calculated and cannot be directly obtained by analyzing instructions.
	if infoInst == nil {
		tool.DebugPrintf("infoInst not found!\n")
		return jumpInst.jumpAddr, 0
	}

	gi := infoInst.calcGenericInfoAddr()
	tool.DebugPrintf("infoInst found: %v, genericInfoAddr: 0x%x\n", infoInst, gi)
	return jumpInst.jumpAddr, gi
}

type posInst struct {
	pos  int
	addr uintptr
	inst arm64asm.Inst
}

func (pi *posInst) String() string {
	return fmt.Sprintf("{pos: %d, addr: 0x%x, inst: %v}", pi.pos, pi.addr, pi.inst)
}

func newGenericJmpInst(base uintptr, pos int, inst arm64asm.Inst) *genericJmpInst {
	ji := &genericJmpInst{
		posInst: &posInst{pos: pos, addr: base + uintptr(pos), inst: inst},
	}
	return ji.init()
}

type genericJmpInst struct {
	*posInst
	jumpAddr      uintptr
	isExtraCall   bool
	extraCallName string
}

func (g *genericJmpInst) String() string {
	return fmt.Sprintf("{posInst: %v, jumpAddr: 0x%x, isExtraCall: %v, extraCallName: %v}", g.posInst, g.jumpAddr, g.isExtraCall, g.extraCallName)
}

func (g *genericJmpInst) init() *genericJmpInst {
	g.jumpAddr = g.calcJumpAddr()
	g.isExtraCall, g.extraCallName = isGenericProxyCallExtra(g.jumpAddr)
	return g
}

func (g *genericJmpInst) calcJumpAddr() uintptr {
	return g.addr + uintptr(g.inst.Args[0].(arm64asm.PCRel))
}

func newGenericInfoInst(base uintptr, pos int, inst arm64asm.Inst) *genericInfoInst {
	return &genericInfoInst{
		adrp: &posInst{pos: pos, addr: base + uintptr(pos), inst: inst},
	}
}

type genericInfoInst struct {
	adrp *posInst
	add  *posInst
}

func (g *genericInfoInst) String() string {
	return fmt.Sprintf("{adrp: %v, add: %v}", g.adrp, g.add)
}

func (g *genericInfoInst) matchJumpInst(jumpInst *genericJmpInst) bool {
	return g.add != nil && g.add.pos < jumpInst.pos
}

func (g *genericInfoInst) putAddInst(pos int, inst arm64asm.Inst) {
	if g.adrp == nil || g.add != nil || g.adrp.pos+instLen != pos {
		return
	}
	base := g.adrp.addr - uintptr(g.adrp.pos)
	g.add = &posInst{pos: pos, addr: base + uintptr(pos), inst: inst}
}

// calcGenericInfoAddr calculates the genericInfo from the adrp and add instructions. Example:
// ADRP X0, .+0xb3000
// ADD X0, X0, #0xec0
func (g *genericInfoInst) calcGenericInfoAddr() uintptr {
	adrpInst := g.adrp.inst
	adrpReg := adrpInst.Args[0].(arm64asm.Reg)
	adrpRes := (g.adrp.addr &^ 0xFFF) + uintptr(adrpInst.Args[1].(arm64asm.PCRel))
	addInst := g.add.inst
	tool.Assert(addInst.Args[0].(arm64asm.RegSP) == arm64asm.RegSP(adrpReg), "invalid addInst: %v", addInst)
	tool.Assert(addInst.Args[1].(arm64asm.RegSP) == arm64asm.RegSP(adrpReg), "invalid addInst: %v", addInst)
	addImmShift0 := addInst.Args[2].(arm64asm.ImmShift)
	type immShift struct {
		imm   uint16
		shift uint8
	}
	addImmShift := (*immShift)(unsafe.Pointer(&addImmShift0))
	tool.Assert(addImmShift.shift == 0, "invalid addInst: %v", addInst)
	addRes := adrpRes + uintptr(addImmShift.imm)
	return addRes
}

// GenericClosureCaptureOffset returns the last captured word loaded by a
// compiler-generated closure. Go 1.27 captures a generic method's dictionary
// after its optional bound receiver. Only inspect the wrapper before RET.
func GenericClosureCaptureOffset(addr uintptr, maxScan int) uintptr {
	code := common.BytesOf(addr, maxScan)
	var offset uintptr
	for pos := 0; pos < maxScan; pos += instLen {
		instruction, err := arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)
		if instruction.Op == arm64asm.RET {
			return offset
		}
		if instruction.Op == arm64asm.BL {
			jump := newGenericJmpInst(addr, pos, instruction)
			if !jump.isExtraCall {
				return offset
			}
		}
		if instruction.Op != arm64asm.LDR && instruction.Op != arm64asm.LDUR {
			continue
		}
		mem, ok := instruction.Args[1].(arm64asm.MemImmediate)
		if !ok || mem.Base != arm64asm.RegSP(arm64asm.X26) || mem.Mode != arm64asm.AddrOffset {
			continue
		}
		// arm64asm deliberately keeps the displacement private.
		type memImmediate struct {
			base arm64asm.RegSP
			mode arm64asm.AddrMode
			imm  int32
		}
		displacement := (*memImmediate)(unsafe.Pointer(&mem)).imm
		if displacement > 0 && uintptr(displacement) > offset {
			offset = uintptr(displacement)
		}
	}
	return offset
}

// RelocateBranches preserves branch targets when a prologue is copied into
// the Origin trampoline. External branches use nearby absolute-jump stubs.
func RelocateBranches(code []byte, original uintptr, prefixEnd, stubOffset int) {
	for pos := 0; pos < prefixEnd; pos += instLen {
		instruction, err := arm64asm.Decode(code[pos:prefixEnd])
		tool.Assert(err == nil, err)
		if instruction.Op == arm64asm.RET {
			return
		}
		for _, argument := range instruction.Args {
			relative, ok := argument.(arm64asm.PCRel)
			if !ok {
				continue
			}
			targetOffset := pos + int(relative)
			if instruction.Op == arm64asm.ADR || instruction.Op == arm64asm.ADRP {
				// Replace address generation with a PC-relative load of the
				// absolute address. The literal stays close to the trampoline
				// even when mmap is more than 4 GiB from the original text.
				address := original + uintptr(targetOffset)
				if instruction.Op == arm64asm.ADRP {
					address = (original+uintptr(pos))&^0xfff + uintptr(relative)
				}
				stubOffset = (stubOffset + 7) &^ 7
				tool.Assert(stubOffset+8 <= len(code), "trampoline literals exceed page size")
				register := uint32(instruction.Args[0].(arm64asm.Reg) - arm64asm.X0)
				encoding := uint32(0x58000000) | uint32((stubOffset-pos)/instLen)<<5 | register
				binary.LittleEndian.PutUint32(code[pos:], encoding)
				binary.LittleEndian.PutUint64(code[stubOffset:], uint64(address))
				stubOffset += 8
				continue
			}
			var shift, width uint
			switch instruction.Op {
			case arm64asm.B:
				shift, width = 0, 26
				if _, conditional := instruction.Args[0].(arm64asm.Cond); conditional {
					shift, width = 5, 19
				}
			case arm64asm.BL:
				shift, width = 0, 26
			case arm64asm.CBZ, arm64asm.CBNZ:
				shift, width = 5, 19
			case arm64asm.TBZ, arm64asm.TBNZ:
				shift, width = 5, 14
			default:
				tool.Assert(false, "cannot relocate PC-relative data instruction: %v", instruction)
			}
			if targetOffset >= 0 && targetOffset < prefixEnd {
				continue
			}
			stub := BranchToOriginal(original + uintptr(targetOffset))
			tool.Assert(stubOffset+len(stub) <= len(code), "trampoline branch stubs exceed page size")
			displacement := int32((stubOffset - pos) / instLen)
			tool.Assert(displacement >= -(1<<(width-1)) && displacement < 1<<(width-1), "trampoline branch is out of range")
			mask := uint32((1<<width)-1) << shift
			encoding := instruction.Enc&^mask | (uint32(displacement) << shift & mask)
			binary.LittleEndian.PutUint32(code[pos:], encoding)
			copy(code[stubOffset:], stub)
			stubOffset += len(stub)
		}
	}
}
