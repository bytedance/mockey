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
	"runtime"
	"sync"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/monkey/mem"
	"github.com/bytedance/mockey/internal/tool"
)

// Patch is a context that holds the address and original codes of the patched function.
type Patch struct {
	code []byte
	// original holds the exact bytes that were at the entry before patching.
	//
	// The legacy path could restore from code[:size] because it copies the
	// original instructions into the trampoline verbatim. Neither short path
	// can: the arm64 near-trampoline path and the amd64 E9 path both *relocate*
	// the displaced instructions before storing them, so the trampoline copy is
	// no longer byte-identical to what was at the entry. Recording the originals
	// unconditionally makes Unpatch correct on every path and removes the need
	// for a separate size field.
	original []byte
	base     uintptr
	// hook keeps the function value whose address is embedded in a short relay
	// reachable after Unpatch. See retiredRelays.
	hook interface{}
	// shortBranch records that the target's entry is a five-byte E9 into a relay
	// inside code, which changes who may free that page. See Unpatch.
	shortBranch bool
}

type retiredRelay struct {
	code []byte
	// hook is deliberately a Go reference, rather than the raw address in code.
	// The garbage collector does not scan executable bytes for pointers.
	hook interface{}
}

// retiredRelays keeps relay pages of short-branch patches mapped for the rest
// of the process. It also retains each relay's hook function value: a goroutine
// which already executed E9 before Unpatch can still load that value from the
// relay after the caller releases its Mocker.
var retiredRelays struct {
	sync.Mutex
	pages []retiredRelay
}

// Base returns the address of the patched function.
func (p *Patch) Base() uintptr {
	return p.base
}

// Unpatch restores the patched function to the original function.
func (p *Patch) Unpatch() {
	mem.WriteWithSTW(p.base, p.original)
	if p.shortBranch {
		// Do not unmap. On the legacy path the entry is MOVABS RDX, hook; JMP [RDX],
		// which goes straight to the hook, so this page is only ever reached through
		// the proxy the caller holds. The short branch changes that: the entry is a
		// five-byte E9 into a relay that lives in this very page, so EVERY call to
		// the target runs through it. A goroutine that has already executed the E9
		// when Unpatch runs is left executing an unmapped page, which the runtime
		// reports as an unrecoverable "unexpected fault address <page>+0x20".
		//
		// Restoring the entry bytes above does not close the window: the jump has
		// already happened. Nothing short of knowing that no thread is inside the
		// relay would make freeing safe, and there is no cheap way to know that, so
		// retire the page instead. It stays mapped and is never reused, costing one
		// 4 KiB page per short-branch patch for the life of the process.
		retiredRelays.Lock()
		retiredRelays.pages = append(retiredRelays.pages, retiredRelay{code: p.code, hook: p.hook})
		retiredRelays.Unlock()
		return
	}
	common.ReleasePage(p.code)
}

// PatchValue replace the target function with a hook function, and stores the target function in the proxy function
// for future restore. Target and hook are values of function. Proxy is a value of proxy function pointer.
func PatchValue(target, hook, proxy reflect.Value, unsafe bool) *Patch {
	tool.Assert(hook.Kind() == reflect.Func, "'%s' is not a function", hook.Kind())
	tool.Assert(proxy.Kind() == reflect.Ptr, "'%v' is not a function pointer", proxy.Kind())

	targetAddr := target.Pointer()
	// Targets too small to hold the long-jump entry sequence get a 4-byte
	// branch to a nearby trampoline instead. On platforms without that
	// strategy this is always false and the code below is unchanged.
	//
	// `unsafe` callers are excluded on purpose: passing unsafe=true tells
	// Disassemble to ignore the RET it finds and patch anyway, because the
	// caller knows the bytes after the RET still belong to the target (this is
	// how generic function analysis patches a shared instantiation stub).
	// Rerouting those to the short path would honour a RET the caller
	// explicitly asked us to disregard, changing established behaviour.
	if !unsafe && useShortPatch(targetAddr, len(inst.BranchInto(0))) {
		return PatchValueShort(target, hook, proxy, unsafe)
	}
	// The first few bytes of the target function code
	const bufSize = 64
	targetCodeBuf := common.BytesOf(targetAddr, bufSize)
	hookAddr := common.PtrAt(hook)
	hookCode := inst.BranchInto(hookAddr)
	var cuttingIdx int
	var proxyCode []byte
	var proxyPrefix []byte
	var usedShortBranch bool

	if unsafe || inst.ShortBranchSize() == 0 {
		// Keep MockUnsafe and non-amd64 architectures on the legacy path.
		cuttingIdx = disassembleOrExplain(targetAddr, targetCodeBuf, len(hookCode), !unsafe)
		proxyCode = common.AllocatePage()
		proxyPrefix = targetCodeBuf[:cuttingIdx]
	} else if idx, ok := inst.TryDisassemble(targetCodeBuf, len(hookCode), true); ok && !inst.HasPCRelative(targetCodeBuf[:idx]) {
		// Preserve the existing 12-byte entry sequence when its trampoline prefix
		// is position independent.
		cuttingIdx = idx
		proxyCode = common.AllocatePage()
		proxyPrefix = targetCodeBuf[:cuttingIdx]
	} else if shortCode, shortIdx, shortPrefix, shortPage, ok := tryShortBranch(targetAddr, targetCodeBuf, hookAddr); ok {
		// A five-byte E9 reaches a nearby relay. The relay restores the function
		// value pointer in RDX before entering the reflect-generated hook.
		hookCode, cuttingIdx, proxyPrefix, proxyCode = shortCode, shortIdx, shortPrefix, shortPage
		usedShortBranch = true
	} else {
		// The short path is unavailable (target too short for a five-byte cut, no
		// page within rel32 range, or a prefix we cannot relocate). Fall back to
		// the legacy 12-byte path so that anything patchable before this change
		// stays patchable: this path may only ever add capability, never remove
		// it. If the legacy path cannot handle it either, Disassemble rejects
		// loudly exactly as it did on the baseline.
		cuttingIdx = disassembleOrExplain(targetAddr, targetCodeBuf, len(hookCode), true)
		proxyCode = common.AllocatePage()
		proxyPrefix = targetCodeBuf[:cuttingIdx]
	}

	original := append([]byte(nil), targetCodeBuf[:cuttingIdx]...)
	// cuttingIdx is no longer guaranteed to equal len(hookCode): when the target
	// ends in an early RET, Disassemble rounds up to the next instruction boundary
	// past it, so cuttingIdx may exceed len(hookCode) (bounded by 12+15-1 = 26 on
	// amd64, one branch width plus at most one maximal x86 instruction). Both the
	// slice read below and the trampoline write must still fit, so pin the two
	// invariants rather than assume them: targetCodeBuf is bufSize (64) bytes, and
	// a page is at least 4096 bytes, so neither can fire today -- this is here to
	// fail loudly instead of corrupting memory if either bound ever changes.
	tool.Assert(cuttingIdx <= bufSize, "cutting index %v exceeds the %v bytes read from the target", cuttingIdx, bufSize)
	tool.Assert(len(proxyPrefix)+len(inst.BranchTo(0)) <= len(proxyCode), "trampoline (%v code bytes + branch) does not fit in a %v byte page", len(proxyPrefix), len(proxyCode))
	// save the original code before the cutting point
	copy(proxyCode, proxyPrefix)
	// construct the branch instruction, i.e. jump to the cutting point
	copy(proxyCode[len(proxyPrefix):], inst.BranchTo(targetAddr+uintptr(cuttingIdx)))
	// inject the proxy code to the proxy function
	fn.InjectInto(proxy, proxyCode)

	tool.DebugPrintf("PatchValue: hook code len(%v), cuttingIdx(%v)\n", len(hookCode), cuttingIdx)

	// Replace target function codes before the cutting point.
	//
	// Pad to the full cutting point rather than writing only the branch. The
	// cutting point is rounded up to an instruction boundary, so it is often
	// wider than the branch -- most visibly a five-byte E9 over a six-byte
	// CMP/JBE prologue. Bytes inside the cut that we leave alone are the tail of
	// an instruction whose head we just overwrote. Execution never reaches them,
	// because the branch leaves first, so the patch itself works either way.
	//
	// The reason this is not cosmetic is that mockey reads its own patched code
	// back: every subsequent PatchValue on the same function disassembles the
	// live entry to find its cutting point. An orphan tail makes that entry an
	// invalid instruction stream, and Disassemble rejects it with
	// "unrecognized instruction" -- naming neither the address nor the real
	// cause. That is reached whenever a mock is still live when the next one is
	// built, i.e. any mock that is never unpatched, which is the normal shape of
	// a Mock(...) at the top level of a test function.
	//
	// Unpatch always restored the whole cutting point, so it never contributed:
	// the entry has to be decodable *while patched*, not only after.
	hookCode = inst.PadEntry(hookCode, cuttingIdx)
	mem.WriteWithSTW(targetAddr, hookCode)

	return &Patch{base: targetAddr, code: proxyCode, original: original, hook: hook.Interface(), shortBranch: usedShortBranch}
}

func align(value, alignment int) int {
	return (value + alignment - 1) &^ (alignment - 1)
}

// diagnosticBytes is how much of the entry a refusal quotes. Sixteen covers any
// single x86 instruction (max 15) plus a byte of context, which is enough to
// tell a mangled entry from a genuinely unpatchable one at a glance.
const diagnosticBytes = 16

// disassembleOrExplain is inst.Disassemble with the target named in the failure.
//
// The bare refusals ("unrecognized instruction", "function is too short to
// patch") say nothing about which function was refused or what was actually
// read. That is a problem specific to this refusal: it is the one failure mode
// that can be caused by a *previous* mock rather than by the target itself, so
// the caller in the stack trace is frequently not the culprit. Without the
// address there is nothing to correlate against, and diagnosing it means
// bisecting the test order by hand.
//
// The address is printed both raw and resolved: raw is what the debug ledger
// (MOCKEY_DEBUG=true) prints, so the two can be matched up directly, and the
// symbol is what a reader actually recognises.
func disassembleOrExplain(targetAddr uintptr, code []byte, required int, checkLen bool) int {
	pos, err := inst.DisassembleErr(code, required, checkLen)
	if err == nil {
		return pos
	}
	name := "unknown function"
	if f := runtime.FuncForPC(targetAddr); f != nil {
		name = f.Name()
	}
	quoted := code
	if len(quoted) > diagnosticBytes {
		quoted = quoted[:diagnosticBytes]
	}
	tool.Assert(false, "%v: cannot patch %v at 0x%x (%v), entry reads % x. "+
		"If those bytes begin with a branch (E9 or 48 BA ... FF 22), this function is "+
		"already patched and was never unpatched: unpatch the earlier mock, or build "+
		"it inside PatchConvey so it is unpatched for you.",
		err, name, targetAddr, targetAddr, quoted)
	return 0 // unreachable: Assert(false) always panics
}

// tryShortBranch builds the five-byte E9 entry sequence plus its near relay
// page. It reports ok=false instead of panicking whenever the short path is
// unavailable, so the caller can fall back to the legacy 12-byte path: this
// feature may only ever add patchable functions, never take one away. Any page
// allocated on a failing path is released before returning.
func tryShortBranch(targetAddr uintptr, targetCodeBuf []byte, hookAddr uintptr) (hookCode []byte, cuttingIdx int, proxyPrefix, proxyCode []byte, ok bool) {
	cuttingIdx, ok = inst.TryDisassemble(targetCodeBuf, inst.ShortBranchSize(), true)
	if !ok {
		return nil, 0, nil, nil, false
	}

	proxyCode, err := common.AllocatePageNear(targetAddr)
	if err != nil {
		tool.DebugPrintf("tryShortBranch: allocate near trampoline failed: %v\n", err)
		return nil, 0, nil, nil, false
	}
	proxyAddr := common.PtrOf(proxyCode)
	proxyPrefix, err = inst.Relocate(targetCodeBuf[:cuttingIdx], targetAddr, proxyAddr)
	if err != nil {
		common.ReleasePage(proxyCode)
		tool.DebugPrintf("tryShortBranch: relocate trampoline failed: %v\n", err)
		return nil, 0, nil, nil, false
	}

	branchBack := inst.BranchTo(targetAddr + uintptr(cuttingIdx))
	relayOffset := align(len(proxyPrefix)+len(branchBack), 16)
	relayCode := inst.BranchInto(hookAddr)
	if relayOffset+len(relayCode) > len(proxyCode) {
		common.ReleasePage(proxyCode)
		tool.DebugPrintf("tryShortBranch: relay does not fit in proxy page\n")
		return nil, 0, nil, nil, false
	}
	copy(proxyCode[relayOffset:], relayCode)

	hookCode, ok = inst.BranchIntoShort(targetAddr, proxyAddr+uintptr(relayOffset))
	if !ok {
		common.ReleasePage(proxyCode)
		tool.DebugPrintf("tryShortBranch: near trampoline is outside rel32 range\n")
		return nil, 0, nil, nil, false
	}
	return hookCode, cuttingIdx, proxyPrefix, proxyCode, true
}

func PatchFunc(fn, hook, proxy interface{}, unsafe bool) *Patch {
	vv := reflect.ValueOf(fn)
	tool.Assert(vv.Kind() == reflect.Func, "'%v' is not a function", fn)
	return PatchValue(vv, reflect.ValueOf(hook), reflect.ValueOf(proxy), unsafe)
}
