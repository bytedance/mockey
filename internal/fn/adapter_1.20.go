//go:build go1.20
// +build go1.20

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

package fn

import (
	"reflect"

	"github.com/bytedance/mockey/internal/tool"
)

func NewAdapter(target interface{}, generic *bool, method *bool) Adapter {
	a := &AdapterImpl{
		target:    target,
		genericIn: generic,
		methodIn:  method,
	}
	return a.init()
}

type AdapterImpl struct {
	target    interface{}
	genericIn *bool
	methodIn  *bool

	generic            bool
	method             bool // DO NOT use it unless absolutely necessary
	targetType         reflect.Type
	extendedTargetType reflect.Type
}

// init initializes the AdapterImpl. If `a.genericIn` or `a.methodIn` is set, it will be used directly, else it will be
// determined by the Analyzer.
func (a *AdapterImpl) init() *AdapterImpl {
	tool.DebugPrintf("[AdapterImpl.init] genericIn: %v, methodIn: %v\n", a.genericIn, a.methodIn)
	var analyzer *Analyzer
	if a.genericIn == nil || a.methodIn == nil {
		analyzer = NewAnalyzer(a.target)
	}
	if a.genericIn == nil {
		a.generic = analyzer.IsGeneric()
	} else {
		a.generic = *a.genericIn
	}
	if a.methodIn == nil {
		a.method = analyzer.IsMethod()
	} else {
		a.method = *a.methodIn
	}

	a.targetType = reflect.TypeOf(a.target)
	a.extendedTargetType = a.extendedTargetType0()
	tool.DebugPrintf("[AdapterImpl.init] generic: %v, method: %v, extendedTargetType: %v\n", a.generic, a.method, a.extendedTargetType)
	return a
}

func (a *AdapterImpl) extendedTargetType0() reflect.Type {
	if !a.generic {
		return a.targetType
	}
	var (
		targetIn      []reflect.Type
		targetInShift int
		targetOut     []reflect.Type
	)
	if a.method {
		// for methods, generic information needs to be inserted at position 1 after go1.20
		targetIn = []reflect.Type{a.targetType.In(0), genericInfoType}
		targetInShift = 1
	} else {
		// for functions, generic information needs to be inserted at position 0
		targetIn = []reflect.Type{genericInfoType}
		targetInShift = 0
	}
	for i := targetInShift; i < a.targetType.NumIn(); i++ {
		targetIn = append(targetIn, a.targetType.In(i))
	}
	for i := 0; i < a.targetType.NumOut(); i++ {
		targetOut = append(targetOut, a.targetType.Out(i))
	}
	return reflect.FuncOf(targetIn, targetOut, a.targetType.IsVariadic())
}

func (a *AdapterImpl) Generic() bool {
	return a.generic
}

func (a *AdapterImpl) ExtendedTargetType() reflect.Type {
	return a.extendedTargetType
}

func (a *AdapterImpl) InputAdapter(inputName string, inputType reflect.Type) func([]reflect.Value) []reflect.Value {
	tool.Assert(inputType.Kind() == reflect.Func, "'%v' is not a function", inputType.Kind())
	targetType := a.ExtendedTargetType()
	tool.Assert(targetType.IsVariadic() == inputType.IsVariadic(), "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)

	// check:
	// 1. function:
	//     a. non-generic function: func(inArgs) outArgs
	//     b. generic function: func(info GenericInfo, inArgs) outArgs
	// 2. method:
	//     a. non-generic method: func(self *struct, inArgs) outArgs
	if tool.CheckFuncArgs(targetType, inputType, 0, 0) {
		return func(targetArgs []reflect.Value) []reflect.Value { return targetArgs }
	}
	// need to adapt the arguments.
	if a.generic {
		f, _ := a.genericAdapter(inputName, inputType)
		return f
	}
	f, _ := a.nonGenericAdapter(inputName, inputType)
	return f
}

func (a *AdapterImpl) ReversedInputAdapter(inputName string, inputType reflect.Type) func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
	tool.Assert(inputType.Kind() == reflect.Func, "'%v' is not a function", inputType.Kind())
	targetType := a.ExtendedTargetType()
	tool.Assert(targetType.IsVariadic() == inputType.IsVariadic(), "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)

	// check:
	// 1. function:
	//     a. non-generic function: func(inArgs) outArgs
	//     b. generic function: func(info GenericInfo, inArgs) outArgs
	// 2. method:
	//     a. non-generic method: func(self *struct, inArgs) outArgs
	if tool.CheckFuncArgs(targetType, inputType, 0, 0) {
		return func(inputArgs, extraArgs []reflect.Value) []reflect.Value { return inputArgs }
	}
	// need to adapt the arguments.
	if a.generic {
		_, rf := a.genericAdapter(inputName, inputType)
		return rf
	}
	_, rf := a.nonGenericAdapter(inputName, inputType)
	return rf
}

type (
	fn         = func(targetArgs []reflect.Value) []reflect.Value
	reversedFn = func(inputArgs, extraArgs []reflect.Value) []reflect.Value
)

func (a *AdapterImpl) nonGenericAdapter(inputName string, inputType reflect.Type) (fn, reversedFn) {
	// check:
	// 1. method:
	//     a. non-generic method: func(inArgs) outArgs
	res := tool.CheckFuncArgs(a.targetType, inputType, 1, 0)
	tool.Assert(res, "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
	f := func(targetArgs []reflect.Value) []reflect.Value {
		return targetArgs[1:]
	}
	rf := func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
		return append([]reflect.Value{extraArgs[0]}, inputArgs...)
	}
	return f, rf
}

func (a *AdapterImpl) genericAdapter(inputName string, inputType reflect.Type) (fn, reversedFn) {
	targetType := a.ExtendedTargetType()

	// check function:
	// 		a. generic function: func(inArgs) outArgs
	if !a.method {
		res := tool.CheckFuncArgs(targetType, inputType, 1, 0)
		tool.Assert(res, "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
		f := func(targetArgs []reflect.Value) []reflect.Value {
			return targetArgs[1:]
		}
		rf := func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
			return append([]reflect.Value{extraArgs[0]}, inputArgs...)
		}
		return f, rf
	}

	// check method:
	// 		a. generic method: func(GenericInfo, self *struct, inArgs)
	if inputType.NumIn() >= 1 && inputType.In(0) == genericInfoType {
		tool.Assert(inputType.In(1) == targetType.In(0), "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
		res := tool.CheckFuncArgs(targetType, inputType, 2, 2)
		tool.Assert(res, "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
		f := func(targetArgs []reflect.Value) []reflect.Value {
			return append([]reflect.Value{targetArgs[1], targetArgs[0]}, targetArgs[2:]...)
		}
		rf := func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
			return append([]reflect.Value{inputArgs[1], inputArgs[0]}, inputArgs[2:]...)
		}
		return f, rf
	}

	// check method:
	// 		a. generic method: func(self *struct, inArgs)
	if inputType.NumIn() >= 1 && inputType.In(0) == targetType.In(0) {
		res := tool.CheckFuncArgs(targetType, inputType, 2, 1)
		tool.Assert(res, "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
		f := func(targetArgs []reflect.Value) []reflect.Value {
			return append([]reflect.Value{targetArgs[0]}, targetArgs[2:]...)
		}
		rf := func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
			return append([]reflect.Value{inputArgs[0], extraArgs[1]}, inputArgs[1:]...)
		}
		return f, rf
	}

	// check method:
	// 		a. generic method: func(inArgs)
	res := tool.CheckFuncArgs(targetType, inputType, 2, 0)
	tool.Assert(res, "args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
	f := func(targetArgs []reflect.Value) []reflect.Value {
		return targetArgs[2:]
	}
	rf := func(inputArgs, extraArgs []reflect.Value) []reflect.Value {
		return append([]reflect.Value{extraArgs[0], extraArgs[1]}, inputArgs...)
	}
	return f, rf
}

func (a *AdapterImpl) CheckReturnArgs(inputName string, inputType reflect.Type) {
	res := tool.CheckFuncReturnArgs(a.targetType, inputType)
	tool.Assert(res, "return args not match: target: %v, %s: %v", a.targetType, inputName, inputType)
}
