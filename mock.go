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
	"sync"

	"github.com/bytedance/mockey/internal/monkey"
	"github.com/bytedance/mockey/internal/tool"
)

type FilterGoroutineType int64

const (
	Disable FilterGoroutineType = 0
	Include FilterGoroutineType = 1
	Exclude FilterGoroutineType = 2
)

type Mocker struct {
	target    reflect.Value // 目标函数
	hook      reflect.Value // mock函数
	proxy     interface{}   // mock之后，原函数地址
	times     int
	mockTimes int
	patch     *monkey.Patch
	lock      sync.Mutex
	isPatched bool
	builder   *MockBuilder
}

type MockBuilder struct {
	target           interface{} // 目标函数
	hook             interface{} // mock函数
	proxy            interface{} // mock之后，原函数地址
	when             interface{} // 条件函数
	filterGoroutine  FilterGoroutineType
	gId              int64
	missHookReceiver bool
	missWhenReceiver bool
}

func Mock(target interface{}) *MockBuilder {
	tool.AssertFunc(target)

	return &MockBuilder{
		target: target,
	}

}

func (builder *MockBuilder) Origin(funcPtr interface{}) *MockBuilder {
	tool.Assert(builder.proxy == nil, "re-set builder origin")
	return builder.origin(funcPtr)
}

func (builder *MockBuilder) origin(funcPtr interface{}) *MockBuilder {
	tool.AssertPtr(funcPtr)
	builder.proxy = funcPtr
	return builder
}

func (builder *MockBuilder) When(when interface{}) *MockBuilder {
	tool.Assert(builder.when == nil, "re-set builder when")
	return builder.setWhen(when)
}

func (builder *MockBuilder) setWhen(when interface{}) *MockBuilder {
	wVal := reflect.ValueOf(when)
	tool.Assert(wVal.Type().NumOut() == 1, "when func ret value not bool")
	out1 := wVal.Type().Out(0)
	tool.Assert(out1.Kind() == reflect.Bool, "when func ret value not bool")
	builder.when = when
	return builder
}

func (builder *MockBuilder) setTo(hook interface{}) *MockBuilder {
	hType := reflect.TypeOf(hook)
	tool.Assert(hType.Kind() == reflect.Func, "hook a is not a func")
	builder.hook = hook
	return builder
}

func (builder *MockBuilder) To(hook interface{}) *MockBuilder {
	tool.Assert(builder.hook == nil, "re-set builder hook")
	return builder.setTo(hook)
}

func (builder *MockBuilder) Return(results ...interface{}) *MockBuilder {
	tool.Assert(builder.hook == nil, "re-set builder hook")
	return builder.setReturn(results...)
}

func (builder *MockBuilder) IncludeCurrentGoRoutine() *MockBuilder {
	return builder.FilterGoRoutine(Include, tool.GetGoroutineID())
}

func (builder *MockBuilder) ExcludeCurrentGoRoutine() *MockBuilder {
	return builder.FilterGoRoutine(Exclude, tool.GetGoroutineID())
}

func (builder *MockBuilder) FilterGoRoutine(filter FilterGoroutineType, gId int64) *MockBuilder {
	builder.filterGoroutine = filter
	builder.gId = gId
	return builder
}

func (builder *MockBuilder) setReturn(results ...interface{}) *MockBuilder {
	tool.CheckReturnType(builder.target, results...)
	targetType := reflect.TypeOf(builder.target)
	builder.hook = reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
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
	return builder
}

func (builder *MockBuilder) Build() *Mocker {
	mocker := Mocker{target: reflect.ValueOf(builder.target), builder: builder}
	mocker.buildHook(builder)
	mocker.Patch()
	return &mocker
}

func (mocker *Mocker) checkReceiver(target reflect.Type, hook interface{}) bool {
	hType := reflect.TypeOf(hook)
	tool.Assert(hType.Kind() == reflect.Func, "Param a is not a func")
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

func (mocker *Mocker) buildHook(builder *MockBuilder) {
	when := builder.when
	hook := builder.hook
	proxy := builder.proxy

	var p reflect.Value

	if proxy == nil {
		proxy := mocker.target
		p = reflect.New(proxy.Type())
	} else {
		p = reflect.ValueOf(proxy)
	}
	if when != nil {
		builder.missWhenReceiver = mocker.checkReceiver(mocker.target.Type(), when)
	}
	if hook != nil {
		builder.missHookReceiver = mocker.checkReceiver(mocker.target.Type(), hook)
	}
	mockerHook := reflect.MakeFunc(mocker.target.Type(), func(args []reflect.Value) []reflect.Value {
		origin := p.Elem()
		mocker.access()

		switch builder.filterGoroutine {
		case Disable:
			break
		case Include:
			if tool.GetGoroutineID() != builder.gId {
				return tool.ReflectCall(origin, args)
			}
		case Exclude:
			if tool.GetGoroutineID() == builder.gId {
				return tool.ReflectCall(origin, args)
			}
		}

		var hVal reflect.Value

		if when != nil {
			wVal := reflect.ValueOf(when)
			ret := tool.ReflectCallWithShiftOne(wVal, args, builder.missWhenReceiver)
			b := ret[0].Bool()

			if b && hook != nil {
				hVal = reflect.ValueOf(hook)
				mocker.mock()
				return tool.ReflectCallWithShiftOne(hVal, args, builder.missHookReceiver)
			}
			return tool.ReflectCall(origin, args)
		}
		if hook == nil {
			return tool.ReflectCall(origin, args)
		} else {
			hVal = reflect.ValueOf(hook)
		}
		mocker.mock()
		return tool.ReflectCallWithShiftOne(hVal, args, builder.missHookReceiver)
	})
	mocker.hook = mockerHook
	mocker.proxy = p.Interface()
}

func (mocker *Mocker) Patch() *Mocker {
	mocker.lock.Lock()
	defer mocker.lock.Unlock()
	if mocker.isPatched {
		return mocker
	}
	mocker.patch = monkey.PatchValue(mocker.target, mocker.hook, reflect.ValueOf(mocker.proxy))
	mocker.isPatched = true
	addToGlobal(mocker)

	return mocker
}

func (mocker *Mocker) UnPatch() *Mocker {
	mocker.lock.Lock()
	defer mocker.lock.Unlock()
	if !mocker.isPatched {
		return mocker
	}
	mocker.patch.Unpatch()
	mocker.isPatched = false
	removeFromGlobal(mocker)
	mocker.times = 0
	mocker.mockTimes = 0

	return mocker
}

func (mocker *Mocker) ExcludeCurrentGoRoutine() *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.ExcludeCurrentGoRoutine()
	})
}

func (mocker *Mocker) FilterGoRoutine(filter FilterGoroutineType, gId int64) *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.FilterGoRoutine(filter, gId)
	})
}

func (mocker *Mocker) IncludeCurrentGoRoutine() *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.IncludeCurrentGoRoutine()
	})
}

func (mocker *Mocker) When(when interface{}) *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.setWhen(when)
	})
}

func (mocker *Mocker) To(to interface{}) *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.setTo(to)
	})
}

func (mocker *Mocker) Return(results ...interface{}) *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.setReturn(results...)
	})
}

func (mocker *Mocker) Origin(funcPtr interface{}) *Mocker {
	return mocker.rePatch(func() {
		mocker.builder.origin(funcPtr)
	})
}

func (mocker *Mocker) rePatch(do func()) *Mocker {
	mocker.UnPatch()
	do()
	mocker.buildHook(mocker.builder)
	mocker.Patch()
	return mocker
}

func (mocker *Mocker) access() {
	mocker.lock.Lock()
	mocker.times++
	mocker.lock.Unlock()
}

func (mocker *Mocker) mock() {
	mocker.lock.Lock()
	mocker.mockTimes++
	mocker.lock.Unlock()
}

func (mocker *Mocker) Times() int {
	return mocker.times
}

func (mocker *Mocker) MockTimes() int {
	return mocker.mockTimes
}

func (mocker *Mocker) key() uintptr {
	return mocker.target.Pointer()
}

func (mocker *Mocker) name() string {
	return mocker.target.String()
}

func (mocker *Mocker) unPatch() {
	mocker.UnPatch()
}
