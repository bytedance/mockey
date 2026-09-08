/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to bound Go 1.27 trampoline instruction reads.
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
	"sort"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/monkey/mem"
	"github.com/bytedance/mockey/internal/tool"
)

// Patch is a context that holds the address and original codes of the patched function.
type Patch struct {
	code      []byte
	base      uintptr
	original  []byte
	entryCode []byte
	hook      reflect.Value // Machine-code pointers do not keep the hook closure alive for GC.
}

// Base returns the address of the patched function.
func (p *Patch) Base() uintptr {
	return p.base
}

// Unpatch restores the patched function to the original function.
func (p *Patch) Unpatch() {
	mem.WriteWithSTW(p.base, p.original)
	common.ReleasePage(p.code)
}

// PatchValue replace the target function with a hook function, and stores the target function in the proxy function
// for future restore. Target and hook are values of function. Proxy is a value of proxy function pointer.
func PatchValue(target, hook, proxy reflect.Value, unsafe bool) *Patch {
	tool.Assert(hook.Kind() == reflect.Func, "'%s' is not a function", hook.Kind())
	tool.Assert(proxy.Kind() == reflect.Ptr, "'%v' is not a function pointer", proxy.Kind())

	targetAddr := target.Pointer()
	// Bound instruction reads by the runtime function table. ARM64 also needs
	// to inspect backward branches after the prefix to keep initialization
	// loops together when constructing the Origin trampoline.
	codeSize := targetCodeSize(targetAddr)
	targetCodeBuf := common.BytesOf(targetAddr, codeSize)
	// construct the branch instruction, i.e. jump to the hook function
	hookCode := inst.BranchInto(common.PtrAt(hook))
	// construct the proxy code
	proxyCode := common.AllocatePage()
	installed := false
	defer func() {
		if !installed {
			common.ReleasePage(proxyCode)
		}
	}()
	tool.DebugPrintf("PatchValue: target addr(0x%x), proxy addr(%p), hook code len(%v)\n", targetAddr, &proxyCode[0], len(hookCode))
	// search the cutting point of the target code, i.e. the minimum length of full instructions that is longer than the hookCode
	cuttingIdx := inst.Disassemble(targetCodeBuf, len(hookCode), !unsafe)
	// MockUnsafe deliberately permits a hook to overlap a short function's
	// boundary. Save exactly the bytes it will overwrite so Unpatch restores
	// them, while keeping the instruction scan within the function itself.
	if cuttingIdx > len(targetCodeBuf) {
		tool.Assert(unsafe, "function is too short to patch")
		targetCodeBuf = common.BytesOf(targetAddr, cuttingIdx)
	}
	// save the original code before the cutting point
	branchBack := inst.BranchToOriginal(targetAddr + uintptr(cuttingIdx))
	tool.Assert(cuttingIdx+len(branchBack) <= len(proxyCode), "initialization loop is too large to patch")
	copy(proxyCode, targetCodeBuf[:cuttingIdx])
	// construct the branch instruction, i.e. jump to the cutting point
	copy(proxyCode[cuttingIdx:], branchBack)
	original := append([]byte(nil), targetCodeBuf[:cuttingIdx]...)
	relocationEnd := cuttingIdx
	if relocationEnd > codeSize {
		relocationEnd = codeSize
	}
	inst.RelocateBranches(proxyCode, targetAddr, relocationEnd, cuttingIdx+len(branchBack))
	// inject the proxy code to the proxy function
	fn.InjectInto(proxy, proxyCode)
	// replace target function codes before the cutting point
	mem.WriteWithSTW(targetAddr, hookCode)
	installed = true

	return &Patch{base: targetAddr, code: proxyCode, original: original, entryCode: hookCode, hook: hook}
}

func PatchFunc(fn, hook, proxy interface{}, unsafe bool) *Patch {
	vv := reflect.ValueOf(fn)
	tool.Assert(vv.Kind() == reflect.Func, "'%v' is not a function", fn)
	return PatchValue(vv, reflect.ValueOf(hook), reflect.ValueOf(proxy), unsafe)
}

// targetCodeSize queries metadata rather than reading past a function while
// looking for its end. runtime.FuncForPC includes dynamically loaded modules,
// unlike the main module's function list used for symbol-name discovery.
func targetCodeSize(entry uintptr) int {
	function := runtime.FuncForPC(entry)
	tool.Assert(function != nil && function.Entry() == entry, "target function bounds not found")
	outside := func(offset int) bool {
		function := runtime.FuncForPC(entry + uintptr(offset))
		return function == nil || function.Entry() != entry
	}
	limit := 64
	if runtime.GOARCH == "arm64" {
		for !outside(limit) {
			tool.Assert(limit < 1<<24, "target function is too large to analyze")
			limit *= 2
		}
	}
	return sort.Search(limit, outside)
}

// IsCurrent reports whether this patch owns the function's current entry.
// Older layered patches remain callable through proxies but do not own it.
func (p *Patch) IsCurrent() bool {
	current := common.BytesOf(p.base, len(p.entryCode))
	for index, instruction := range p.entryCode {
		if current[index] != instruction {
			return false
		}
	}
	return true
}
