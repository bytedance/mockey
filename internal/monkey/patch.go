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

type replaceTargetFunc func(uintptr, []byte, []byte) (mem.ReplaceState, error)

func invokeReplaceTarget(replaceTarget replaceTargetFunc, target uintptr, replacement, original []byte) (state mem.ReplaceState, panicked bool, recovered interface{}, err error) {
	panicked = true
	defer func() {
		recovered = recover()
	}()

	state, err = replaceTarget(target, replacement, original)
	panicked = false
	return
}

type failedPatch struct {
	patch *Patch
	cause interface{}
}

var patchRegistry = struct {
	sync.Mutex
	byTarget       map[uintptr][]*Patch
	failedByTarget map[uintptr]*failedPatch
}{
	byTarget:       make(map[uintptr][]*Patch),
	failedByTarget: make(map[uintptr]*failedPatch),
}

// IsPatchTargetPoisoned reports whether a failed replacement left target in an
// uncertain state. Callers must retain all hook and proxy references while it is true.
func IsPatchTargetPoisoned(target uintptr) bool {
	patchRegistry.Lock()
	defer patchRegistry.Unlock()
	_, poisoned := patchRegistry.failedByTarget[target]
	return poisoned
}

// Patch is a context that holds the address and original codes of the patched function.
type Patch struct {
	size          int
	code          []byte
	original      []byte
	installed     []byte
	base          uintptr
	hook          reflect.Value
	proxy         reflect.Value
	previousProxy reflect.Value
}

// Base returns the address of the patched function.
func (p *Patch) Base() uintptr {
	return p.base
}

// Unpatch restores the patched function to the original function.
func (p *Patch) Unpatch() {
	p.unpatch(mem.ReplaceWithSTW, inst.ReleaseProxy)
}

func (p *Patch) unpatch(replaceTarget replaceTargetFunc, releaseProxy func([]byte)) {
	patchRegistry.Lock()
	defer patchRegistry.Unlock()

	if failed, poisoned := patchRegistry.failedByTarget[p.base]; poisoned {
		runtime.KeepAlive(failed.patch)
		tool.Assert(false, "patch target %#x is in an uncertain state after: %v", p.base, failed.cause)
	}

	stack := patchRegistry.byTarget[p.base]
	tool.Assert(
		len(stack) > 0 && stack[len(stack)-1] == p,
		"patches at %#x must be unpatched in reverse creation order",
		p.base,
	)

	state, panicked, recovered, err := invokeReplaceTarget(replaceTarget, p.base, p.original[:p.size], p.installed)
	if panicked {
		patchRegistry.failedByTarget[p.base] = &failedPatch{patch: p, cause: recovered}
		panic(recovered)
	}
	switch state {
	case mem.ReplaceApplied:
		if err != nil {
			patchRegistry.failedByTarget[p.base] = &failedPatch{patch: p, cause: err}
			tool.Assert(false, "restoring patch at %#x returned an inconsistent error: %v", p.base, err)
			return
		}
	case mem.ReplaceRestored:
		tool.Assert(err != nil, "restoring patch at %#x failed without an error", p.base)
		tool.Assert(false, "restore patch at %#x failed; installed bytes were restored: %v", p.base, err)
		return
	case mem.ReplaceUncertain:
		patchRegistry.failedByTarget[p.base] = &failedPatch{patch: p, cause: err}
		tool.Assert(err != nil, "restoring patch at %#x became uncertain without an error", p.base)
		tool.Assert(false, "restore patch at %#x left the target in an uncertain state: %v", p.base, err)
		return
	default:
		patchRegistry.failedByTarget[p.base] = &failedPatch{patch: p, cause: err}
		tool.Assert(false, "restore patch at %#x returned invalid state %d", p.base, state)
		return
	}

	p.proxy.Elem().Set(p.previousProxy)
	releaseProxy(p.code)
	if len(stack) == 1 {
		delete(patchRegistry.byTarget, p.base)
	} else {
		patchRegistry.byTarget[p.base] = stack[:len(stack)-1]
	}
	p.code = nil
	p.installed = nil
	p.hook = reflect.Value{}
	p.proxy = reflect.Value{}
	p.previousProxy = reflect.Value{}
}

// PatchValue replace the target function with a hook function, and stores the target function in the proxy function
// for future restore. Target and hook are values of function. Proxy is a value of proxy function pointer.
func PatchValue(target, hook, proxy reflect.Value, unsafe bool) *Patch {
	return patchValue(target, hook, proxy, unsafe, mem.ReplaceWithSTW, inst.ReleaseProxy)
}

func patchValue(target, hook, proxy reflect.Value, unsafe bool, replaceTarget replaceTargetFunc, releaseProxy func([]byte)) *Patch {
	tool.Assert(hook.Kind() == reflect.Func, "'%s' is not a function", hook.Kind())
	tool.Assert(proxy.Kind() == reflect.Ptr, "'%v' is not a function pointer", proxy.Kind())
	tool.Assert(proxy.Elem().Kind() == reflect.Func, "'%v' is not a function pointer", proxy.Elem().Kind())

	targetAddr := target.Pointer()
	patchRegistry.Lock()
	defer patchRegistry.Unlock()
	if failed, poisoned := patchRegistry.failedByTarget[targetAddr]; poisoned {
		runtime.KeepAlive(failed.patch)
		tool.Assert(false, "patch target %#x is in an uncertain state after: %v", targetAddr, failed.cause)
	}

	targetCodeBuf := common.BytesOf(targetAddr, inst.MaxPatchSize())
	cuttingIdx := inst.PatchSize(targetAddr, targetCodeBuf, !unsafe)
	originalCode := append([]byte(nil), targetCodeBuf[:cuttingIdx]...)
	previousProxy := reflect.New(proxy.Elem().Type()).Elem()
	previousProxy.Set(proxy.Elem())

	proxyCode := inst.AllocateProxy(targetAddr)
	releaseOnFailure := true
	proxyInjected := false
	defer func() {
		if !releaseOnFailure {
			return
		}
		if proxyInjected {
			proxy.Elem().Set(previousProxy)
		}
		releaseProxy(proxyCode)
	}()

	proxyAddr := common.PtrOf(proxyCode)
	proxyBody, targetPatch := inst.BuildPatch(
		targetAddr,
		proxyAddr,
		common.PtrAt(hook),
		originalCode,
	)
	tool.Assert(len(proxyBody) <= len(proxyCode), "proxy code is too large: %d > %d", len(proxyBody), len(proxyCode))
	tool.Assert(len(targetPatch) <= len(originalCode), "target patch is too large: %d > %d", len(targetPatch), len(originalCode))
	copy(proxyCode, proxyBody)
	installedCode := append([]byte(nil), originalCode...)
	copy(installedCode, targetPatch)
	tool.DebugPrintf(
		"PatchValue: target addr(0x%x), proxy addr(%p), target patch len(%v)\n",
		targetAddr,
		&proxyCode[0],
		len(installedCode),
	)

	patch := &Patch{
		size:          cuttingIdx,
		code:          proxyCode,
		original:      originalCode,
		installed:     installedCode,
		base:          targetAddr,
		hook:          hook,
		proxy:         proxy,
		previousProxy: previousProxy,
	}
	fn.InjectInto(proxy, proxyCode)
	proxyInjected = true

	state, panicked, recovered, err := invokeReplaceTarget(replaceTarget, targetAddr, installedCode, originalCode)
	if panicked {
		patchRegistry.failedByTarget[targetAddr] = &failedPatch{patch: patch, cause: recovered}
		releaseOnFailure = false
		panic(recovered)
	}
	switch state {
	case mem.ReplaceApplied:
		if err != nil {
			patchRegistry.failedByTarget[targetAddr] = &failedPatch{patch: patch, cause: err}
			releaseOnFailure = false
			tool.Assert(false, "installing patch at %#x returned an inconsistent error: %v", targetAddr, err)
			return nil
		}
		patchRegistry.byTarget[targetAddr] = append(patchRegistry.byTarget[targetAddr], patch)
		releaseOnFailure = false
		return patch
	case mem.ReplaceRestored:
		tool.Assert(err != nil, "installing patch at %#x failed without an error", targetAddr)
		tool.Assert(false, "install patch at %#x failed; original bytes were restored: %v", targetAddr, err)
		return nil
	case mem.ReplaceUncertain:
		patchRegistry.failedByTarget[targetAddr] = &failedPatch{patch: patch, cause: err}
		releaseOnFailure = false
		tool.Assert(err != nil, "installing patch at %#x became uncertain without an error", targetAddr)
		tool.Assert(false, "install patch at %#x left the target in an uncertain state: %v", targetAddr, err)
		return nil
	default:
		patchRegistry.failedByTarget[targetAddr] = &failedPatch{patch: patch, cause: err}
		releaseOnFailure = false
		tool.Assert(false, "install patch at %#x returned invalid state %d", targetAddr, state)
		return nil
	}
}

func PatchFunc(fn, hook, proxy interface{}, unsafe bool) *Patch {
	vv := reflect.ValueOf(fn)
	tool.Assert(vv.Kind() == reflect.Func, "'%v' is not a function", fn)
	return PatchValue(vv, reflect.ValueOf(hook), reflect.ValueOf(proxy), unsafe)
}
