//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

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

package fn

import (
	"reflect"
	"runtime"
	"strings"
	"unsafe"

	monkeyFn "github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/tool"
)

func (a *AnalyzerImpl) initGenericMethodClosure() {
	if a.genericIn != nil && !*a.genericIn {
		return
	}
	a.nameAnalyzer = NewNameAnalyzerByValue(a.targetValue)
	if !a.nameAnalyzer.IsAnonymousFormat() {
		return
	}
	f := runtime.FuncForPC(a.targetValue.Pointer())
	// runtime._func.funcID is at byte 40 in Go 1.27. FuncIDWrapper is 23.
	// Check the compiler's marker as well as the name: a user-written closure
	// that happens to call a generic method must remain an ordinary function.
	const funcIDOffset, funcIDWrapper = 40, 23
	if *(*uint8)(unsafe.Add(unsafe.Pointer(f), funcIDOffset)) != funcIDWrapper {
		return
	}

	entry, _ := inst.GetGenericAddr(a.targetValue.Pointer(), 10000)
	callee := NewNameAnalyzer(runtime.FuncForPC(entry).Name(), false)
	if !callee.IsGeneric() || !callee.HasMiddleName() || callee.IsAnonymousFormat() {
		return
	}
	captureOffset := inst.GenericClosureCaptureOffset(a.targetValue.Pointer(), 10000)
	closure := &genericMethodClosure{entry: entry}
	if captureOffset == 8 && a.targetType.NumIn() > 0 && methodReceiverMatches(a.targetType.In(0), callee) {
		closure.receiver = a.targetType.In(0)
	} else {
		// Pointer method values capture the receiver followed by the dictionary.
		// unsafe.Pointer has the same ABI and pointer map as every *T and lets
		// us adapt the bound signature without guessing T from its runtime name.
		tool.Assert(callee.IsPtrReceiver() && captureOffset == 16,
			"generic method value or promoted receiver is unsupported; use the declared receiver's method expression")
		closure.bound = true
		closure.receiver = reflect.TypeOf(unsafe.Pointer(nil))
	}
	// A function value starts with its entry PC. Synthetic method-expression
	// closures capture just the dictionary; pointer method values capture it
	// after the receiver. Keep the reflect.Value alive while reading funcval.
	type functionValue struct {
		typ  unsafe.Pointer
		data unsafe.Pointer
	}
	value := a.targetValue.Interface()
	context := (*functionValue)(unsafe.Pointer(&value)).data
	closure.dictionary = GenericInfo(*(*uintptr)(unsafe.Add(context, captureOffset)))
	runtime.KeepAlive(value)
	a.genericClosure = closure
}

func methodReceiverMatches(receiver reflect.Type, callee *NameAnalyzer) bool {
	pointer := receiver.Kind() == reflect.Pointer
	if pointer {
		receiver = receiver.Elem()
	}
	name := receiver.Name()
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	if pointer {
		name = "(*" + name + ")"
	}
	return receiver.PkgPath() == callee.PkgName() && name == callee.MiddleName()
}

func (a *AnalyzerImpl) genericClosureRuntimeTarget() (reflect.Value, GenericInfo, bool) {
	if a.genericClosure == nil {
		return reflect.Value{}, 0, false
	}
	return monkeyFn.MakeFunc(a.runtimeTargetType, a.genericClosure.entry), a.genericClosure.dictionary, true
}
