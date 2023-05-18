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
	checkReceiver(reflect.TypeOf(m.builder.target), when) // inputs must be in same or has an extra self receiver
	m.when = when
}

func (m *mockCondition) SetReturn(results ...interface{}) {
	tool.Assert(m.hook == nil, "re-set builder hook")
	m.SetReturnForce(results...)
}

func (m *mockCondition) SetReturnForce(results ...interface{}) {
	tool.CheckReturnType(m.builder.target, results...)
	targetType := reflect.TypeOf(m.builder.target)
	m.hook = reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		valueResults := make([]reflect.Value, 0)
		for i, result := range results {
			rValue := reflect.Zero(targetType.Out(i))
			if result != nil {
				rValue = reflect.ValueOf(result).Convert(targetType.Out(i))
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
	tool.Assert(hType.Kind() == reflect.Func, "to a is not a func")
	m.hook = to
}

func checkReceiver(target reflect.Type, hook interface{}) bool {
	hType := reflect.TypeOf(hook)
	tool.Assert(hType.Kind() == reflect.Func, "Param(%v) a is not a func", hType.Kind())
	tool.Assert(target.IsVariadic() == hType.IsVariadic(), "target:%v, hook:%v args not match", target, hook)
	// has receiver
	if tool.CheckFuncArgs(target, hType, 0) {
		return false
	}
	if tool.CheckFuncArgs(target, hType, 1) {
		return true
	}
	tool.Assert(false, "target:%v, hook:%v args not match", target, hook)
	return false
}
