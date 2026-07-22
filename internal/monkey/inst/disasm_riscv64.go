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
	"fmt"
	"reflect"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/riscv64/riscv64asm"
)

func calcFnAddrRange(name string, fn func()) (uintptr, uintptr) {
	v := reflect.ValueOf(fn)
	var start, end uintptr
	start = v.Pointer()
	maxScan := 2000
	code := common.BytesOf(start, maxScan)
	pos := 0
	for pos < maxScan {
		inst, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)

		if isRet(inst) {
			end = start + uintptr(pos)
			return start, end
		}

		pos += inst.Len
	}
	tool.Assert(false, "%v end not found", name)
	return 0, 0
}

func Disassemble(code []byte, required int, checkLen bool) int {
	var scan reachableCodeScan
	pos, maxEnd := 0, 0
	for {
		inst, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil || !checkLen, err)
		if err != nil || inst.Len <= 0 {
			return max(maxEnd, pos)
		}
		tool.DebugPrintf("Disassemble: %3d\t0x%x\t%v\n", pos, common.PtrOf(code)+uintptr(pos), inst)
		maxEnd = max(maxEnd, pos+inst.Len)
		if target, ok := scan.observe(pos, inst, code[pos:]); ok && target > required {
			required = target
		}
		nextPos, stop := scan.advance(pos, inst, required)
		if stop {
			if maxEnd < required {
				tool.Assert(!isRet(inst) || !checkLen, "function is too short to patch")
				return required
			}
			return maxEnd
		}
		pos = nextPos
		if pos >= required {
			return max(maxEnd, pos)
		}
	}
}

func conditionalBranchTarget(pos int, inst riscv64asm.Inst) (int, bool) {
	switch inst.Op {
	case riscv64asm.BEQ, riscv64asm.BNE, riscv64asm.BLT, riscv64asm.BGE, riscv64asm.BLTU, riscv64asm.BGEU:
		return pos + int(inst.Args[2].(riscv64asm.Simm).Imm), true
	default:
		return 0, false
	}
}

type reachableTarget struct {
	offset        int
	allowBoundary bool
}

type reachableCodeScan struct {
	visited map[int]struct{}
	pending []reachableTarget
}

func (scan *reachableCodeScan) observe(pos int, inst riscv64asm.Inst, encoding []byte) (int, bool) {
	if scan.visited == nil {
		scan.visited = make(map[int]struct{})
	}
	scan.visited[pos] = struct{}{}

	if target, ok := conditionalBranchTarget(pos, inst); ok {
		scan.enqueue(target, true)
		return target, target > pos
	}
	if inst.Op == riscv64asm.JAL {
		target := pos + int(jalImmediate(encoding, inst))
		rd := inst.Args[0].(riscv64asm.Reg)
		scan.enqueue(target, rd == riscv64asm.X0)
	}
	return 0, false
}

func (scan *reachableCodeScan) advance(pos int, inst riscv64asm.Inst, copiedLimit int) (next int, stop bool) {
	end := pos + inst.Len
	if instructionHasFallthrough(inst) && end < copiedLimit && !scan.wasVisited(end) {
		return end, false
	}
	if target, ok := scan.takePending(copiedLimit); ok {
		return target, false
	}
	return end, true
}

func (scan *reachableCodeScan) enqueue(target int, allowBoundary bool) {
	if target < 0 || scan.wasVisited(target) {
		return
	}
	for index, pending := range scan.pending {
		if pending.offset == target {
			scan.pending[index].allowBoundary = pending.allowBoundary || allowBoundary
			return
		}
	}
	scan.pending = append(scan.pending, reachableTarget{
		offset:        target,
		allowBoundary: allowBoundary,
	})
}

func (scan *reachableCodeScan) takePending(copiedLimit int) (int, bool) {
	index := -1
	target := int(^uint(0) >> 1)
	for currentIndex, current := range scan.pending {
		if current.offset > copiedLimit ||
			current.offset == copiedLimit && !current.allowBoundary ||
			scan.wasVisited(current.offset) ||
			current.offset >= target {
			continue
		}
		index = currentIndex
		target = current.offset
	}
	if index < 0 {
		return 0, false
	}
	scan.pending = append(scan.pending[:index], scan.pending[index+1:]...)
	return target, true
}

func (scan *reachableCodeScan) wasVisited(pos int) bool {
	_, ok := scan.visited[pos]
	return ok
}

func instructionHasFallthrough(inst riscv64asm.Inst) bool {
	if isRet(inst) || isIndirectJump(inst) {
		return false
	}
	return inst.Op != riscv64asm.JAL || inst.Args[0].(riscv64asm.Reg) != riscv64asm.X0
}

func jalImmediate(encoding []byte, inst riscv64asm.Inst) int32 {
	if inst.Len == 2 {
		return decodeCJOffset(encoding)
	}
	return int32(inst.Args[1].(riscv64asm.Simm).Imm)
}

func GetGenericAddr(addr uintptr, maxScan int) (jumpAddr, genericInfoAddr uintptr) {
	code := common.BytesOf(addr, maxScan)
	var (
		allJumpInsts []*genericJmpInst
		allInfoInsts []*genericInfoInst
		auipcByReg   = map[riscv64asm.Reg]*posInst{}
		pos          int
	)
loop:
	for pos < maxScan {
		remaining := code[pos:]
		// maxScan is a byte limit and may bisect the final 32-bit instruction.
		// Stop at that boundary after retaining every complete call already seen.
		if len(remaining) < 2 || (remaining[0]&3 == 3 && len(remaining) < 4) {
			break
		}
		inst, err := riscv64asm.Decode(remaining)
		tool.Assert(err == nil, err)
		tool.DebugPrintf("GetGenericAddr: %3d\t0x%x\t%v\n", pos, addr+uintptr(pos), inst)

		switch {
		case isRet(inst):
			break loop
		case inst.Op == riscv64asm.AUIPC:
			pi := &posInst{pos: pos, addr: addr + uintptr(pos), inst: inst}
			if reg, ok := auipcDst(inst); ok {
				auipcByReg[reg] = pi
			}
			allInfoInsts = append(allInfoInsts, newGenericInfoInst(pi))
		case inst.Op == riscv64asm.ADDI:
			if len(allInfoInsts) > 0 {
				allInfoInsts[len(allInfoInsts)-1].putAddInst(pos, addr+uintptr(pos), inst)
			}
		case isCall(inst):
			allJumpInsts = append(allJumpInsts, newGenericJmpInst(addr, pos, inst, auipcByReg))
		}
		pos += inst.Len
	}

	var (
		jumpInst *genericJmpInst
		infoInst *genericInfoInst
	)
	for _, cur := range allJumpInsts {
		if cur.isExtraCall {
			continue
		}
		tool.Assert(jumpInst == nil, "invalid jumpInsts: %v", allJumpInsts)
		jumpInst = cur
	}
	tool.Assert(jumpInst != nil, "invalid jumpInsts: %v", allJumpInsts)
	tool.DebugPrintf("jumpInst found: %v\n", jumpInst)

	for _, cur := range allInfoInsts {
		if cur.matchJumpInst(jumpInst) {
			infoInst = cur
		}
	}
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
	inst riscv64asm.Inst
}

func (pi *posInst) String() string {
	return fmt.Sprintf("{pos: %d, addr: 0x%x, inst: %v}", pi.pos, pi.addr, pi.inst)
}

func newGenericJmpInst(base uintptr, pos int, inst riscv64asm.Inst, auipcByReg map[riscv64asm.Reg]*posInst) *genericJmpInst {
	ji := &genericJmpInst{
		posInst: &posInst{pos: pos, addr: base + uintptr(pos), inst: inst},
	}
	return ji.init(auipcByReg)
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

func (g *genericJmpInst) init(auipcByReg map[riscv64asm.Reg]*posInst) *genericJmpInst {
	g.jumpAddr = g.calcJumpAddr(auipcByReg)
	// Linker-generated trampolines are common for runtime helpers on RISC-V.
	// Classify the resolved destination, but keep jumpAddr pointing at the
	// original call target so the generic business call is returned unchanged.
	resolvedAddr := resolvePCRelativeJump(g.jumpAddr)
	g.isExtraCall, g.extraCallName = isGenericProxyCallExtra(resolvedAddr)
	return g
}

func resolvePCRelativeJump(target uintptr) uintptr {
	code := common.BytesOf(target, 8)
	high, err := riscv64asm.Decode(code)
	if err != nil || high.Op != riscv64asm.AUIPC {
		return target
	}
	rd := high.Args[0].(riscv64asm.Reg)
	low, err := riscv64asm.Decode(code[high.Len:])
	if err != nil || low.Op != riscv64asm.JALR {
		return target
	}
	if low.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		return target
	}
	regOffset := low.Args[1].(riscv64asm.RegOffset)
	if regOffset.OfsReg != rd {
		return target
	}
	return uintptr(int64(target) + auipcImm(high) + int64(regOffset.Ofs.Imm))
}

func (g *genericJmpInst) calcJumpAddr(auipcByReg map[riscv64asm.Reg]*posInst) uintptr {
	if g.inst.Op == riscv64asm.JAL {
		var offset int64
		if g.inst.Len == 2 {
			offset = int64(decodeCJOffset(common.BytesOf(g.addr, g.inst.Len)))
		} else {
			offset = int64(g.inst.Args[1].(riscv64asm.Simm).Imm)
		}
		return uintptr(int64(g.addr) + offset)
	}
	regOffset := g.inst.Args[1].(riscv64asm.RegOffset)
	if auipc, ok := auipcByReg[regOffset.OfsReg]; ok {
		return uintptr(int64(auipc.addr) + auipcImm(auipc.inst) + int64(regOffset.Ofs.Imm))
	}
	tool.Assert(false, "missing AUIPC for jump: %v", g)
	return 0
}

func newGenericInfoInst(auipc *posInst) *genericInfoInst {
	return &genericInfoInst{auipc: auipc}
}

type genericInfoInst struct {
	auipc *posInst
	addi  *posInst
}

func (g *genericInfoInst) String() string {
	return fmt.Sprintf("{auipc: %v, addi: %v}", g.auipc, g.addi)
}

func (g *genericInfoInst) matchJumpInst(jumpInst *genericJmpInst) bool {
	return g.addi != nil && g.addi.pos < jumpInst.pos
}

func (g *genericInfoInst) putAddInst(pos int, addr uintptr, inst riscv64asm.Inst) {
	if g.auipc == nil || g.addi != nil || g.auipc.pos+g.auipc.inst.Len != pos {
		return
	}
	auipcReg, ok := auipcDst(g.auipc.inst)
	if !ok || inst.Args[0].(riscv64asm.Reg) != auipcReg || inst.Args[1].(riscv64asm.Reg) != auipcReg {
		return
	}
	g.addi = &posInst{pos: pos, addr: addr, inst: inst}
}

func (g *genericInfoInst) calcGenericInfoAddr() uintptr {
	return uintptr(int64(g.auipc.addr) + auipcImm(g.auipc.inst) + int64(g.addi.inst.Args[2].(riscv64asm.Simm).Imm))
}

func isRet(inst riscv64asm.Inst) bool {
	if inst.Op != riscv64asm.JALR {
		return false
	}
	if inst.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		return false
	}
	regOffset := inst.Args[1].(riscv64asm.RegOffset)
	return regOffset.OfsReg == riscv64asm.X1 && regOffset.Ofs.Imm == 0
}

func isIndirectJump(inst riscv64asm.Inst) bool {
	return inst.Op == riscv64asm.JALR && inst.Args[0].(riscv64asm.Reg) == riscv64asm.X0 && !isRet(inst)
}

// isCall is intentionally X1-only: GetGenericAddr wants the generic wrapper
// business call and must ignore compiler-inserted X5 morestack calls.
func isCall(inst riscv64asm.Inst) bool {
	switch inst.Op {
	case riscv64asm.JAL:
		return inst.Args[0].(riscv64asm.Reg) == riscv64asm.X1
	case riscv64asm.JALR:
		return inst.Args[0].(riscv64asm.Reg) == riscv64asm.X1
	default:
		return false
	}
}

func auipcDst(inst riscv64asm.Inst) (riscv64asm.Reg, bool) {
	if inst.Op != riscv64asm.AUIPC {
		return 0, false
	}
	return inst.Args[0].(riscv64asm.Reg), true
}

func auipcImm(inst riscv64asm.Inst) int64 {
	imm := int64(inst.Args[1].(riscv64asm.Uimm).Imm)
	if imm&(1<<19) != 0 {
		imm |= ^int64((1 << 20) - 1)
	}
	return imm << 12
}
