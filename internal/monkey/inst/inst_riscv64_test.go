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
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/smartystreets/goconvey/convey"
	"golang.org/x/arch/riscv64/riscv64asm"
)

func genericAddrTarget[T int | float64](l, r T) T {
	return l + r
}

func TestBranchTo(t *testing.T) {
	convey.Convey("TestBranchTo", t, func() {
		inst := fmt.Sprintf("%x", BranchTo(0x123456789abcef01))
		convey.So(inst, convey.ShouldEqual, "970f000083bf0f0167800f001300000001efbc9a78563412")
	})
}

func TestBranchInto(t *testing.T) {
	convey.Convey("TestBranchInto", t, func() {
		inst := fmt.Sprintf("%x", BranchInto(0x123456789abcef01))
		convey.So(inst, convey.ShouldEqual, "170d0000033d0d01833f0d0067800f0001efbc9a78563412")
	})
}

func TestDisassembleBranchInto(t *testing.T) {
	convey.Convey("TestDisassembleBranchInto", t, func() {
		code := BranchInto(^uintptr(0))
		convey.So(Disassemble(code, len(code), true), convey.ShouldEqual, len(code))
	})
}

func TestGetGenericAddr(t *testing.T) {
	convey.Convey("TestGetGenericAddr", t, func() {
		addr := reflect.ValueOf(genericAddrTarget[int]).Pointer()
		jumpAddr, genericInfoAddr := GetGenericAddr(addr, 10000)
		convey.So(jumpAddr, convey.ShouldNotEqual, uintptr(0))
		convey.So(genericInfoAddr, convey.ShouldNotEqual, uintptr(0))
	})
}

func TestDisassembleRVA23Instruction(t *testing.T) {
	convey.Convey("TestDisassembleRVA23Instruction", t, func() {
		// CZERO.EQZ X7, X6, X5 is part of the RVA23 Zicond extension.
		code := uint32ToBytes(0x0e5353b3)
		convey.So(Disassemble(code, len(code), true), convey.ShouldEqual, len(code))
	})
}

func TestDisassembleFarJALUsesVeneer(t *testing.T) {
	code := jal(riscv64Zero, 512)
	if got := Disassemble(code, len(code), true); got != len(code) {
		t.Fatalf("far JAL cutting point: got %d, want %d", got, len(code))
	}
}

func TestDisassembleCompressedForwardBranch(t *testing.T) {
	code := make([]byte, 32)
	binary.LittleEndian.PutUint16(code, 0xc005) // C.BEQZ X8, +32
	for pos := 2; pos < len(code); pos += 2 {
		binary.LittleEndian.PutUint16(code[pos:], 0x0001) // C.NOP
	}
	if got := Disassemble(code, 2, true); got != len(code) {
		t.Fatalf("compressed forward-branch cutting point: got %d, want %d", got, len(code))
	}
}

func TestBuildProxyRelocatesExternalJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x4000000000)
		callOffset = int32(0x400)
	)

	proxy := BuildProxy(targetAddr, proxyAddr, jal(riscv64Zero, callOffset))
	instruction, err := riscv64asm.Decode(proxy)
	if err != nil {
		t.Fatalf("decode relocated JAL: %v", err)
	}
	if rd := instruction.Args[0].(riscv64asm.Reg); rd != riscv64asm.X0 {
		t.Fatalf("relocated JAL link register: got %v, want X0", rd)
	}
	veneerOffset := int(instruction.Args[1].(riscv64asm.Simm).Imm)
	if veneerOffset <= 4 || veneerOffset+24 > len(proxy) {
		t.Fatalf("invalid JAL veneer offset %d for %d-byte proxy", veneerOffset, len(proxy))
	}
	gotTarget := binary.LittleEndian.Uint64(proxy[veneerOffset+16 : veneerOffset+24])
	wantTarget := uint64(targetAddr + uintptr(callOffset))
	if gotTarget != wantTarget {
		t.Fatalf("JAL veneer target: got %#x, want %#x", gotTarget, wantTarget)
	}
}

func TestBuildProxyRelocatesAUIPC(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x4000000000)
		imm20      = int32(0x123)
	)

	proxy := BuildProxy(targetAddr, proxyAddr, auipc(10, imm20))
	jump, err := riscv64asm.Decode(proxy)
	if err != nil {
		t.Fatalf("decode AUIPC replacement: %v", err)
	}
	if rd := jump.Args[0].(riscv64asm.Reg); rd != riscv64asm.X0 {
		t.Fatalf("AUIPC replacement link register: got %v, want X0", rd)
	}
	veneerOffset := int(jump.Args[1].(riscv64asm.Simm).Imm)
	if veneerOffset <= 4 || veneerOffset+24 > len(proxy) {
		t.Fatalf("invalid AUIPC veneer offset %d for %d-byte proxy", veneerOffset, len(proxy))
	}
	gotValue := binary.LittleEndian.Uint64(proxy[veneerOffset+16 : veneerOffset+24])
	wantValue := uint64(targetAddr + uintptr(imm20<<12))
	if gotValue != wantValue {
		t.Fatalf("AUIPC veneer value: got %#x, want %#x", gotValue, wantValue)
	}

	returnJump, err := riscv64asm.Decode(proxy[veneerOffset+8:])
	if err != nil {
		t.Fatalf("decode AUIPC veneer return: %v", err)
	}
	returnTarget := proxyAddr + uintptr(veneerOffset+8) +
		uintptr(returnJump.Args[1].(riscv64asm.Simm).Imm)
	if want := proxyAddr + 4; returnTarget != want {
		t.Fatalf("AUIPC veneer return target: got %#x, want %#x", returnTarget, want)
	}
}

func TestBuildProxyRelocatesInternalLiteralBase(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x4000000000)
	)

	proxy := BuildProxy(targetAddr, proxyAddr, BranchInto(0x123456789abcdef0))
	jump, err := riscv64asm.Decode(proxy)
	if err != nil {
		t.Fatalf("decode internal-literal AUIPC replacement: %v", err)
	}
	veneerOffset := int(jump.Args[1].(riscv64asm.Simm).Imm)
	if veneerOffset <= len(BranchInto(0)) || veneerOffset+24 > len(proxy) {
		t.Fatalf("invalid internal-literal veneer offset %d for %d-byte proxy", veneerOffset, len(proxy))
	}
	gotBase := binary.LittleEndian.Uint64(proxy[veneerOffset+16 : veneerOffset+24])
	if gotBase != uint64(proxyAddr) {
		t.Fatalf("relocated internal-literal base: got %#x, want %#x", gotBase, proxyAddr)
	}
}

func TestBuildPatchRelocatesCallAtCopyEnd(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)

	code := make([]byte, 0, 24)
	for range 3 {
		code = append(code, nop()...)
	}
	code = append(code, jal(riscv64RA, 12)...)
	code = append(code, nop()...)
	code = append(code, nop()...)

	if got := PatchSize(targetAddr, code, true); got != len(code) {
		t.Fatalf("patch size: got %d, want %d", got, len(code))
	}

	proxyCode, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code)
	call, err := riscv64asm.Decode(proxyCode[12:])
	if err != nil {
		t.Fatalf("decode relocated call: %v", err)
	}
	veneerOffset := 12 + int(call.Args[1].(riscv64asm.Simm).Imm)
	if veneerOffset <= len(code) || veneerOffset+40 > len(proxyCode) {
		t.Fatalf("invalid call veneer offset %d for %d-byte proxy", veneerOffset, len(proxyCode))
	}
	if got, want := binary.LittleEndian.Uint64(proxyCode[veneerOffset+24:]), uint64(targetAddr+16); got != want {
		t.Fatalf("call return PC: got %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint64(proxyCode[veneerOffset+32:]), uint64(targetAddr+24); got != want {
		t.Fatalf("call target: got %#x, want %#x", got, want)
	}
	plan, ok := makePatchPlan(code)
	if !ok {
		t.Fatal("return gateway plan failed")
	}
	gateway := plan.gateways[riscv64RA]
	if got, want := targetPatch[16:20], shortJump(4, targetAddr+16, targetAddr+uintptr(gateway.start)); !bytes.Equal(got, want) {
		t.Fatalf("return landing: got %x, want %x", got, want)
	}
}

func TestAllocateProxySupportsManyConcurrentPages(t *testing.T) {
	const proxyCount = 70

	targetAddr := reflect.ValueOf(genericAddrTarget[int]).Pointer()
	proxies := make([][]byte, 0, proxyCount)
	defer func() {
		for index := len(proxies) - 1; index >= 0; index-- {
			ReleaseProxy(proxies[index])
		}
	}()

	seen := make(map[uintptr]struct{}, proxyCount)
	for index := 0; index < proxyCount; index++ {
		proxy := AllocateProxy(targetAddr)
		address := common.PtrOf(proxy)
		if _, ok := seen[address]; ok {
			t.Fatalf("proxy %d reused live address %#x", index, address)
		}
		seen[address] = struct{}{}
		if len(proxy) == 0 {
			t.Fatalf("proxy %d has zero length", index)
		}
		if _, _, ok := pcRelativeParts(targetAddr, address+uintptr(len(proxy)-1)); !ok {
			t.Fatalf("proxy %d at %#x is outside AUIPC/JALR range of %#x", index, address, targetAddr)
		}
		proxies = append(proxies, proxy)
	}
}

func TestGenericJumpUsesLocalCompressedOffsetDecoder(t *testing.T) {
	code := cJ(6)
	instruction, err := riscv64asm.Decode(code)
	if err != nil {
		t.Fatalf("decode C.J: %v", err)
	}
	address := common.PtrOf(code)
	jump := &genericJmpInst{
		posInst: &posInst{addr: address, inst: instruction},
	}
	if got, want := jump.calcJumpAddr(nil), address+6; got != want {
		t.Fatalf("generic C.J target: got %#x, want %#x", got, want)
	}
}

func TestGenericScannerIgnoresX5LinkCalls(t *testing.T) {
	// GetGenericAddr looks for the wrapper's single business call. Go uses X5
	// for compiler-inserted morestack calls, so those must not become a second
	// generic target. Proxy relocation classifies X5 independently.
	for _, encoding := range [][]byte{
		jal(int(riscv64asm.X5), 8),
		jalr(int(riscv64asm.X5), 10, 0),
	} {
		instruction, err := riscv64asm.Decode(encoding)
		if err != nil {
			t.Fatalf("decode X5 call: %v", err)
		}
		if isCall(instruction) {
			t.Fatalf("generic scanner classified X5 instruction as business call: %v", instruction)
		}
	}

	instruction, err := riscv64asm.Decode(jal(riscv64RA, 8))
	if err != nil {
		t.Fatalf("decode X1 call: %v", err)
	}
	if !isCall(instruction) {
		t.Fatalf("generic scanner missed X1 business call: %v", instruction)
	}
}

func TestGenericJumpClassifiesPCRelativeTrampoline(t *testing.T) {
	code := make([]byte, 24)
	copy(code, jal(riscv64RA, 8))
	copy(code[8:], auipc(int(riscv64asm.X31), 0))
	copy(code[12:], jalr(riscv64Zero, int(riscv64asm.X31), 8))

	base := common.PtrOf(code)
	target := base + 16
	const helperName = "runtime helper behind linker trampoline"
	oldName, existed := proxyCallRace[target]
	proxyCallRace[target] = helperName
	t.Cleanup(func() {
		if existed {
			proxyCallRace[target] = oldName
		} else {
			delete(proxyCallRace, target)
		}
	})

	instruction, err := riscv64asm.Decode(code)
	if err != nil {
		t.Fatalf("decode call to trampoline: %v", err)
	}
	jump := newGenericJmpInst(base, 0, instruction, nil)
	if got, want := jump.jumpAddr, base+8; got != want {
		t.Fatalf("generic call target: got %#x, want trampoline %#x", got, want)
	}
	if !jump.isExtraCall || jump.extraCallName != helperName {
		t.Fatalf("generic trampoline classification: got extra=%v name=%q", jump.isExtraCall, jump.extraCallName)
	}
}

func TestGetGenericAddrStopsAtPartialInstructionBoundary(t *testing.T) {
	code := make([]byte, 24)
	copy(code, jal(riscv64RA, 16))
	copy(code[4:], nop())
	copy(code[8:], nop())

	base := common.PtrOf(code)
	jumpAddr, genericInfoAddr := GetGenericAddr(base, 10)
	if got, want := jumpAddr, base+16; got != want {
		t.Fatalf("generic call target: got %#x, want %#x", got, want)
	}
	if genericInfoAddr != 0 {
		t.Fatalf("generic info address without AUIPC/ADDI: got %#x, want 0", genericInfoAddr)
	}
}
