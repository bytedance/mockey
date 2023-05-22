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
	"sync/atomic"

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
	times     int64
	mockTimes int64
	patch     *monkey.Patch
	lock      sync.Mutex
	isPatched bool
	builder   *MockBuilder

	outerCaller tool.CallerInfo // Mocker 的外部调用位置
}

type MockBuilder struct {
	target interface{} // 目标函数
	// hook            interface{} // mock函数
	proxyCaller interface{} // mock之后，原函数地址
	// when            interface{} // 条件函数
	conditions      []*mockCondition // 条件转移
	filterGoroutine FilterGoroutineType
	gId             int64
	unsafe          bool
}

func Mock(target interface{}) *MockBuilder {
	tool.AssertFunc(target)

	builder := &MockBuilder{
		target: target,
	}
	builder.resetCondition()
	return builder
}

// MockUnsafe has the full ability of the Mock function and removes some security restrictions. This is an alternative
// when the Mock function fails. It may cause some unknown problems, so we recommend using Mock under normal conditions.
func MockUnsafe(target interface{}) *MockBuilder {
	builder := Mock(target)
	builder.unsafe = true
	return builder
}

func (builder *MockBuilder) resetCondition() *MockBuilder {
	builder.conditions = []*mockCondition{builder.newCondition()} // at least 1 condition is needed
	return builder
}

func (builder *MockBuilder) Origin(funcPtr interface{}) *MockBuilder {
	tool.Assert(builder.proxyCaller == nil, "re-set builder origin")
	return builder.origin(funcPtr)
}

func (builder *MockBuilder) origin(funcPtr interface{}) *MockBuilder {
	tool.AssertPtr(funcPtr)
	builder.proxyCaller = funcPtr
	return builder
}

func (builder *MockBuilder) lastCondition() *mockCondition {
	cond := builder.conditions[len(builder.conditions)-1]
	if cond.Complete() {
		cond = builder.newCondition()
		builder.conditions = append(builder.conditions, cond)
	}
	return cond
}

func (builder *MockBuilder) newCondition() *mockCondition {
	return &mockCondition{builder: builder}
}

func (builder *MockBuilder) When(when interface{}) *MockBuilder {
	builder.lastCondition().SetWhen(when)
	return builder
}

func (builder *MockBuilder) To(hook interface{}) *MockBuilder {
	builder.lastCondition().SetTo(hook)
	return builder
}

func (builder *MockBuilder) Return(results ...interface{}) *MockBuilder {
	builder.lastCondition().SetReturn(results...)
	return builder
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

func (builder *MockBuilder) Build() *Mocker {
	mocker := Mocker{target: reflect.ValueOf(builder.target), builder: builder}
	mocker.buildHook()
	mocker.Patch()
	return &mocker
}

func (mocker *Mocker) checkReceiver(target reflect.Type, hook interface{}) bool {
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

func (mocker *Mocker) buildHook() {
	proxySetter := mocker.buildProxy()

	origin := reflect.ValueOf(mocker.proxy).Elem()
	originExec := func(args []reflect.Value) []reflect.Value {
		return tool.ReflectCall(origin, args)
	}

	match := []func(args []reflect.Value) bool{}
	exec := []func(args []reflect.Value) []reflect.Value{}

	for _, condition := range mocker.builder.conditions {
		when := condition.when
		hook := condition.hook

		if when == nil {
			// when condition is not set, just go into hook exec
			match = append(match, func(args []reflect.Value) bool { return true })
		} else {
			missWhenReceiver := mocker.checkReceiver(mocker.target.Type(), when)
			match = append(match, func(args []reflect.Value) bool {
				return tool.ReflectCallWithShiftOne(reflect.ValueOf(when), args, missWhenReceiver)[0].Bool()
			})
		}

		if hook == nil {
			exec = append(exec, originExec)
		} else {
			missHookReceiver := mocker.checkReceiver(mocker.target.Type(), hook)
			exec = append(exec, func(args []reflect.Value) []reflect.Value {
				mocker.mock()
				return tool.ReflectCallWithShiftOne(reflect.ValueOf(hook), args, missHookReceiver)
			})
		}
	}

	mockerHook := reflect.MakeFunc(mocker.target.Type(), func(args []reflect.Value) []reflect.Value {
		proxySetter(args) // 设置origin调用proxy

		mocker.access()
		switch mocker.builder.filterGoroutine {
		case Disable:
			break
		case Include:
			if tool.GetGoroutineID() != mocker.builder.gId {
				return originExec(args)
			}
		case Exclude:
			if tool.GetGoroutineID() == mocker.builder.gId {
				return originExec(args)
			}
		}

		for i, matchFn := range match {
			execFn := exec[i]
			if matchFn(args) {
				return execFn(args)
			}
		}

		return originExec(args)
	})
	mocker.hook = mockerHook
}

func (mocker *Mocker) buildProxy() func(args []reflect.Value) {
	proxy := reflect.New(mocker.target.Type())

	proxyCallerSetter := func(args []reflect.Value) {}
	missProxyReceiver := false
	if mocker.builder.proxyCaller != nil {
		pVal := reflect.ValueOf(mocker.builder.proxyCaller)
		tool.Assert(pVal.Kind() == reflect.Ptr && pVal.Elem().Kind() == reflect.Func, "origin receiver must be a function pointer")
		pElem := pVal.Elem()
		missProxyReceiver = mocker.checkReceiver(mocker.target.Type(), pElem.Interface())

		if missProxyReceiver {
			proxyCallerSetter = func(args []reflect.Value) {
				pElem.Set(reflect.MakeFunc(pElem.Type(), func(innerArgs []reflect.Value) (results []reflect.Value) {
					return tool.ReflectCall(proxy.Elem(), append(args[0:1], innerArgs...))
				}))
			}
		} else {
			proxyCallerSetter = func(args []reflect.Value) {
				pElem.Set(reflect.MakeFunc(pElem.Type(), func(innerArgs []reflect.Value) (results []reflect.Value) {
					return tool.ReflectCall(proxy.Elem(), innerArgs)
				}))
			}
		}
	}
	mocker.proxy = proxy.Interface()
	return proxyCallerSetter
}

func (mocker *Mocker) Patch() *Mocker {
	mocker.lock.Lock()
	defer mocker.lock.Unlock()
	if mocker.isPatched {
		return mocker
	}
	mocker.patch = monkey.PatchValue(mocker.target, mocker.hook, reflect.ValueOf(mocker.proxy), mocker.builder.unsafe)
	mocker.isPatched = true
	addToGlobal(mocker)

	mocker.outerCaller = tool.OuterCaller()
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
	atomic.StoreInt64(&mocker.times, 0)
	atomic.StoreInt64(&mocker.mockTimes, 0)

	return mocker
}

func (mocker *Mocker) Release() *MockBuilder {
	mocker.UnPatch()
	mocker.builder.resetCondition()
	return mocker.builder
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
	tool.Assert(len(mocker.builder.conditions) == 1, "only one-condition mocker could reset when (You can call Release first, then rebuild mocker)")

	return mocker.rePatch(func() {
		mocker.builder.conditions[0].SetWhenForce(when)
	})
}

func (mocker *Mocker) To(to interface{}) *Mocker {
	tool.Assert(len(mocker.builder.conditions) == 1, "only one-condition mocker could reset to  (You can call Release first, then rebuild mocker)")

	return mocker.rePatch(func() {
		mocker.builder.conditions[0].SetToForce(to)
	})
}

func (mocker *Mocker) Return(results ...interface{}) *Mocker {
	tool.Assert(len(mocker.builder.conditions) == 1, "only one-condition mocker could reset return  (You can call Release first, then rebuild mocker)")

	return mocker.rePatch(func() {
		mocker.builder.conditions[0].SetReturnForce(results...)
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
	mocker.buildHook()
	mocker.Patch()
	return mocker
}

func (mocker *Mocker) access() {
	atomic.AddInt64(&mocker.times, 1)
}

func (mocker *Mocker) mock() {
	atomic.AddInt64(&mocker.mockTimes, 1)
}

func (mocker *Mocker) Times() int {
	return int(mocker.times)
}

func (mocker *Mocker) MockTimes() int {
	return int(mocker.mockTimes)
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

func (mocker *Mocker) caller() tool.CallerInfo {
	return mocker.outerCaller
}
