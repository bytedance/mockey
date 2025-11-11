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

package mockey

import (
	"reflect"

	"github.com/bytedance/mockey/internal/tool"
)

type mockCondition struct {
	when interface{} // condition
	hook interface{} // mock function

	builder *MockBuilder
}

func (m *mockCondition) Complete() bool {
	return m.when != nil && m.hook != nil
}

func (m *mockCondition) SetWhen(when interface{}) {
	tool.Assert(m.when == nil, "re-set builder when")
	m.SetWhenForce(when)
}

func (m *mockCondition) SetWhenForce(when interface{}) {
	wVal := reflect.ValueOf(when)
	tool.Assert(wVal.Type().NumOut() == 1, "when func ret value not bool")
	out1 := wVal.Type().Out(0)
	tool.Assert(out1.Kind() == reflect.Bool, "when func ret value not bool")

	hookType := m.builder.targetType()
	inTypes := []reflect.Type{}
	for i := 0; i < hookType.NumIn(); i++ {
		inTypes = append(inTypes, hookType.In(i))
	}

	hasGeneric, hasReceiver := m.checkGenericAndReceiver(wVal.Type())
	whenType := reflect.FuncOf(inTypes, []reflect.Type{out1}, hookType.IsVariadic())
	m.when = reflect.MakeFunc(whenType, func(args []reflect.Value) (results []reflect.Value) {
		results = tool.ReflectCall(wVal, m.adaptArgsForReflectCall(args, hasGeneric, hasReceiver))
		return
	}).Interface()
}

func (m *mockCondition) SetReturn(results ...interface{}) {
	tool.Assert(m.hook == nil, "re-set builder hook")
	m.SetReturnForce(results...)
}

func (m *mockCondition) SetReturnForce(results ...interface{}) {
	getResult := func() []interface{} { return results }
	if len(results) == 1 {
		seq, ok := results[0].(SequenceOpt)
		if ok {
			getResult = seq.GetNext
		}
	}

	hookType := m.builder.targetType()
	m.hook = reflect.MakeFunc(hookType, func(_ []reflect.Value) []reflect.Value {
		results := getResult()
		tool.CheckReturnType(m.builder.target, results...)
		valueResults := make([]reflect.Value, 0)
		for i, result := range results {
			rValue := reflect.Zero(hookType.Out(i))
			if result != nil {
				rValue = reflect.ValueOf(result).Convert(hookType.Out(i))
			}
			valueResults = append(valueResults, rValue)
		}
		return valueResults
	}).Interface()
}

func (m *mockCondition) SetTo(to interface{}) {
	tool.Assert(m.hook == nil, "re-set builder hook")
	m.SetToForce(to)
}

func (m *mockCondition) SetToForce(to interface{}) {
	hType := reflect.TypeOf(to)
	tool.Assert(hType.Kind() == reflect.Func, "to is not a func")
	hasGeneric, hasReceiver := m.checkGenericAndReceiver(hType)
	tool.Assert(m.builder.generic || !hasGeneric, "non-generic function should not have 'GenericInfo' as first argument")
	m.hook = reflect.MakeFunc(m.builder.targetType(), func(args []reflect.Value) (results []reflect.Value) {
		results = tool.ReflectCall(reflect.ValueOf(to), m.adaptArgsForReflectCall(args, hasGeneric, hasReceiver))
		return
	}).Interface()
}

// checkGenericAndReceiver check if typ has GenericsInfo and selfReceiver as argument
//
// The hook function will look like func(_ GenericInfo, self *struct, arg0 int ...)
// When we use 'When' or 'To', our input hook function will looks like:
//  1. func(arg0 int ...)
//  2. func(info GenericInfo, arg0 int ...)
//  3. func(self *struct, arg0 int ...)
//  4. func(info GenericInfo, self *struct, arg0 int ...)
//
// All above input hooks are legal, but we need to make an adaptation when calling then
func (m *mockCondition) checkGenericAndReceiver(typ reflect.Type) (hasGeneric bool, hasReceiver bool) {
	targetType := reflect.TypeOf(m.builder.target)
	tool.Assert(typ.Kind() == reflect.Func, "Param(%v) a is not a func", typ.Kind())
	tool.Assert(targetType.IsVariadic() == typ.IsVariadic(), "target: %v, hook: %v args not match", targetType, typ)

	shiftTyp := 0
	if typ.NumIn() > 0 && typ.In(0) == genericInfoType {
		shiftTyp = 1
	}

	// has receiver
	if tool.CheckFuncArgs(targetType, typ, 0, shiftTyp) {
		return shiftTyp == 1, true
	}

	if tool.CheckFuncArgs(targetType, typ, 1, shiftTyp) {
		return shiftTyp == 1, false
	}
	tool.Assert(false, "target: %v, hook: %v args not match", targetType, typ)
	return false, false // not here
}

// adaptArgsForReflectCall makes an adaption for reflect call
//
// see (*mockCondition).checkGenericAndReceiver for more info
func (m *mockCondition) adaptArgsForReflectCall(args []reflect.Value, hasGeneric, hasReceiver bool) []reflect.Value {
	adaption := []reflect.Value{}
	if m.builder.generic {
		if hasGeneric {
			adaption = append(adaption, args[0])
		}
		args = args[1:]
	}
	if !hasReceiver {
		args = args[1:]
	}
	adaption = append(adaption, args...)
	return adaption
}
