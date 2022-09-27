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

// Patch is a context that holds the address and original codes of the patched function.
type Patch struct {
	size int
	code []byte
	base uintptr
}

// Unpatch restores the patched function to the original function.
func (p *Patch) Unpatch() {
	mem.WriteWithSTW(p.base, p.code[:p.size])
	common.ReleasePage(p.code)
}

// PatchValue replace the target function with a hook function, and stores the target function in the proxy function
// for future restore. Target and hook are values of function. Proxy is a value of proxy function pointer.
func PatchValue(target reflect.Value, hook reflect.Value, proxy reflect.Value) *Patch {
	tool.Assert(hook.Kind() == reflect.Func, "'%s' is not a function", hook.Kind())
	tool.Assert(proxy.Kind() == reflect.Ptr, "'%v' is not a function pointer", proxy.Kind())
	tool.Assert(hook.Type() == target.Type(), "'%v' and '%s' mismatch", hook.Type(), target.Type())
	tool.Assert(proxy.Elem().Type() == target.Type(), "'*%v' and '%s' mismatch", proxy.Elem().Type(), target.Type())

	targetAddr := target.Pointer()
	// The first few bytes of the target function code
	const bufSize = 64
	targetCodeBuf := common.BytesOf(targetAddr, bufSize)
	// construct the branch instruction, i.e. jump to the hook function
	hookCode := inst.BranchInto(common.PtrAt(hook))
	// search the cutting point of the target code, i.e. the minimum length of full instructions that is longer than the hookCode
	cuttingIdx := inst.Disassemble(targetCodeBuf, len(hookCode))

	// construct the proxy code
	proxyCode := common.AllocatePage()
	// save the original code before the cutting point
	copy(proxyCode, targetCodeBuf[:cuttingIdx])
	// construct the branch instruction, i.e. jump to the cutting point
	copy(proxyCode[cuttingIdx:], inst.BranchTo(targetAddr+uintptr(cuttingIdx)))
	// inject the proxy code to the proxy function
	fn.InjectInto(proxy, proxyCode)

	tool.DebugPrintf("PatchValue: hook code len(%v), cuttingIdx(%v)\n", len(hookCode), cuttingIdx)

	// replace target function codes before the cutting point
	mem.WriteWithSTW(targetAddr, hookCode)

	return &Patch{base: targetAddr, code: proxyCode, size: cuttingIdx}
}

func PatchFunc(fn interface{}, hook interface{}, proxy interface{}) *Patch {
	vv := reflect.ValueOf(fn)
	tool.Assert(vv.Kind() == reflect.Func, "'%v' is not a function", fn)
	return PatchValue(vv, reflect.ValueOf(hook), reflect.ValueOf(proxy))
}

func PatchMethod(val interface{}, method string, hook interface{}, proxy interface{}) *Patch {
	m, ok := reflect.TypeOf(val).MethodByName(method)
	tool.Assert(ok, "unknown method '%s'", method)
	return PatchValue(m.Func, reflect.ValueOf(hook), reflect.ValueOf(proxy))
}
