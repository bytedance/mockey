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

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/x86/x86asm"
)

func calcFnAddrRange(name string, fn func()) (uintptr, uintptr) {
	v := reflect.ValueOf(fn)
	var start, end uintptr
	start = v.Pointer()
	maxScan := 2000
	code := common.BytesOf(v.Pointer(), 2000)
	pos := 0

	for pos < maxScan {
		inst, err := x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)

		args := []interface{}{name, inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}

		if inst.Op == x86asm.RET {
			end = start + uintptr(pos)
			return start, end
		}

		pos += int(inst.Len)
	}
	tool.Assert(false, "%v ret not found", name)
	return 0, 0
}

func Disassemble(code []byte, required int, checkLen bool) int {
	var pos int
	var err error
	var inst x86asm.Inst

	for pos < required {
		inst, err = x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)
		tool.DebugPrintf("Disassemble: %3d\t0x%x\t%v\n", pos, common.PtrOf(code)+uintptr(pos), inst)
		tool.Assert(inst.Op != x86asm.RET || !checkLen, "function is too short to patch")
		pos += inst.Len
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
		inst, err := x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)
		args := []interface{}{inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("GetGenericAddr: %3d\t0x%x\t%v\n", pos, addr+uintptr(pos), inst)

		switch inst.Op {
		case x86asm.CALL:
			allJumpInsts = append(allJumpInsts, newGenericJmpInst(addr, pos, inst))
		case x86asm.LEA:
			allInfoInsts = append(allInfoInsts, newGenericInfoInst(addr, pos, inst))
		case x86asm.RET:
			break loop
		}
		pos += inst.Len
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

	// Find the exact infoInst before the jumpInst
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
	inst x86asm.Inst
}

func (pi *posInst) String() string {
	return fmt.Sprintf("{pos: %d, addr: 0x%x, inst: %v}", pi.pos, pi.addr, pi.inst)
}

func newGenericJmpInst(base uintptr, pos int, inst x86asm.Inst) *genericJmpInst {
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
	return g.addr + uintptr(g.inst.Len) + uintptr(g.inst.Args[0].(x86asm.Rel))
}

func newGenericInfoInst(base uintptr, pos int, inst x86asm.Inst) *genericInfoInst {
	return &genericInfoInst{
		lea: &posInst{pos: pos, addr: base + uintptr(pos), inst: inst},
	}
}

type genericInfoInst struct {
	lea *posInst
}

func (g *genericInfoInst) String() string {
	return fmt.Sprintf("{lea: %v}", g.lea)
}

func (g *genericInfoInst) matchJumpInst(jumpInst *genericJmpInst) bool {
	mem, ok := g.lea.inst.Args[1].(x86asm.Mem)
	if !ok {
		return false
	}
	if mem.Base != x86asm.RIP {
		return false
	}
	return g.lea.pos+g.lea.inst.Len <= jumpInst.pos
}

// calcGenericInfoAddr calculates the genericInfo from the lea instructions. Example:
// LEA RBX, [RIP+0x145d07]
func (g *genericInfoInst) calcGenericInfoAddr() uintptr {
	return g.lea.addr + uintptr(g.lea.inst.Len) + uintptr(g.lea.inst.Args[1].(x86asm.Mem).Disp)
}

// GenericClosureCaptureOffset returns the last captured word loaded by a
// compiler-generated closure. The closure context register is DX on amd64.
func GenericClosureCaptureOffset(addr uintptr, maxScan int) uintptr {
	code := common.BytesOf(addr, maxScan)
	var offset uintptr
	for pos := 0; pos < maxScan; {
		instruction, err := x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)
		if instruction.Op == x86asm.RET {
			return offset
		}
		if instruction.Op == x86asm.CALL {
			jump := newGenericJmpInst(addr, pos, instruction)
			if !jump.isExtraCall {
				return offset
			}
		}
		if instruction.Op == x86asm.MOV {
			mem, ok := instruction.Args[1].(x86asm.Mem)
			if ok && mem.Base == x86asm.RDX && mem.Index == 0 && mem.Disp > 0 && uintptr(mem.Disp) > offset {
				offset = uintptr(mem.Disp)
			}
		}
		pos += instruction.Len
	}
	return offset
}

// RelocateBranches makes relative control flow in the copied prefix target
// local absolute-jump stubs. In particular, an Origin call can take the stack
// growth branch even when the original call that reached the hook did not.
func RelocateBranches(code []byte, original uintptr, prefixEnd, stubOffset int) {
	for pos := 0; pos < prefixEnd; {
		instruction, err := x86asm.Decode(code[pos:prefixEnd], 64)
		tool.Assert(err == nil, err)
		if instruction.Op == x86asm.RET {
			return
		}
		if instruction.Op == x86asm.LEA {
			if address, ok := instruction.Args[1].(x86asm.Mem); ok {
				// x86asm does not mark address-size-prefixed EIP references
				// as PC-relative. Copying one unchanged would use the proxy PC.
				tool.Assert(address.Base != x86asm.EIP, "cannot relocate EIP-relative LEA: %v", instruction)
			}
		}
		if relative, ok := instruction.Args[0].(x86asm.Rel); ok {
			targetOffset := pos + instruction.Len + int(relative)
			if targetOffset < 0 || targetOffset >= prefixEnd {
				stub := BranchToOriginal(original + uintptr(targetOffset))
				tool.Assert(stubOffset+len(stub) <= len(code), "trampoline branch stubs exceed page size")
				displacement := int64(stubOffset - pos - instruction.Len)
				switch instruction.PCRel {
				case 1:
					tool.Assert(displacement >= -128 && displacement <= 127, "short trampoline branch is out of range")
					code[pos+instruction.PCRelOff] = byte(int8(displacement))
				case 4:
					binary.LittleEndian.PutUint32(code[pos+instruction.PCRelOff:], uint32(int32(displacement)))
				default:
					tool.Assert(false, "unsupported relative branch: %v", instruction)
				}
				copy(code[stubOffset:], stub)
				stubOffset += len(stub)
			}
		} else if instruction.PCRel != 0 {
			// Address generation can load an absolute address from a nearby
			// literal when the trampoline is outside the original disp32 range.
			// Other data references must retain their signed displacement.
			tool.Assert(instruction.PCRel == 4, "unsupported PC-relative instruction: %v", instruction)
			old := int64(int32(binary.LittleEndian.Uint32(code[pos+instruction.PCRelOff:])))
			displacement := old + int64(original) - int64(common.PtrOf(code))
			if displacement != int64(int32(displacement)) && instruction.Op == x86asm.LEA {
				address, ok := instruction.Args[1].(x86asm.Mem)
				tool.Assert(ok && instruction.AddrSize == 64 && address.Base == x86asm.RIP && address.Segment == 0,
					"cannot relocate PC-relative LEA: %v", instruction)
				tool.Assert(stubOffset >= prefixEnd && stubOffset <= len(code)-8, "trampoline literals exceed page size")
				// LEA and MOV share ModRM and register/operand-size prefixes.
				// Replacing the opcode preserves flags and needs no scratch register.
				code[pos+instruction.PCRelOff-2] = 0x8b
				binary.LittleEndian.PutUint64(code[stubOffset:], uint64(original+uintptr(pos+instruction.Len)+uintptr(old)))
				displacement = int64(stubOffset - pos - instruction.Len)
				stubOffset += 8
			}
			tool.Assert(displacement == int64(int32(displacement)), "PC-relative data reference is out of trampoline range: %v", instruction)
			binary.LittleEndian.PutUint32(code[pos+instruction.PCRelOff:], uint32(int32(displacement)))
		}
		pos += instruction.Len
	}
}
