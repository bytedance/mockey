/*
 * Copyright 2026 ByteDance Inc.
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

package mockey

import (
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"

	monkeyFn "github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/tool"
)

type originalInvocation struct {
	owner  *Mocker
	proxy  reflect.Value
	parent *originalInvocation
}

var originalCalls = struct {
	sync.Mutex
	byGoroutine map[int64]*originalInvocation
}{byGoroutine: make(map[int64]*originalInvocation)}

var (
	originalCallCount int64
	callOriginalEntry = reflect.ValueOf((*Mocker).callOriginal).Pointer()
)

// callOriginal provides a recognizable caller frame and tracks the exact
// proxy being invoked. The per-goroutine stack only disambiguates retries;
// it does not suppress recursive calls made by an original function.
//
//go:noinline
func (mocker *Mocker) callOriginal(proxy reflect.Value, args []reflect.Value) []reflect.Value {
	goroutine := tool.GetGoroutineID()
	originalCalls.Lock()
	invocation := &originalInvocation{owner: mocker, proxy: proxy, parent: originalCalls.byGoroutine[goroutine]}
	originalCalls.byGoroutine[goroutine] = invocation
	originalCalls.Unlock()
	atomic.AddInt64(&originalCallCount, 1)
	defer func() {
		originalCalls.Lock()
		if invocation.parent == nil {
			delete(originalCalls.byGoroutine, goroutine)
		} else {
			originalCalls.byGoroutine[goroutine] = invocation.parent
		}
		originalCalls.Unlock()
		atomic.AddInt64(&originalCallCount, -1)
	}()
	return tool.ReflectCall(proxy, args)
}

// originalStackRetry recognizes only a stack-growth retry that returns from
// an original proxy straight into the currently installed entry hook. A real
// recursive call has its original body (or user callback) as the nearer caller.
//
//go:noinline
func (mocker *Mocker) originalStackRetry() *originalInvocation {
	if atomic.LoadInt64(&originalCallCount) == 0 {
		return nil
	}
	goroutine := tool.GetGoroutineID()
	originalCalls.Lock()
	invocation := originalCalls.byGoroutine[goroutine]
	originalCalls.Unlock()
	if invocation == nil || mocker.patch == nil {
		return nil
	}
	if invocation.owner.builder.analyzer.RuntimeTargetValue().Pointer() != mocker.builder.analyzer.RuntimeTargetValue().Pointer() {
		return nil
	}
	if !mocker.patch.IsCurrent() {
		return nil
	}
	var pcs [32]uintptr
	// Skip runtime.Callers, this detector, and the immediate mock hook.
	count := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:count])
	for {
		frame, more := frames.Next()
		if !originalCallBridge(frame.Function) {
			if frame.Entry == callOriginalEntry {
				return invocation
			}
			return nil
		}
		if !more {
			return nil
		}
	}
}

func originalCallBridge(name string) bool {
	if len(name) >= len(".abi0") && name[len(name)-len(".abi0"):] == ".abi0" {
		name = name[:len(name)-len(".abi0")]
	}
	switch name {
	case "reflect.callReflect", "reflect.makeFuncStub", "reflect.Value.call", "reflect.Value.Call", "reflect.Value.CallSlice",
		"runtime.reflectcall", "github.com/bytedance/mockey/internal/tool.ReflectCall":
		return true
	}
	if len(name) >= len("runtime.call") && name[:len("runtime.call")] == "runtime.call" {
		suffix := name[len("runtime.call"):]
		if suffix == "" {
			return false
		}
		for _, character := range suffix {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func (mocker *Mocker) resumeOriginal(invocation *originalInvocation, args []reflect.Value) []reflect.Value {
	proxy := invocation.proxy
	if proxy.Type() != mocker.builder.runtimeTargetType() {
		// A newer generic instantiation can share the retried entry with an
		// older layer. As in normal generic mismatch dispatch, use the current
		// hook's ABI view of the saved proxy so its reflected results match.
		proxy = monkeyFn.MakeFunc(mocker.builder.runtimeTargetType(), proxy.Pointer())
	}
	return invocation.owner.callOriginal(proxy, args)
}
