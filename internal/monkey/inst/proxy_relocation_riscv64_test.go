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
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/arch/riscv64/riscv64asm"
)

func nopCode(size int) []byte {
	code := make([]byte, 0, size)
	for len(code)+4 <= size {
		code = append(code, nop()...)
	}
	if len(code) < size {
		code = append(code, 0x01, 0x00) // C.NOP
	}
	return code
}

func decodedJumpTarget(t *testing.T, code []byte, pc uintptr) (uintptr, riscv64asm.Inst) {
	t.Helper()
	instruction, err := riscv64asm.Decode(code)
	if err != nil {
		t.Fatalf("decode jump at %#x: %v", pc, err)
	}
	if instruction.Op != riscv64asm.JAL {
		t.Fatalf("instruction at %#x: got %v, want JAL", pc, instruction.Op)
	}
	var offset int64
	if instruction.Len == 2 {
		offset = int64(decodeCJOffset(code))
	} else {
		offset = int64(instruction.Args[1].(riscv64asm.Simm).Imm)
	}
	return uintptr(int64(pc) + offset), instruction
}

func decodedPCRelativeTarget(t *testing.T, code []byte, pc uintptr) uintptr {
	t.Helper()
	high, err := riscv64asm.Decode(code)
	if err != nil || high.Op != riscv64asm.AUIPC {
		t.Fatalf("decode gateway AUIPC at %#x: %v, %v", pc, high.Op, err)
	}
	low, err := riscv64asm.Decode(code[high.Len:])
	if err != nil || low.Op != riscv64asm.JALR {
		t.Fatalf("decode gateway JALR at %#x: %v, %v", pc+uintptr(high.Len), low.Op, err)
	}
	offset := low.Args[1].(riscv64asm.RegOffset)
	return uintptr(int64(pc) + auipcImm(high) + int64(offset.Ofs.Imm))
}

func compressedJALR(rs1 int) []byte {
	return uint16ToBytes(uint16(0x9002 | rs1<<7))
}

func TestBuildProxyAlignsTailLiteral(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	for _, codeSize := range []int{24, 26, 30} {
		t.Run(fmt.Sprint(codeSize), func(t *testing.T) {
			code := nopCode(codeSize)
			proxy := BuildProxy(targetAddr, proxyAddr, code)
			tailOffset := (codeSize + 7) &^ 7
			if got := proxyAddr + uintptr(tailOffset+16); got%8 != 0 {
				t.Fatalf("tail literal address %#x is not 8-byte aligned", got)
			}
			if got, want := binary.LittleEndian.Uint64(proxy[tailOffset+16:tailOffset+24]), uint64(targetAddr+uintptr(codeSize)); got != want {
				t.Fatalf("tail target: got %#x, want %#x", got, want)
			}
			for offset := codeSize; offset < tailOffset; offset += 2 {
				if got := binary.LittleEndian.Uint16(proxy[offset:]); got != 0x0001 {
					t.Fatalf("padding at +%d: got %#x, want C.NOP", offset, got)
				}
			}
		})
	}
}

func TestCompressedJumpEncoding(t *testing.T) {
	for _, offset := range []int32{-2048, -2, 2, 2046} {
		instruction, err := riscv64asm.Decode(cJ(offset))
		if err != nil {
			t.Fatalf("decode C.J %d: %v", offset, err)
		}
		if instruction.Len != 2 || instruction.Op != riscv64asm.JAL {
			t.Fatalf("C.J %d decoded as %v length %d", offset, instruction.Op, instruction.Len)
		}
		if got := decodeCJOffset(cJ(offset)); got != offset {
			t.Fatalf("C.J immediate: got %d, want %d", got, offset)
		}
	}
}

func TestPCRelativeParts(t *testing.T) {
	for _, test := range []struct {
		from uintptr
		to   uintptr
	}{
		{from: 0x1000, to: 0x1000},
		{from: 0x400000, to: 0x200000},
		{from: 0x200000, to: 0x40123456},
	} {
		high, low, ok := pcRelativeParts(test.from, test.to)
		if !ok {
			t.Fatalf("pcRelativeParts(%#x, %#x) failed", test.from, test.to)
		}
		if got := int64(test.from) + int64(high)<<12 + int64(low); got != int64(test.to) {
			t.Fatalf("reconstructed target: got %#x, want %#x", got, test.to)
		}
	}
	if _, _, ok := pcRelativeParts(0x1000, 0x1000+(1<<31)); ok {
		t.Fatal("accepted an out-of-range positive AUIPC/JALR target")
	}
}

func TestExtendTMPSequence(t *testing.T) {
	code := append(auipc(riscv64TMP, 1), ld(10, riscv64TMP, 0)...)
	code = append(code, nop()...)
	if got := extendTMPSequence(code, 4); got != 8 {
		t.Fatalf("TMP sequence boundary: got %d, want 8", got)
	}
	if got := extendTMPSequence(nopCode(8), 4); got != 4 {
		t.Fatalf("plain boundary: got %d, want 4", got)
	}
}

func TestBuildProxyRejectsExternalConditionalBranch(t *testing.T) {
	// BEQ X0, X0, +16.
	code := append(uint32ToBytes(0x00000863), nop()...)
	defer func() {
		if recover() == nil {
			t.Fatal("external conditional branch did not panic")
		}
	}()
	BuildProxy(0x200000, 0x400000, code)
}

func TestBuildPatchRelocatesAdjacentCalls(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(32)
	copy(code[8:], jal(riscv64RA, 0x400))
	copy(code[12:], jal(riscv64RA, 0x500))

	patchSize := PatchSize(targetAddr, code, true)
	if patchSize != len(code) {
		t.Fatalf("patch size: got %d, want %d", patchSize, len(code))
	}
	proxy, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	plan, ok := makePatchPlan(code[:patchSize])
	if !ok {
		t.Fatal("return gateway plan failed")
	}
	if len(plan.landings) != 2 {
		t.Fatalf("return landings: got %d, want 2", len(plan.landings))
	}
	gateway := plan.gateways[riscv64RA]

	for index, call := range []struct {
		pos    int
		offset int32
	}{
		{pos: 8, offset: 0x400},
		{pos: 12, offset: 0x500},
	} {
		returnPos := call.pos + 4
		landingTarget, landing := decodedJumpTarget(
			t,
			targetPatch[returnPos:],
			targetAddr+uintptr(returnPos),
		)
		if landing.Len != 4 || landing.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
			t.Fatalf("landing %d: got %v, want 4-byte JAL X0", index, landing)
		}
		if want := targetAddr + uintptr(gateway.start); landingTarget != want {
			t.Fatalf("landing %d target: got %#x, want %#x", index, landingTarget, want)
		}

		veneerAddr, source := decodedJumpTarget(
			t,
			proxy[call.pos:],
			proxyAddr+uintptr(call.pos),
		)
		if source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
			t.Fatalf("source call %d writes %v, want X0", index, source.Args[0])
		}
		veneerOffset := int(veneerAddr - proxyAddr)
		if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+24:]), uint64(targetAddr+uintptr(returnPos)); got != want {
			t.Fatalf("call %d return PC: got %#x, want %#x", index, got, want)
		}
		if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+32:]), uint64(int64(targetAddr+uintptr(call.pos))+int64(call.offset)); got != want {
			t.Fatalf("call %d target: got %#x, want %#x", index, got, want)
		}
	}

	dispatcherAddr := decodedPCRelativeTarget(
		t,
		targetPatch[gateway.start:gateway.end],
		targetAddr+uintptr(gateway.start),
	)
	dispatcherOffset := int(dispatcherAddr - proxyAddr)
	if dispatcherOffset < 0 || dispatcherOffset+24 > len(proxy) {
		t.Fatalf("dispatcher offset %d is outside %d-byte proxy", dispatcherOffset, len(proxy))
	}
	if got, want := binary.LittleEndian.Uint64(proxy[dispatcherOffset+16:]), uint64(proxyAddr-targetAddr); got != want {
		t.Fatalf("dispatcher delta: got %#x, want %#x", got, want)
	}
}

func TestBuildPatchRelocatesIndirectCalls(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	for _, test := range []struct {
		name    string
		linkReg int
		baseReg int
		offset  int32
	}{
		{name: "RA", linkReg: riscv64RA, baseReg: 10, offset: 12},
		{name: "same register", linkReg: riscv64RA, baseReg: riscv64RA, offset: 12},
		{name: "X5", linkReg: int(riscv64asm.X5), baseReg: riscv64TMP, offset: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := nopCode(32)
			copy(code[8:], jalr(test.linkReg, test.baseReg, test.offset))
			patchSize := PatchSize(targetAddr, code, true)
			proxy, targetPatch := BuildPatch(
				targetAddr,
				proxyAddr,
				hookAddr,
				code[:patchSize],
			)

			veneerAddr, source := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
			if source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
				t.Fatalf("proxy source writes %v, want X0", source.Args[0])
			}
			veneerOffset := int(veneerAddr - proxyAddr)
			veneer := proxy[veneerOffset:]
			auipcOffset := 0
			if test.linkReg == test.baseReg {
				capture, err := riscv64asm.Decode(veneer)
				if err != nil || capture.Op != riscv64asm.ADDI {
					t.Fatalf("decode target capture: %v, %v", capture.Op, err)
				}
				if got := capture.Args[0].(riscv64asm.Reg); got != riscv64asm.X31 {
					t.Fatalf("capture destination: got %v, want X31", got)
				}
				auipcOffset = capture.Len
			}

			high, err := riscv64asm.Decode(veneer[auipcOffset:])
			if err != nil || high.Op != riscv64asm.AUIPC {
				t.Fatalf("decode return AUIPC: %v, %v", high.Op, err)
			}
			low, err := riscv64asm.Decode(veneer[auipcOffset+high.Len:])
			if err != nil || low.Op != riscv64asm.ADDI {
				t.Fatalf("decode return ADDI: %v, %v", low.Op, err)
			}
			gotReturn := int64(veneerAddr+uintptr(auipcOffset)) +
				auipcImm(high) +
				int64(low.Args[2].(riscv64asm.Simm).Imm)
			if want := int64(targetAddr + 12); gotReturn != want {
				t.Fatalf("indirect return PC: got %#x, want %#x", gotReturn, want)
			}
			jumpOffset := auipcOffset + high.Len + low.Len
			jump, err := riscv64asm.Decode(veneer[jumpOffset:])
			if err != nil || jump.Op != riscv64asm.JALR {
				t.Fatalf("decode indirect jump: %v, %v", jump.Op, err)
			}
			jumpBase := jump.Args[1].(riscv64asm.RegOffset)
			if test.linkReg == test.baseReg {
				if jumpBase.OfsReg != riscv64asm.X31 || jumpBase.Ofs.Imm != 0 {
					t.Fatalf("same-register jump base: got %v", jumpBase)
				}
			} else if int(jumpBase.OfsReg) != test.baseReg || jumpBase.Ofs.Imm != test.offset {
				t.Fatalf("indirect jump base: got %v, want X%d+%d", jumpBase, test.baseReg, test.offset)
			}

			plan, ok := makePatchPlan(code[:patchSize])
			if !ok {
				t.Fatal("return gateway plan failed")
			}
			gateway := plan.gateways[test.linkReg]
			landingTarget, landing := decodedJumpTarget(t, targetPatch[12:], targetAddr+12)
			if landing.Len != 4 || landingTarget != targetAddr+uintptr(gateway.start) {
				t.Fatalf("indirect return landing: got %v to %#x", landing, landingTarget)
			}
		})
	}
}

func TestBuildProxyRelocatesExternalCompressedJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := append(cJ(0x400), nopCode(22)...)
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, source := decodedJumpTarget(t, proxy, proxyAddr)
	if source.Len != 2 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("compressed source: got %v, want 2-byte JAL X0", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+0x400); got != want {
		t.Fatalf("compressed jump target: got %#x, want %#x", got, want)
	}
}

func TestBuildPatchRelocatesCompressedIndirectCall(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(8)
	code = append(code, compressedJALR(10)...)
	code = append(code, 0x01, 0x00) // C.NOP
	code = append(code, nopCode(20)...)

	patchSize := PatchSize(targetAddr, code, true)
	proxy, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	veneerAddr, source := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	if source.Len != 2 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("compressed indirect source: got %v, want C.J", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	high, err := riscv64asm.Decode(proxy[veneerOffset:])
	if err != nil || high.Op != riscv64asm.AUIPC {
		t.Fatalf("decode compressed-call return AUIPC: %v, %v", high.Op, err)
	}
	low, err := riscv64asm.Decode(proxy[veneerOffset+high.Len:])
	if err != nil || low.Op != riscv64asm.ADDI {
		t.Fatalf("decode compressed-call return ADDI: %v, %v", low.Op, err)
	}
	gotReturn := int64(veneerAddr) +
		auipcImm(high) +
		int64(low.Args[2].(riscv64asm.Simm).Imm)
	if want := int64(targetAddr + 10); gotReturn != want {
		t.Fatalf("compressed call return PC: got %#x, want %#x", gotReturn, want)
	}
	jump, err := riscv64asm.Decode(proxy[veneerOffset+high.Len+low.Len:])
	if err != nil || jump.Op != riscv64asm.JALR {
		t.Fatalf("decode compressed indirect target jump: %v, %v", jump.Op, err)
	}
	if base := jump.Args[1].(riscv64asm.RegOffset); base.OfsReg != riscv64asm.X10 || base.Ofs.Imm != 0 {
		t.Fatalf("compressed indirect target base: got %v, want X10", base)
	}

	plan, ok := makePatchPlan(code[:patchSize])
	if !ok {
		t.Fatal("compressed return gateway plan failed")
	}
	gateway := plan.gateways[riscv64RA]
	landingTarget, landing := decodedJumpTarget(t, targetPatch[10:], targetAddr+10)
	if landing.Len != 4 || landingTarget != targetAddr+uintptr(gateway.start) {
		t.Fatalf("compressed return landing: got %v to %#x", landing, landingTarget)
	}
}

func assertPanicContains(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil {
			t.Fatalf("operation did not panic; want message containing %q", want)
		}
		if got := fmt.Sprint(value); !strings.Contains(got, want) {
			t.Fatalf("panic: got %q, want substring %q", got, want)
		}
	}()
	fn()
}

func beqOffset(offset int32) []byte {
	immediate := uint32(offset)
	instruction := ((immediate >> 12) & 0x1 << 31) |
		((immediate >> 5) & 0x3f << 25) |
		((immediate >> 1) & 0xf << 8) |
		((immediate >> 11) & 0x1 << 7) |
		0x63
	return uint32ToBytes(instruction)
}

func chainedForwardPrefix(size int) []byte {
	code := nopCode(size)
	for pos := 0; pos+4 < size; {
		target := pos + 4092
		if target > size {
			target = size
		}
		copy(code[pos:], beqOffset(int32(target-pos)))
		if target == size {
			break
		}
		pos = target - 4
	}
	return code
}

func TestBuildPatchMixedCallLandingSizes(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)

	jalThenCompressed := nopCode(32)
	copy(jalThenCompressed[8:], jal(riscv64RA, 0x400))
	copy(jalThenCompressed[12:], compressedJALR(10))
	copy(jalThenCompressed[14:], []byte{0x01, 0x00})

	compressedThenJAL := nopCode(32)
	copy(compressedThenJAL[8:], compressedJALR(10))
	copy(compressedThenJAL[10:], jal(riscv64RA, 0x400))
	copy(compressedThenJAL[14:], []byte{0x01, 0x00})

	for _, test := range []struct {
		name string
		code []byte
		want []codeRange
	}{
		{
			name: "JAL then C.JALR",
			code: jalThenCompressed,
			want: []codeRange{{start: 12, end: 14}, {start: 14, end: 18}},
		},
		{
			name: "C.JALR then JAL",
			code: compressedThenJAL,
			want: []codeRange{{start: 10, end: 14}, {start: 14, end: 18}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			patchSize := PatchSize(targetAddr, test.code, true)
			_, targetPatch := BuildPatch(
				targetAddr,
				proxyAddr,
				hookAddr,
				test.code[:patchSize],
			)
			plan, ok := makePatchPlan(test.code[:patchSize])
			if !ok {
				t.Fatal("mixed-width return gateway plan failed")
			}
			if len(plan.landings) != len(test.want) {
				t.Fatalf("landings: got %v, want %v", plan.landings, test.want)
			}
			for index, want := range test.want {
				landing := plan.landings[index]
				if landing.start != want.start || landing.end != want.end {
					t.Fatalf("landing %d: got [%d,%d), want [%d,%d)",
						index, landing.start, landing.end, want.start, want.end)
				}
				if index+1 < len(plan.landings) &&
					landing.end > plan.landings[index+1].start {
					t.Fatalf("landing %d overlaps landing %d", index, index+1)
				}
				gateway := plan.gateways[landing.linkReg]
				gotTarget, instruction := decodedJumpTarget(
					t,
					targetPatch[landing.start:],
					targetAddr+uintptr(landing.start),
				)
				if instruction.Len != landing.size() {
					t.Fatalf("landing %d jump length: got %d, want %d",
						index, instruction.Len, landing.size())
				}
				if wantTarget := targetAddr + uintptr(gateway.start); gotTarget != wantTarget {
					t.Fatalf("landing %d target: got %#x, want %#x",
						index, gotTarget, wantTarget)
				}
			}
		})
	}
}

func TestBuildPatchRelocatesInternalDirectCallToProxy(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(24)
	copy(code[16:], jal(riscv64RA, -8))

	patchSize := PatchSize(targetAddr, code, true)
	proxy, _ := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	plan, ok := makePatchPlan(code[:patchSize])
	if !ok {
		t.Fatal("internal-call gateway plan failed")
	}
	if gateway := plan.gateways[riscv64RA]; gateway.start != 8 {
		t.Fatalf("gateway: got [%d,%d), want local callee slot [8,16)",
			gateway.start, gateway.end)
	}

	veneerAddr, _ := decodedJumpTarget(t, proxy[16:], proxyAddr+16)
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+32:]), uint64(proxyAddr+8); got != want {
		t.Fatalf("internal callee target: got %#x, want proxy target %#x", got, want)
	}
}

func TestBuildPatchRelocatesAUIPCCallTarget(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	for _, test := range []struct {
		name       string
		auipcImm   int32
		jalrOffset int32
		wantBase   uintptr
		internal   bool
	}{
		{
			name:       "internal",
			auipcImm:   0,
			jalrOffset: -4,
			wantBase:   proxyAddr + 12,
			internal:   true,
		},
		{
			name:       "external",
			auipcImm:   1,
			jalrOffset: 0,
			wantBase:   targetAddr + 12 + 1<<12,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := nopCode(32)
			copy(code[12:], auipc(10, test.auipcImm))
			copy(code[16:], jalr(riscv64RA, 10, test.jalrOffset))

			patchSize := PatchSize(targetAddr, code, true)
			proxy, _ := BuildPatch(
				targetAddr,
				proxyAddr,
				hookAddr,
				code[:patchSize],
			)
			veneerAddr, _ := decodedJumpTarget(t, proxy[12:], proxyAddr+12)
			veneerOffset := int(veneerAddr - proxyAddr)
			if got := uintptr(binary.LittleEndian.Uint64(proxy[veneerOffset+16:])); got != test.wantBase {
				t.Fatalf("AUIPC call base: got %#x, want %#x", got, test.wantBase)
			}

			if test.internal {
				plan, ok := makePatchPlan(code[:patchSize])
				if !ok {
					t.Fatal("internal AUIPC/JALR gateway plan failed")
				}
				if gateway := plan.gateways[riscv64RA]; gateway.start != 8 {
					t.Fatalf("gateway: got [%d,%d), want local callee slot [8,16)",
						gateway.start, gateway.end)
				}
			}
		})
	}
}

func TestBuildProxyRejectsLongCompressedJump(t *testing.T) {
	code := append(cJ(2046), nopCode(2022)...)
	assertPanicContains(t, "compressed call/jump", func() {
		BuildProxy(0x200000, 0x400000, code)
	})
}

func TestBuildProxyRejectsFullPagePrefix(t *testing.T) {
	code := nopCode(MaxPatchSize())
	assertPanicContains(t, "exceeds the", func() {
		BuildProxy(0x200000, 0x400000, code)
	})
}

func TestPatchSizeRejectsPrefixWithoutProxyCapacity(t *testing.T) {
	code := chainedForwardPrefix(MaxPatchSize())
	assertPanicContains(t, "exceeds the", func() {
		PatchSize(0x200000, code, true)
	})
}

func TestPatchSizeRejectsDistantCompressedCallClusters(t *testing.T) {
	code := chainedForwardPrefix(8192)
	copy(code[8:], compressedJALR(10))
	copy(code[10:], compressedJALR(11))
	copy(code[8176:], compressedJALR(10))
	copy(code[8178:], compressedJALR(11))

	assertPanicContains(t, "no common +/-2 KiB gateway range", func() {
		PatchSize(0x200000, code, true)
	})
}

func TestPatchSizeRejectsNoProgress(t *testing.T) {
	assertPanicContains(t, "did not reach the initial prefix", func() {
		PatchSize(0x200000, nil, false)
	})
}

func TestProxyMappingReachabilityChecksBothDirectionsAndOverflow(t *testing.T) {
	const pageSize = uintptr(64 << 10)
	if !proxyMappingReachable(0x40000000, pageSize, 0x50000000, pageSize) {
		t.Fatal("rejected nearby 64 KiB target/proxy ranges")
	}

	const target = uintptr(0x100000000)
	const asymmetricDistance = uintptr(2147483000)
	proxy := target - asymmetricDistance
	if _, _, ok := pcRelativeParts(target, proxy); !ok {
		t.Fatal("test setup: negative direction should be encodable")
	}
	if _, _, ok := pcRelativeParts(proxy, target); ok {
		t.Fatal("test setup: positive reverse direction should be out of range")
	}
	if proxyMappingReachable(target, 1, proxy, 1) {
		t.Fatal("accepted mapping reachable only in the forward direction")
	}

	if proxyMappingReachable(^uintptr(0)-8, 16, 0x1000, pageSize) {
		t.Fatal("accepted overflowing target range")
	}
	if proxyMappingReachable(0x1000, pageSize, ^uintptr(0)-8, 16) {
		t.Fatal("accepted overflowing proxy range")
	}

	highAddress := ^uintptr(0) - 0x1000
	if _, _, ok := pcRelativeParts(highAddress, 0x1000); ok {
		t.Fatal("accepted wrapped high-to-low PC-relative difference")
	}
	if _, _, ok := pcRelativeParts(0x1000, highAddress); ok {
		t.Fatal("accepted wrapped low-to-high PC-relative difference")
	}
	if proxyMappingReachable(highAddress, 1, 0x1000, 1) {
		t.Fatal("accepted high/low mapping through signed wraparound")
	}
}

func TestBuildPatchRelocatesInternalX5DirectCall(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(24)
	copy(code[16:], jal(int(riscv64asm.X5), -8))

	patchSize := PatchSize(targetAddr, code, true)
	proxy, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	plan, ok := makePatchPlan(code[:patchSize])
	if !ok {
		t.Fatal("internal X5-call gateway plan failed")
	}
	gateway := plan.gateways[int(riscv64asm.X5)]
	landingTarget, landing := decodedJumpTarget(t, targetPatch[20:], targetAddr+20)
	if landing.Len != 4 || landingTarget != targetAddr+uintptr(gateway.start) {
		t.Fatalf("X5 landing: got %v to %#x", landing, landingTarget)
	}

	veneerAddr, _ := decodedJumpTarget(t, proxy[16:], proxyAddr+16)
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+24:]), uint64(targetAddr+20); got != want {
		t.Fatalf("X5 return PC: got %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+32:]), uint64(proxyAddr+8); got != want {
		t.Fatalf("X5 internal callee: got %#x, want %#x", got, want)
	}
}

func TestPatchSizeAccountsForAUIPCVeneerCapacity(t *testing.T) {
	codeSize := MaxPatchSize() - riscv64TailSize - riscv64HookSize
	code := chainedForwardPrefix(codeSize)
	copy(code[16:], auipc(10, 1))

	assertPanicContains(t, "proxy requires", func() {
		PatchSize(0x200000, code, true)
	})
}

func TestBuildProxyChecksEachCompressedVeneerOffset(t *testing.T) {
	code := nopCode(1904)
	for pos := 0; pos < 81*4; pos += 4 {
		copy(code[pos:], auipc(10, 1))
	}
	copy(code[1800:], cJ(2046))
	copy(code[1802:], []byte{0x01, 0x00})

	assertPanicContains(t, "cannot reach its veneer", func() {
		BuildProxy(0x200000, 0x400000, code)
	})
}

func TestBuildProxyRelocatesAUIPCPastBypassedTailJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[0:], beqOffset(8))
	copy(code[4:], jalr(riscv64Zero, 10, 0))
	copy(code[8:], auipc(11, 1))

	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, source := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	if source.Len != 4 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("reachable AUIPC source: got %v, want JAL X0", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+8+1<<12); got != want {
		t.Fatalf("reachable AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestBuildPatchPlansCallPastBypassedTailJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(24)
	copy(code[0:], beqOffset(8))
	copy(code[4:], jalr(riscv64Zero, 10, 0))
	copy(code[8:], jal(riscv64RA, 0x400))

	patchSize := PatchSize(targetAddr, code, true)
	proxy, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	plan, ok := makePatchPlan(code[:patchSize])
	if !ok || len(plan.landings) != 1 || plan.landings[0].start != 12 {
		t.Fatalf("reachable call plan: got %+v, ok=%v", plan.landings, ok)
	}
	gateway := plan.gateways[riscv64RA]
	landingTarget, landing := decodedJumpTarget(t, targetPatch[12:], targetAddr+12)
	if landing.Len != 4 || landingTarget != targetAddr+uintptr(gateway.start) {
		t.Fatalf("reachable call landing: got %v to %#x", landing, landingTarget)
	}

	veneerAddr, _ := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+24:]), uint64(targetAddr+12); got != want {
		t.Fatalf("reachable call return PC: got %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+32:]), uint64(targetAddr+8+0x400); got != want {
		t.Fatalf("reachable call target: got %#x, want %#x", got, want)
	}
}

func TestProxyStopsAtUnbypassedTailJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[4:], jalr(riscv64Zero, 10, 0))
	for index := 8; index < len(code); index++ {
		code[index] = 0xff
	}

	if got := PatchSize(targetAddr, code, true); got != len(code) {
		t.Fatalf("tail-jump patch size: got %d, want %d", got, len(code))
	}
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	if !bytes.Equal(proxy[8:len(code)], code[8:]) {
		t.Fatalf("unreachable bytes after tail jump were modified: got %x, want %x",
			proxy[8:len(code)], code[8:])
	}
}

func TestBuildProxyRelocatesAUIPCPastDirectJump(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[0:], jal(riscv64Zero, 8))
	copy(code[4:], jalr(riscv64Zero, 10, 0))
	copy(code[8:], auipc(11, 1))

	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, source := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	if source.Len != 4 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("direct-jump AUIPC source: got %v, want JAL X0", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+8+1<<12); got != want {
		t.Fatalf("direct-jump AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestBuildPatchScansInternalCallTargetPastTail(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(24)
	copy(code[4:], jal(riscv64RA, 8))
	copy(code[8:], jalr(riscv64Zero, 10, 0))
	copy(code[12:], auipc(11, 1))

	patchSize := PatchSize(targetAddr, code, true)
	proxy, targetPatch := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	plan, ok := makePatchPlan(code[:patchSize])
	if !ok || len(plan.landings) != 1 || plan.landings[0].start != 8 {
		t.Fatalf("internal-call plan: got %+v, ok=%v", plan.landings, ok)
	}
	gateway := plan.gateways[riscv64RA]
	landingTarget, landing := decodedJumpTarget(t, targetPatch[8:], targetAddr+8)
	if landing.Len != 4 || landingTarget != targetAddr+uintptr(gateway.start) {
		t.Fatalf("internal-call landing: got %v to %#x", landing, landingTarget)
	}

	callVeneerAddr, _ := decodedJumpTarget(t, proxy[4:], proxyAddr+4)
	callVeneerOffset := int(callVeneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[callVeneerOffset+32:]), uint64(proxyAddr+12); got != want {
		t.Fatalf("internal callee target: got %#x, want %#x", got, want)
	}
	auipcVeneerAddr, _ := decodedJumpTarget(t, proxy[12:], proxyAddr+12)
	auipcVeneerOffset := int(auipcVeneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[auipcVeneerOffset+16:]), uint64(targetAddr+12+1<<12); got != want {
		t.Fatalf("internal callee AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestDirectJumpSkipsUnreachableGap(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[0:], jal(riscv64Zero, 16))
	for index := 4; index < 16; index++ {
		code[index] = 0xff
	}
	copy(code[16:], auipc(11, 1))

	if got := PatchSize(targetAddr, code, true); got != len(code) {
		t.Fatalf("direct-jump patch size: got %d, want %d", got, len(code))
	}
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, _ := decodedJumpTarget(t, proxy[16:], proxyAddr+16)
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+16+1<<12); got != want {
		t.Fatalf("post-gap AUIPC value: got %#x, want %#x", got, want)
	}
	if !bytes.Equal(proxy[4:16], code[4:16]) {
		t.Fatalf("unreachable direct-jump gap was modified: got %x, want %x", proxy[4:16], code[4:16])
	}
}

func TestProxyStopsAtReturnWithoutBypass(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[4:], jalr(riscv64Zero, riscv64RA, 0))
	for index := 8; index < len(code); index++ {
		code[index] = 0xff
	}

	if got := PatchSize(targetAddr, code, false); got != len(code) {
		t.Fatalf("return patch size: got %d, want %d", got, len(code))
	}
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	if !bytes.Equal(proxy[8:len(code)], code[8:]) {
		t.Fatalf("bytes after return were modified: got %x, want %x",
			proxy[8:len(code)], code[8:])
	}
}

func TestBuildProxyRelocatesAUIPCPastBypassedReturn(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[0:], beqOffset(8))
	copy(code[4:], jalr(riscv64Zero, riscv64RA, 0))
	copy(code[8:], auipc(11, 1))

	if got := PatchSize(targetAddr, code, true); got != len(code) {
		t.Fatalf("bypassed-return patch size: got %d, want %d", got, len(code))
	}
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, source := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	if source.Len != 4 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("reachable AUIPC source: got %v, want JAL X0", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+8+1<<12); got != want {
		t.Fatalf("reachable AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestBuildPatchScansInternalCallTargetPastReturn(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
		hookAddr   = uintptr(0x600000)
	)
	code := nopCode(24)
	copy(code[8:], jal(riscv64RA, 8))
	copy(code[12:], jalr(riscv64Zero, riscv64RA, 0))
	copy(code[16:], auipc(11, 1))

	patchSize := PatchSize(targetAddr, code, true)
	proxy, _ := BuildPatch(targetAddr, proxyAddr, hookAddr, code[:patchSize])
	callVeneerAddr, _ := decodedJumpTarget(t, proxy[8:], proxyAddr+8)
	callVeneerOffset := int(callVeneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[callVeneerOffset+32:]), uint64(proxyAddr+16); got != want {
		t.Fatalf("internal callee target: got %#x, want %#x", got, want)
	}
	auipcVeneerAddr, _ := decodedJumpTarget(t, proxy[16:], proxyAddr+16)
	auipcVeneerOffset := int(auipcVeneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[auipcVeneerOffset+16:]), uint64(targetAddr+16+1<<12); got != want {
		t.Fatalf("internal callee AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestBuildProxyRelocatesBackwardReachableGap(t *testing.T) {
	const (
		targetAddr = uintptr(0x200000)
		proxyAddr  = uintptr(0x400000)
	)
	code := nopCode(24)
	copy(code[0:], jal(riscv64Zero, 16))
	copy(code[4:], auipc(11, 1))
	copy(code[16:], beqOffset(-12))

	if got := PatchSize(targetAddr, code, true); got != len(code) {
		t.Fatalf("backward-reachable patch size: got %d, want %d", got, len(code))
	}
	proxy := BuildProxy(targetAddr, proxyAddr, code)
	veneerAddr, source := decodedJumpTarget(t, proxy[4:], proxyAddr+4)
	if source.Len != 4 || source.Args[0].(riscv64asm.Reg) != riscv64asm.X0 {
		t.Fatalf("backward-reachable AUIPC source: got %v, want JAL X0", source)
	}
	veneerOffset := int(veneerAddr - proxyAddr)
	if got, want := binary.LittleEndian.Uint64(proxy[veneerOffset+16:]), uint64(targetAddr+4+1<<12); got != want {
		t.Fatalf("backward-reachable AUIPC value: got %#x, want %#x", got, want)
	}
}

func TestCollectReturnLandingsSortsWorklistBlocks(t *testing.T) {
	code := nopCode(28)
	copy(code[0:], jal(riscv64Zero, 16))
	copy(code[4:], jal(riscv64RA, 64))
	copy(code[8:], jal(riscv64Zero, 16))
	copy(code[16:], jal(riscv64RA, 64))
	copy(code[20:], beqOffset(-16))
	copy(code[24:], jalr(riscv64Zero, riscv64RA, 0))

	landings := collectReturnLandings(code)
	if len(landings) != 2 {
		t.Fatalf("return landing count: got %d, want 2", len(landings))
	}
	for index, want := range []int{8, 20} {
		if got := landings[index].start; got != want {
			t.Fatalf("return landing %d: got +%d, want +%d", index, got, want)
		}
	}
}

func TestDisassembleBranchToCutoffBypassesReturn(t *testing.T) {
	code := nopCode(32)
	copy(code[0:], beqOffset(32))
	copy(code[4:], jalr(riscv64Zero, riscv64RA, 0))

	if got := Disassemble(code, 8, true); got != len(code) {
		t.Fatalf("branch-to-cutoff cutting point: got %d, want %d", got, len(code))
	}
}

func TestDisassembleTailJumpMayLandAtCutoff(t *testing.T) {
	code := nopCode(24)
	copy(code[0:], jal(riscv64Zero, 24))

	if got := Disassemble(code, len(code), true); got != len(code) {
		t.Fatalf("tail-jump cutting point: got %d, want %d", got, len(code))
	}
}

func TestDisassembleCallToCutoffDoesNotHideShortFunction(t *testing.T) {
	code := nopCode(24)
	copy(code[0:], jal(riscv64RA, 24))
	copy(code[4:], jalr(riscv64Zero, riscv64RA, 0))

	assertPanicContains(t, "function is too short to patch", func() {
		Disassemble(code, len(code), true)
	})
}
