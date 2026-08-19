//go:build darwin && arm64

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

package monkey

import (
	"reflect"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/monkey/mem"
	"github.com/bytedance/mockey/internal/tool"
)

// useShortPatch reports whether the target is too short for the long-jump
// entry sequence and should therefore be patched with a near trampoline.
//
// On platforms without a short-patch strategy this is always false, so
// PatchValue keeps its previous behaviour byte for byte: the target still
// reaches Disassemble and still fails loudly with "function is too short to
// patch". See patch_short_stub.go.
func useShortPatch(addr uintptr, required int) bool {
	return inst.TooShortToPatch(common.BytesOf(addr, 64), required)
}

// PatchValueShort is PatchValue with a near-trampoline entry stub: it writes a
// single 4-byte `B` at the function entry instead of the 24-byte
// MOVZ/MOVK*3/LDR/BR sequence, so functions as small as one instruction can be
// patched.
//
// Layout:
//
//	target entry : B  -> trampoline           (4 bytes, the only thing overwritten)
//	trampoline   : MOVZ/MOVK x26, &hookfn     (24 bytes, on a page within +/-128MB)
//	               LDR x19,[x26]; BR x19
//	proxy page   : <copied original 4 bytes>; MOVZ/... x26, target+4; BR x26
func PatchValueShort(target, hook, proxy reflect.Value, unsafe bool) *Patch {
	tool.Assert(hook.Kind() == reflect.Func, "'%s' is not a function", hook.Kind())
	tool.Assert(proxy.Kind() == reflect.Ptr, "'%v' is not a function pointer", proxy.Kind())

	targetAddr := target.Pointer()

	// 1. trampoline slot near the target, holding the full long-jump sequence.
	//    Slots are carved out of shared near pages, so one page serves many
	//    patches. Because the page is shared, other slots on it may already be
	//    live code, so the write goes through the same stop-the-world path used
	//    to patch any other live code rather than toggling page permissions.
	//
	//    Slots are deliberately never reclaimed. Unpatch restores the entry but
	//    cannot know whether another thread is at that moment executing inside
	//    the trampoline, and a slot is 32 bytes: leaking it is far cheaper than
	//    freeing memory that is still being executed.
	tramp := common.AllocTrampoline(targetAddr, inst.ShortBranchRange)
	tool.Assert(tramp != nil, "allocate near trampoline failed for target 0x%x", targetAddr)
	trampAddr := common.PtrOf(tramp)
	mem.WriteWithSTW(trampAddr, inst.BranchInto(common.PtrAt(hook)))

	// 2. 4-byte entry stub
	stub, ok := inst.ShortBranchTo(targetAddr, trampAddr)
	tool.Assert(ok, "trampoline 0x%x out of B range from 0x%x", trampAddr, targetAddr)

	// 3. proxy: original instruction(s) + jump back. 4 bytes is exactly one
	//    arm64 instruction, so no instruction is ever cut in half.
	//    The copied instruction is relocated, so it must not be PC-relative:
	//    an ADRP/B/CBZ moved to another page silently computes the wrong thing.
	cuttingIdx := inst.ShortBranchLen
	targetCodeBuf := common.BytesOf(targetAddr, cuttingIdx)
	proxyCode := common.AllocatePage()
	orig := make([]byte, cuttingIdx)
	copy(orig, targetCodeBuf)
	relocated, ok2, why := inst.RelocateFirstInst(targetCodeBuf, targetAddr, common.PtrOf(proxyCode))
	tool.Assert(ok2, "cannot patch short function: %s", why)
	copy(proxyCode, relocated)
	copy(proxyCode[cuttingIdx:], inst.BranchTo(targetAddr+uintptr(cuttingIdx)))
	fn.InjectInto(proxy, proxyCode)

	// 4. overwrite the entry
	mem.WriteWithSTW(targetAddr, stub)

	return &Patch{base: targetAddr, code: proxyCode, original: orig}
}
