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
	target    reflect.Value // mock target value
	hook      reflect.Value // mock hook
	proxy     interface{}   // proxy function to origin
	times     int64
	mockTimes int64
	patch     *monkey.Patch
	lock      sync.Mutex
	isPatched bool
	builder   *MockBuilder

	outerCaller tool.CallerInfo
}

type MockBuilder struct {
	target          interface{}      // mock target
	proxyCaller     interface{}      // origin function caller hook
	conditions      []*mockCondition // mock conditions
	filterGoroutine FilterGoroutineType
	gId             int64
	unsafe          bool
	generic         bool
}

// Mock mocks target function
//
// If target is a generic method or method of generic types, you need add a genericOpt, like this:
//
//	func f[int, float64](x int, y T1) T2
//	Mock(f[int, float64], OptGeneric)
func Mock(target interface{}, opt ...optionFn) *MockBuilder {
	tool.AssertFunc(target)

	option := resolveOpt(opt...)

	builder := &MockBuilder{
		target:  target,
		unsafe:  option.unsafe,
		generic: option.generic,
	}
	builder.resetCondition()
	return builder
}

// MockUnsafe has the full ability of the Mock function and removes some security restrictions. This is an alternative
// when the Mock function fails. It may cause some unknown problems, so we recommend using Mock under normal conditions.
func MockUnsafe(target interface{}) *MockBuilder {
	return Mock(target, OptUnsafe)
}

func (builder *MockBuilder) hookType() reflect.Type {
	targetType := reflect.TypeOf(builder.target)
	if builder.generic {
		targetIn := []reflect.Type{genericInfoType}
		for i := 0; i < targetType.NumIn(); i++ {
			targetIn = append(targetIn, targetType.In(i))
		}
		targetOut := []reflect.Type{}
		for i := 0; i < targetType.NumOut(); i++ {
			targetOut = append(targetOut, targetType.Out(i))
		}
		return reflect.FuncOf(targetIn, targetOut, targetType.IsVariadic())
	}
	return targetType
}

func (builder *MockBuilder) resetCondition() *MockBuilder {
	builder.conditions = []*mockCondition{builder.newCondition()} // at least 1 condition is needed
	return builder
}

// Origin add an origin hook which can be used to call un-mocked origin function
//
// For example:
//
//	 origin := Fun // only need the same type
//	 mock := func(p string) string {
//		 return origin(p + "mocked")
//	 }
//	 mock2 := Mock(Fun).To(mock).Origin(&origin).Build()
//
// Origin only works when call origin hook directly, target will still be mocked in recursive call
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

// When declares the condition hook that's called to determine whether the mock should be executed.
//
// The condition hook function must have the same parameters as the target function.
//
// The following example would execute the mock when input int is negative
//
//	func Fun(input int) string {
//		return strconv.Itoa(input)
//	}
//	Mock(Fun).When(func(input int) bool { return input < 0 }).Return("0").Build()
//
// Note that if the target function is a struct method, you may optionally include
// the receiver as the first argument of the condition hook function. For example,
//
//	type Foo struct {
//		Age int
//	}
//	func (f *Foo) GetAge(younger int) string {
//		return strconv.Itoa(f.Age - younger)
//	}
//	Mock((*Foo).GetAge).When(func(f *Foo, younger int) bool { return younger < 0 }).Return("0").Build()
func (builder *MockBuilder) When(when interface{}) *MockBuilder {
	builder.lastCondition().SetWhen(when)
	return builder
}

// To declares the hook function that's called to replace the target function.
//
// The hook function must have the same signature as the target function.
//
// The following example would make Fun always return true
//
//	func Fun(input string) bool {
//		return input == "fun"
//	}
//
//	Mock(Fun).To(func(_ string) bool {return true}).Build()
//
// Note that if the target function is a struct method, you may optionally include
// the receiver as the first argument of the hook function. For example,
//
//	type Foo struct {
//		Name string
//	}
//	func (f *Foo) Bar(other string) bool {
//		return other == f.Name
//	}
//	Mock((*Foo).Bar).To(func(f *Foo, other string) bool {return true}).Build()
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

func (mocker *Mocker) missReceiver(target reflect.Type, hook interface{}) bool {
	hType := reflect.TypeOf(hook)
	tool.Assert(hType.Kind() == reflect.Func, "Param(%v) a is not a func", hType.Kind())
	tool.Assert(target.IsVariadic() == hType.IsVariadic(), "target:%v, hook:%v args not match", target, hook)
	// has receiver
	if tool.CheckFuncArgs(target, hType, 0, 0) {
		return false
	}
	if tool.CheckFuncArgs(target, hType, 1, 0) {
		return true
	}
	tool.Assert(false, "target:%v, hook:%v args not match", target, hook)
	return false
}

func (mocker *Mocker) buildHook() {
	proxySetter := mocker.buildProxy()

	originExec := func(args []reflect.Value) []reflect.Value {
		return tool.ReflectCall(reflect.ValueOf(mocker.proxy).Elem(), args)
	}

	match := []func(args []reflect.Value) bool{}
	exec := []func(args []reflect.Value) []reflect.Value{}

	for i := range mocker.builder.conditions {
		condition := mocker.builder.conditions[i]
		if condition.when == nil {
			// when condition is not set, just go into hook exec
			match = append(match, func(args []reflect.Value) bool { return true })
		} else {
			match = append(match, func(args []reflect.Value) bool {
				return tool.ReflectCall(reflect.ValueOf(condition.when), args)[0].Bool()
			})
		}

		if condition.hook == nil {
			// hook condition is not set, just go into original exec
			exec = append(exec, originExec)
		} else {
			exec = append(exec, func(args []reflect.Value) []reflect.Value {
				mocker.mock()
				return tool.ReflectCall(reflect.ValueOf(condition.hook), args)
			})
		}
	}

	mockerHook := reflect.MakeFunc(mocker.builder.hookType(), func(args []reflect.Value) []reflect.Value {
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

// buildProx create a proxyCaller which could call origin directly
func (mocker *Mocker) buildProxy() func(args []reflect.Value) {
	proxy := reflect.New(mocker.builder.hookType())

	proxyCallerSetter := func(args []reflect.Value) {}
	if mocker.builder.proxyCaller != nil {
		pVal := reflect.ValueOf(mocker.builder.proxyCaller)
		tool.Assert(pVal.Kind() == reflect.Ptr && pVal.Elem().Kind() == reflect.Func, "origin receiver must be a function pointer")
		pElem := pVal.Elem()

		shift := 0
		if mocker.builder.generic {
			shift += 1
		}
		if mocker.missReceiver(mocker.target.Type(), pElem.Interface()) {
			shift += 1
		}
		proxyCallerSetter = func(args []reflect.Value) {
			pElem.Set(reflect.MakeFunc(pElem.Type(), func(innerArgs []reflect.Value) (results []reflect.Value) {
				return tool.ReflectCall(proxy.Elem(), append(args[0:shift], innerArgs...))
			}))
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
	mocker.patch = monkey.PatchValue(mocker.target, mocker.hook, reflect.ValueOf(mocker.proxy), mocker.builder.unsafe, mocker.builder.generic)
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
	return int(atomic.LoadInt64(&mocker.times))
}

func (mocker *Mocker) MockTimes() int {
	return int(atomic.LoadInt64(&mocker.mockTimes))
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
