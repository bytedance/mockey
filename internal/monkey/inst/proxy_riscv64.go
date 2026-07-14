//go:build linux && riscv64
// +build linux,riscv64

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
	"sort"
	"syscall"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/riscv64/riscv64asm"
)

const (
	riscv64EntryPatchSize  = 8
	riscv64InitialCopySize = 24
	riscv64GatewaySize     = 8
	riscv64TailSize        = 24
	riscv64HookSize        = 24
	riscv64AUIPCVeneerSize = 24
	riscv64JumpVeneerSize  = 24
	riscv64CallVeneerSize  = 40
	riscv64IndirectSize    = 12
	riscv64SameRegSize     = 16
	riscv64DispatcherSize  = 24

	mmapFixedNoReplace = 0x100000
	proxySearchStep    = uintptr(64 << 20)
)

var (
	runtimeMorestackAddr       uintptr
	runtimeMorestackNoctxtAddr uintptr
)

func MaxPatchSize() int {
	return common.PageSize()
}

// AllocateProxy reserves an executable-proxy page within AUIPC/JALR range of
// the target. Keeping the page close lets static target PCs serve as safe
// stack-map landing points while execution resumes in the proxy.
func AllocateProxy(targetAddr uintptr) []byte {
	pageSize := uintptr(common.PageSize())
	targetPage := targetAddr &^ (pageSize - 1)

	// A non-fixed hint works on kernels that honor nearby mmap hints.
	for distance := proxySearchStep; distance < 1<<31; distance += proxySearchStep {
		for _, hint := range proxyCandidates(targetPage, distance, pageSize) {
			if page, ok := mmapProxyPage(hint, pageSize, false, targetAddr); ok {
				return page
			}
		}
	}

	// MAP_FIXED_NOREPLACE guarantees proximity without replacing an existing
	// mapping. Search at base-page granularity so many concurrent patches near
	// the same text mapping do not exhaust a sparse set of hints. Older kernels
	// may reject or ignore the flag; returned addresses are range-checked.
	for distance := pageSize; distance < 1<<31; distance += pageSize {
		for _, hint := range proxyCandidates(targetPage, distance, pageSize) {
			if page, ok := mmapProxyPage(hint, pageSize, true, targetAddr); ok {
				return page
			}
		}
	}

	tool.Assert(false, "cannot allocate a RISC-V proxy page within AUIPC/JALR range of %#x", targetAddr)
	return nil
}

func ReleaseProxy(proxy []byte) {
	_, _, errno := syscall.RawSyscall(
		syscall.SYS_MUNMAP,
		common.PtrOf(proxy),
		uintptr(len(proxy)),
		0,
	)
	tool.Assert(errno == 0, "free proxy page failed: %v", errno)
}

func proxyCandidates(base, distance, pageSize uintptr) []uintptr {
	candidates := make([]uintptr, 0, 2)
	if base <= ^uintptr(0)-distance {
		candidates = append(candidates, (base+distance)&^(pageSize-1))
	}
	if base > distance {
		candidates = append(candidates, (base-distance)&^(pageSize-1))
	}
	return candidates
}

func addressRangeEnd(start, size uintptr) (uintptr, bool) {
	if size == 0 || start > ^uintptr(0)-(size-1) {
		return 0, false
	}
	return start + size - 1, true
}

func proxyMappingReachable(targetAddr, targetSize, proxyAddr, proxySize uintptr) bool {
	targetEnd, targetOK := addressRangeEnd(targetAddr, targetSize)
	proxyEnd, proxyOK := addressRangeEnd(proxyAddr, proxySize)
	if !targetOK || !proxyOK {
		return false
	}
	for _, from := range []uintptr{targetAddr, targetEnd} {
		for _, to := range []uintptr{proxyAddr, proxyEnd} {
			_, _, forward := pcRelativeParts(from, to)
			_, _, reverse := pcRelativeParts(to, from)
			if !forward || !reverse {
				return false
			}
		}
	}
	return true
}

func mmapProxyPage(hint, size uintptr, fixed bool, targetAddr uintptr) ([]byte, bool) {
	flags := uintptr(syscall.MAP_ANON | syscall.MAP_PRIVATE)
	if fixed {
		flags |= mmapFixedNoReplace
	}
	addr, _, errno := syscall.RawSyscall6(
		syscall.SYS_MMAP,
		hint,
		size,
		uintptr(syscall.PROT_READ|syscall.PROT_WRITE),
		flags,
		^uintptr(0),
		0,
	)
	if errno != 0 {
		return nil, false
	}
	if !proxyMappingReachable(targetAddr, uintptr(MaxPatchSize()), addr, size) {
		_, _, _ = syscall.RawSyscall(syscall.SYS_MUNMAP, addr, size, 0)
		return nil, false
	}
	return common.BytesOf(addr, int(size)), true
}

// PatchSize returns enough whole original instructions for the entry jump,
// every static return landing, and the target-side gateways used by calls in
// the copied prefix.
func PatchSize(targetAddr uintptr, code []byte, checkLen bool) int {
	cuttingIdx := Disassemble(code, riscv64InitialCopySize, checkLen)
	tool.Assert(
		cuttingIdx >= riscv64InitialCopySize,
		"RISC-V patch scan did not reach the initial prefix at %#x: got %d, need %d",
		targetAddr,
		cuttingIdx,
		riscv64InitialCopySize,
	)
	for {
		required := cuttingIdx
		if boundary := extendTMPSequence(code, cuttingIdx); boundary > required {
			required = boundary
		}
		landings := collectReturnLandings(code[:cuttingIdx])
		assertCompressedGatewayLayout(landings)
		for _, landing := range landings {
			if landing.end > required {
				required = landing.end
			}
		}
		if required <= cuttingIdx {
			assertProxyLayout(code[:cuttingIdx], true)
			if _, ok := makePatchPlan(code[:cuttingIdx]); ok {
				return cuttingIdx
			}
			required = cuttingIdx + riscv64GatewaySize
		}
		tool.Assert(
			required <= len(code) && required <= MaxPatchSize(),
			"RISC-V patch prefix is too large at %#x: need at least %d bytes",
			targetAddr,
			required,
		)
		nextCuttingIdx := Disassemble(code, required, checkLen)
		tool.Assert(
			nextCuttingIdx >= required && nextCuttingIdx > cuttingIdx,
			"RISC-V patch scan made no progress at %#x: %d -> %d (need %d)",
			targetAddr,
			cuttingIdx,
			nextCuttingIdx,
			required,
		)
		cuttingIdx = nextCuttingIdx
		tool.Assert(
			cuttingIdx <= MaxPatchSize(),
			"RISC-V patch prefix is too large at %#x: %d",
			targetAddr,
			cuttingIdx,
		)
	}
}

// BuildPatch creates both the relocated origin proxy and the bytes installed
// at the real target. Return landings preserve the static target PC in X1/X5
// and use target-side gateways to resume at the matching proxy offset.
func BuildPatch(targetAddr, proxyAddr, hookAddr uintptr, code []byte) (proxy, targetPatch []byte) {
	assertProxyLayout(code, true)
	assertCompressedGatewayLayout(collectReturnLandings(code))
	plan, ok := makePatchPlan(code)
	tool.Assert(ok, "cannot place RISC-V return gateways in a %d-byte patch", len(code))

	targetPatch = append([]byte(nil), code...)
	proxy = buildProxy(targetAddr, proxyAddr, code, targetPatch)

	for _, linkReg := range plan.linkRegisters() {
		proxy = alignVeneer(proxy)
		dispatcherOffset := len(proxy)
		proxy = appendReturnDispatcher(proxy, linkReg, proxyAddr-targetAddr)
		gateway := plan.gateways[linkReg]
		copy(
			targetPatch[gateway.start:gateway.end],
			pcRelativeJump(
				targetAddr+uintptr(gateway.start),
				proxyAddr+uintptr(dispatcherOffset),
			),
		)
	}
	for _, landing := range plan.landings {
		gateway := plan.gateways[landing.linkReg]
		copy(
			targetPatch[landing.start:landing.end],
			shortJump(
				landing.size(),
				targetAddr+uintptr(landing.start),
				targetAddr+uintptr(gateway.start),
			),
		)
	}

	proxy = alignVeneer(proxy)
	hookOffset := len(proxy)
	proxy = appendHookTrampoline(proxy, hookAddr)
	assertProxyCapacity(len(proxy))
	copy(
		targetPatch[:riscv64EntryPatchSize],
		pcRelativeJump(targetAddr, proxyAddr+uintptr(hookOffset)),
	)
	return proxy, targetPatch
}

// BuildProxy is kept as the relocation-only entry point used by focused unit
// tests. External calls require BuildPatch because they also need a static
// return landing in the target.
func BuildProxy(targetAddr, proxyAddr uintptr, code []byte) []byte {
	return buildProxy(targetAddr, proxyAddr, code, nil)
}

// RISC-V PC-relative instructions cannot be copied to an mmap'ed proxy
// verbatim because the proxy is generally far from .text.
func buildProxy(targetAddr, proxyAddr uintptr, code, targetPatch []byte) []byte {
	assertProxyLayout(code, targetPatch != nil)
	proxy := append([]byte(nil), code...)
	proxy = alignVeneer(proxy)
	proxy = append(proxy, BranchTo(targetAddr+uintptr(len(code)))...)

	var scan reachableCodeScan
	for pos := 0; pos < len(code); {
		instruction, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil, "decode proxy instruction at +%d: %v", pos, err)
		scan.observe(pos, instruction, code[pos:])

		switch instruction.Op {
		case riscv64asm.AUIPC:
			rd := instruction.Args[0].(riscv64asm.Reg)
			if rd != riscv64asm.X0 {
				value := relocatedAUIPCValue(code, pos, instruction, targetAddr, proxyAddr)
				var veneerOffset int
				proxy, veneerOffset = appendAUIPCVeneer(
					proxy,
					proxyAddr,
					int(rd),
					value,
					proxyAddr+uintptr(pos+instruction.Len),
				)
				copy(
					proxy[pos:pos+instruction.Len],
					jal(riscv64Zero, checkedJALOffset(proxyAddr+uintptr(pos), proxyAddr+uintptr(veneerOffset))),
				)
			}
		case riscv64asm.JAL:
			jumpOffset := jalImmediate(code[pos:], instruction)
			targetOffset := pos + int(jumpOffset)
			rd := instruction.Args[0].(riscv64asm.Reg)
			if rd != riscv64asm.X0 || isExternalJAL(targetOffset, len(code), rd) {
				originalTarget := addSigned(targetAddr+uintptr(pos), int64(jumpOffset))
				internalTarget := targetOffset >= 0 && targetOffset < len(code)
				calleeTarget := originalTarget
				if internalTarget {
					calleeTarget = proxyAddr + uintptr(targetOffset)
				}
				proxy = alignVeneer(proxy)
				veneerOffset := len(proxy)
				switch rd {
				case riscv64asm.X0:
					proxy = appendAbsoluteJumpVeneer(proxy, calleeTarget)
				case riscv64asm.X1, riscv64asm.X5:
					tool.Assert(targetPatch != nil, "call relocation requires a target return landing")
					if rd == riscv64asm.X5 && !internalTarget {
						tool.Assert(
							isRuntimeMorestack(originalTarget),
							"cannot relocate an external X5 call at +%d to %#x",
							pos,
							originalTarget,
						)
					}
					returnPos := pos + instruction.Len
					proxy = appendCallVeneer(
						proxy,
						int(rd),
						targetAddr+uintptr(returnPos),
						calleeTarget,
					)
				default:
					tool.Assert(
						false,
						"cannot safely relocate a call using %v at +%d",
						rd,
						pos,
					)
				}
				// A two-byte C.J cannot be expanded in place. shortJump rejects a
				// veneer beyond its architectural +/-2 KiB range explicitly.
				copy(
					proxy[pos:pos+instruction.Len],
					shortJump(
						instruction.Len,
						proxyAddr+uintptr(pos),
						proxyAddr+uintptr(veneerOffset),
					),
				)
			}
		case riscv64asm.JALR:
			rd := instruction.Args[0].(riscv64asm.Reg)
			switch rd {
			case riscv64asm.X0:
				// Returns and indirect tail jumps keep their existing link state.
			case riscv64asm.X1, riscv64asm.X5:
				tool.Assert(targetPatch != nil, "indirect call relocation requires a target return landing")
				regOffset := instruction.Args[1].(riscv64asm.RegOffset)
				proxy = alignVeneer(proxy)
				veneerOffset := len(proxy)
				proxy = appendIndirectCallVeneer(
					proxy,
					proxyAddr,
					int(rd),
					int(regOffset.OfsReg),
					int32(regOffset.Ofs.Imm),
					targetAddr+uintptr(pos+instruction.Len),
				)
				copy(
					proxy[pos:pos+instruction.Len],
					shortJump(
						instruction.Len,
						proxyAddr+uintptr(pos),
						proxyAddr+uintptr(veneerOffset),
					),
				)
			default:
				tool.Assert(false, "cannot safely relocate an indirect call using %v at +%d", rd, pos)
			}
		case riscv64asm.BEQ, riscv64asm.BNE, riscv64asm.BLT, riscv64asm.BGE, riscv64asm.BLTU, riscv64asm.BGEU:
			targetOffset := pos + int(instruction.Args[2].(riscv64asm.Simm).Imm)
			tool.Assert(
				targetOffset >= 0 && targetOffset <= len(code),
				"cannot relocate external conditional branch at +%d to target offset %d",
				pos,
				targetOffset,
			)
		}

		nextPos, stop := scan.advance(pos, instruction, len(code))
		if stop {
			break
		}
		pos = nextPos
	}
	assertProxyCapacity(len(proxy))
	return proxy
}

func alignedProxyOffset(size int) int {
	return (size + 7) &^ 7
}

func assertProxyLayout(code []byte, patched bool) {
	proxySize := alignedProxyOffset(len(code)) + riscv64TailSize
	assertProxyCapacity(proxySize)

	var scan reachableCodeScan
	for pos := 0; pos < len(code); {
		instruction, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil, "decode proxy layout at +%d: %v", pos, err)
		scan.observe(pos, instruction, code[pos:])

		switch instruction.Op {
		case riscv64asm.AUIPC:
			if instruction.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
				proxySize = alignedProxyOffset(proxySize) + riscv64AUIPCVeneerSize
			}
		case riscv64asm.JAL:
			jumpOffset := jalImmediate(code[pos:], instruction)
			rd := instruction.Args[0].(riscv64asm.Reg)
			targetOffset := pos + int(jumpOffset)
			if rd != riscv64asm.X0 || isExternalJAL(targetOffset, len(code), rd) {
				proxySize = alignedProxyOffset(proxySize)
				assertSourceVeneerReachable(pos, instruction.Len, proxySize)
				switch rd {
				case riscv64asm.X0:
					proxySize += riscv64JumpVeneerSize
				case riscv64asm.X1, riscv64asm.X5:
					tool.Assert(patched, "RISC-V call relocation requires BuildPatch")
					proxySize += riscv64CallVeneerSize
				default:
					tool.Assert(false, "cannot safely relocate a call using %v at +%d", rd, pos)
				}
			}
		case riscv64asm.JALR:
			rd := instruction.Args[0].(riscv64asm.Reg)
			switch rd {
			case riscv64asm.X0:
			case riscv64asm.X1, riscv64asm.X5:
				tool.Assert(patched, "RISC-V indirect call relocation requires BuildPatch")
				proxySize = alignedProxyOffset(proxySize)
				assertSourceVeneerReachable(pos, instruction.Len, proxySize)
				base := instruction.Args[1].(riscv64asm.RegOffset).OfsReg
				if rd == base {
					proxySize += riscv64SameRegSize
				} else {
					proxySize += riscv64IndirectSize
				}
			default:
				tool.Assert(false, "cannot safely relocate an indirect call using %v at +%d", rd, pos)
			}
		}
		assertProxyCapacity(proxySize)

		nextPos, stop := scan.advance(pos, instruction, len(code))
		if stop {
			break
		}
		pos = nextPos
	}

	if !patched {
		return
	}
	seenLink := make(map[int]bool, 2)
	for _, landing := range collectReturnLandings(code) {
		seenLink[landing.linkReg] = true
	}
	for _, linkReg := range []int{riscv64RA, int(riscv64asm.X5)} {
		if !seenLink[linkReg] {
			continue
		}
		proxySize = alignedProxyOffset(proxySize) + riscv64DispatcherSize
		assertProxyCapacity(proxySize)
	}
	proxySize = alignedProxyOffset(proxySize) + riscv64HookSize
	assertProxyCapacity(proxySize)
}

func assertSourceVeneerReachable(sourceOffset, instructionSize, veneerOffset int) {
	difference := veneerOffset - sourceOffset
	switch instructionSize {
	case 2:
		tool.Assert(
			difference >= -2048 && difference < 2048 && difference%2 == 0,
			"RISC-V compressed call/jump at +%d cannot reach its veneer at +%d (+/-2 KiB limit)",
			sourceOffset,
			veneerOffset,
		)
	case 4:
		tool.Assert(
			difference >= -(1<<20) && difference < 1<<20 && difference%2 == 0,
			"RISC-V call/jump at +%d cannot reach its veneer at +%d (+/-1 MiB limit)",
			sourceOffset,
			veneerOffset,
		)
	default:
		tool.Assert(false, "unsupported RISC-V jump size %d", instructionSize)
	}
}

func assertProxyCapacity(required int) {
	tool.Assert(
		required <= MaxPatchSize(),
		"RISC-V proxy requires %d bytes, exceeds the %d-byte proxy page",
		required,
		MaxPatchSize(),
	)
}

func assertCompressedGatewayLayout(landings []returnLanding) {
	for _, linkReg := range []int{riscv64RA, int(riscv64asm.X5)} {
		lower := riscv64EntryPatchSize
		upper := int(^uint(0) >> 1)
		seen := false
		for _, landing := range landings {
			if landing.linkReg != linkReg || landing.size() != 2 {
				continue
			}
			seen = true
			if candidate := landing.start - 2048; candidate > lower {
				lower = candidate
			}
			if candidate := landing.start + 2046; candidate < upper {
				upper = candidate
			}
		}
		tool.Assert(
			!seen || lower <= upper,
			"RISC-V compressed return landings for X%d have no common +/-2 KiB gateway range",
			linkReg,
		)
	}
}

type codeRange struct {
	start int
	end   int
}

func isExternalJAL(targetOffset, copiedLen int, rd riscv64asm.Reg) bool {
	if targetOffset < 0 || targetOffset > copiedLen {
		return true
	}
	return targetOffset == copiedLen && rd != riscv64asm.X0
}

func appendHookTrampoline(proxy []byte, hookAddr uintptr) []byte {
	proxy = append(proxy, auipc(riscv64CTXT, 0)...)
	proxy = append(proxy, ld(riscv64CTXT, riscv64CTXT, 16)...)
	proxy = append(proxy, ld(riscv64TMP, riscv64CTXT, 0)...)
	proxy = append(proxy, jalr(riscv64Zero, riscv64TMP, 0)...)
	return appendUint64(proxy, uint64(hookAddr))
}

func appendAbsoluteJumpVeneer(proxy []byte, target uintptr) []byte {
	proxy = append(proxy, auipc(riscv64TMP, 0)...)
	proxy = append(proxy, ld(riscv64TMP, riscv64TMP, 16)...)
	proxy = append(proxy, jalr(riscv64Zero, riscv64TMP, 0)...)
	proxy = append(proxy, nop()...)
	return appendUint64(proxy, uint64(target))
}

// appendCallVeneer preserves a static target return PC in the link register.
// runtime.morestack and ordinary callees therefore see the target function's
// real stack maps. The target return PC contains a nearby jump back into the
// proxy continuation.
func appendCallVeneer(proxy []byte, linkReg int, returnAddr, target uintptr) []byte {
	proxy = append(proxy, auipc(linkReg, 0)...)
	proxy = append(proxy, ld(linkReg, linkReg, 24)...)
	proxy = append(proxy, auipc(riscv64TMP, 0)...)
	proxy = append(proxy, ld(riscv64TMP, riscv64TMP, 24)...)
	proxy = append(proxy, jalr(riscv64Zero, riscv64TMP, 0)...)
	proxy = append(proxy, nop()...)
	proxy = appendUint64(proxy, uint64(returnAddr))
	return appendUint64(proxy, uint64(target))
}

func isRuntimeMorestack(target uintptr) bool {
	resolved := resolvePCRelativeJump(target)
	return resolved == runtimeMorestackAddr || resolved == runtimeMorestackNoctxtAddr
}

func relocatedAUIPCValue(
	code []byte,
	pos int,
	instruction riscv64asm.Inst,
	targetAddr uintptr,
	proxyAddr uintptr,
) uintptr {
	value := addSigned(targetAddr+uintptr(pos), auipcImm(instruction))
	nextPos := pos + instruction.Len
	if nextPos >= len(code) {
		return value
	}
	next, err := riscv64asm.Decode(code[nextPos:])
	if err != nil {
		return value
	}

	rd := instruction.Args[0].(riscv64asm.Reg)
	var (
		effective uintptr
		found     bool
	)
	if next.Op == riscv64asm.ADDI &&
		next.Args[1].(riscv64asm.Reg) == rd {
		effective = addSigned(value, int64(next.Args[2].(riscv64asm.Simm).Imm))
		found = true
	} else {
		for _, arg := range next.Args {
			regOffset, ok := arg.(riscv64asm.RegOffset)
			if !ok || regOffset.OfsReg != rd {
				continue
			}
			effective = addSigned(value, int64(regOffset.Ofs.Imm))
			found = true
			break
		}
	}
	if !found || effective < targetAddr || effective >= targetAddr+uintptr(len(code)) {
		return value
	}
	delta, ok := signedAddressDifference(targetAddr, proxyAddr)
	tool.Assert(ok, "invalid RISC-V proxy delta: %#x -> %#x", targetAddr, proxyAddr)
	return addSigned(value, delta)
}

func appendAUIPCVeneer(
	proxy []byte,
	proxyAddr uintptr,
	rd int,
	value uintptr,
	returnAddr uintptr,
) ([]byte, int) {
	proxy = alignVeneer(proxy)
	veneerOffset := len(proxy)
	proxy = append(proxy, auipc(rd, 0)...)
	proxy = append(proxy, ld(rd, rd, 16)...)
	proxy = append(
		proxy,
		jal(riscv64Zero, checkedJALOffset(proxyAddr+uintptr(veneerOffset+8), returnAddr))...,
	)
	proxy = append(proxy, nop()...)
	proxy = appendUint64(proxy, uint64(value))
	return proxy, veneerOffset
}

func alignVeneer(proxy []byte) []byte {
	for len(proxy)%8 != 0 {
		// C.NOP keeps every possible entry in the executable proxy decodable.
		proxy = append(proxy, 0x01, 0x00)
	}
	return proxy
}

func pcRelativeJump(from, to uintptr) []byte {
	high, low, ok := pcRelativeParts(from, to)
	tool.Assert(ok, "RISC-V AUIPC/JALR jump from %#x to %#x is out of range", from, to)
	result := append([]byte(nil), auipc(riscv64TMP, high)...)
	return append(result, jalr(riscv64Zero, riscv64TMP, low)...)
}

func signedAddressDifference(from, to uintptr) (int64, bool) {
	if to >= from {
		distance := to - from
		if distance > uintptr(^uint64(0)>>1) {
			return 0, false
		}
		return int64(distance), true
	}
	distance := from - to
	if distance > uintptr(^uint64(0)>>1) {
		return 0, false
	}
	return -int64(distance), true
}

func pcRelativeParts(from, to uintptr) (high, low int32, ok bool) {
	difference, ok := signedAddressDifference(from, to)
	if !ok || difference < -(1<<32) || difference > 1<<32 {
		return 0, 0, false
	}
	high64 := (difference + 0x800) >> 12
	low64 := difference - high64<<12
	if high64 < -(1<<19) || high64 >= 1<<19 || low64 < -2048 || low64 > 2047 {
		return 0, 0, false
	}
	return int32(high64), int32(low64), true
}

func checkedJALOffset(from, to uintptr) int32 {
	offset, ok := signedAddressDifference(from, to)
	tool.Assert(
		ok && offset%2 == 0 && offset >= -(1<<20) && offset < 1<<20,
		"RISC-V proxy jump from %#x to %#x is out of range",
		from,
		to,
	)
	return int32(offset)
}

func addSigned(base uintptr, offset int64) uintptr {
	if offset >= 0 {
		delta := uintptr(offset)
		tool.Assert(base <= ^uintptr(0)-delta, "invalid RISC-V PC-relative address: %#x + %d", base, offset)
		return base + delta
	}
	delta := uintptr(-(offset + 1)) + 1
	tool.Assert(base >= delta, "invalid RISC-V PC-relative address: %#x + %d", base, offset)
	return base - delta
}

// appendIndirectCallVeneer gives an indirect callee the real target return PC.
// If JALR reads and writes the same register, preserve its old target first.
func appendIndirectCallVeneer(
	proxy []byte,
	proxyAddr uintptr,
	linkReg, baseReg int,
	offset int32,
	returnAddr uintptr,
) []byte {
	if linkReg == baseReg {
		proxy = append(proxy, addi(riscv64TMP, baseReg, offset)...)
		auipcAddr := proxyAddr + uintptr(len(proxy))
		high, low, ok := pcRelativeParts(auipcAddr, returnAddr)
		tool.Assert(ok, "RISC-V indirect call return PC %#x is out of range", returnAddr)
		proxy = append(proxy, auipc(linkReg, high)...)
		proxy = append(proxy, addi(linkReg, linkReg, low)...)
		return append(proxy, jalr(riscv64Zero, riscv64TMP, 0)...)
	}

	auipcAddr := proxyAddr + uintptr(len(proxy))
	high, low, ok := pcRelativeParts(auipcAddr, returnAddr)
	tool.Assert(ok, "RISC-V indirect call return PC %#x is out of range", returnAddr)
	proxy = append(proxy, auipc(linkReg, high)...)
	proxy = append(proxy, addi(linkReg, linkReg, low)...)
	return append(proxy, jalr(riscv64Zero, baseReg, offset)...)
}

// appendReturnDispatcher maps a real target return PC in linkReg to the same
// instruction offset in the proxy without changing that link register.
func appendReturnDispatcher(proxy []byte, linkReg int, proxyDelta uintptr) []byte {
	proxy = append(proxy, auipc(riscv64TMP, 0)...)
	proxy = append(proxy, ld(riscv64TMP, riscv64TMP, 16)...)
	proxy = append(proxy, add(riscv64TMP, linkReg, riscv64TMP)...)
	proxy = append(proxy, jalr(riscv64Zero, riscv64TMP, 0)...)
	return appendUint64(proxy, uint64(proxyDelta))
}

type returnLanding struct {
	codeRange
	linkReg int
}

func (landing returnLanding) size() int {
	return landing.end - landing.start
}

type patchPlan struct {
	landings []returnLanding
	gateways map[int]codeRange
}

func (plan patchPlan) linkRegisters() []int {
	registers := make([]int, 0, len(plan.gateways))
	for _, register := range []int{riscv64RA, int(riscv64asm.X5)} {
		if _, ok := plan.gateways[register]; ok {
			registers = append(registers, register)
		}
	}
	return registers
}

func collectReturnLandings(code []byte) []returnLanding {
	landings := make([]returnLanding, 0, 1)
	var scan reachableCodeScan
	for pos := 0; pos < len(code); {
		instruction, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil, "decode return landing at +%d: %v", pos, err)
		scan.observe(pos, instruction, code[pos:])

		rd := riscv64asm.X0
		call := false
		switch instruction.Op {
		case riscv64asm.JAL:
			rd = instruction.Args[0].(riscv64asm.Reg)
			call = rd != riscv64asm.X0
		case riscv64asm.JALR:
			rd = instruction.Args[0].(riscv64asm.Reg)
			call = rd != riscv64asm.X0
		}
		if call {
			tool.Assert(
				rd == riscv64asm.X1 || rd == riscv64asm.X5,
				"cannot safely relocate a call using %v at +%d",
				rd,
				pos,
			)
			returnPos := pos + instruction.Len
			landing := returnLanding{
				codeRange: codeRange{start: returnPos, end: returnPos + 4},
				linkReg:   int(rd),
			}
			tool.Assert(
				landing.start >= riscv64EntryPatchSize,
				"RISC-V return PC at +%d overlaps the entry patch",
				landing.start,
			)
			landings = append(landings, landing)
		}

		nextPos, stop := scan.advance(pos, instruction, len(code))
		if stop {
			break
		}
		pos = nextPos
	}
	sort.Slice(landings, func(left, right int) bool {
		return landings[left].start < landings[right].start
	})
	// A return landing normally needs a 4-byte JAL. If the next return PC is
	// only two bytes away, use C.J for the earlier landing so adjacent mixed
	// 2/4-byte calls do not overlap. The final landing remains four bytes.
	for index := 0; index+1 < len(landings); index++ {
		distance := landings[index+1].start - landings[index].start
		tool.Assert(distance >= 2, "invalid RISC-V return landing distance %d", distance)
		if distance < 4 {
			landings[index].end = landings[index].start + 2
		}
	}
	return landings
}

func makePatchPlan(code []byte) (patchPlan, bool) {
	plan := patchPlan{
		landings: collectReturnLandings(code),
		gateways: make(map[int]codeRange, 2),
	}
	blocked := []codeRange{{start: 0, end: riscv64EntryPatchSize}}
	for _, landing := range plan.landings {
		if landing.end > len(code) {
			return patchPlan{}, false
		}
		for _, current := range blocked[1:] {
			tool.Assert(
				landing.end <= current.start || landing.start >= current.end,
				"overlapping RISC-V return landings [%d,%d) and [%d,%d)",
				landing.start,
				landing.end,
				current.start,
				current.end,
			)
		}
		blocked = append(blocked, landing.codeRange)
	}
	registers := make([]int, 0, 2)
	for _, landing := range plan.landings {
		if _, exists := plan.gateways[landing.linkReg]; exists {
			continue
		}
		plan.gateways[landing.linkReg] = codeRange{}
		registers = append(registers, landing.linkReg)
	}
	for register := range plan.gateways {
		delete(plan.gateways, register)
	}
	if !placeReturnGateways(code, registers, 0, blocked, &plan) {
		return patchPlan{}, false
	}
	return plan, true
}

func placeReturnGateways(
	code []byte,
	registers []int,
	index int,
	blocked []codeRange,
	plan *patchPlan,
) bool {
	if index == len(registers) {
		return true
	}
	register := registers[index]
	for offset := riscv64EntryPatchSize; offset+riscv64GatewaySize <= len(code); offset += 2 {
		candidate := codeRange{start: offset, end: offset + riscv64GatewaySize}
		if !rangeAvailable(candidate, blocked) ||
			!gatewayReachable(candidate.start, register, plan.landings) {
			continue
		}
		plan.gateways[register] = candidate
		if placeReturnGateways(
			code,
			registers,
			index+1,
			append(blocked, candidate),
			plan,
		) {
			return true
		}
		delete(plan.gateways, register)
	}
	return false
}

func rangeAvailable(candidate codeRange, blocked []codeRange) bool {
	for _, current := range blocked {
		if candidate.end > current.start && candidate.start < current.end {
			return false
		}
	}
	return true
}

func gatewayReachable(offset, linkReg int, landings []returnLanding) bool {
	for _, landing := range landings {
		if landing.linkReg != linkReg || landing.size() != 2 {
			continue
		}
		difference := offset - landing.start
		if difference < -2048 || difference >= 2048 {
			return false
		}
	}
	return true
}

func shortJump(size int, from, to uintptr) []byte {
	switch size {
	case 2:
		return cJ(checkedCJOffset(from, to))
	case 4:
		return jal(riscv64Zero, checkedJALOffset(from, to))
	default:
		tool.Assert(false, "unsupported RISC-V jump size %d", size)
		return nil
	}
}

func checkedCJOffset(from, to uintptr) int32 {
	offset, ok := signedAddressDifference(from, to)
	tool.Assert(
		ok && offset%2 == 0 && offset >= -2048 && offset < 2048,
		"RISC-V compressed jump from %#x to %#x is out of range",
		from,
		to,
	)
	return int32(offset)
}
