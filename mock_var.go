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

	"github.com/bytedance/mockey/internal/tool"
)

type mockerInstance interface {
	identityKey() uintptr
	layerKey() uintptr
	name() string
	unPatchLocked()

	caller() tool.CallerInfo
}

type MockerVar struct {
	target     reflect.Value // 目标变量地址
	hook       reflect.Value // mock变量
	targetType reflect.Type
	origin     interface{} // 原始值
	lock       sync.Mutex
	isPatched  bool

	outerCaller tool.CallerInfo
}

func MockValue(targetPtr interface{}) *MockerVar {
	tool.AssertPtr(targetPtr)

	target := reflect.ValueOf(targetPtr).Elem()
	return &MockerVar{
		target:     target,
		targetType: target.Type(),
	}
}

func (mocker *MockerVar) To(value interface{}) *MockerVar {
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()
	mocker.lock.Lock()
	defer mocker.lock.Unlock()

	var v reflect.Type

	if value == nil {
		mocker.hook = reflect.Zero(mocker.targetType)
		v = mocker.targetType
	} else {
		mocker.hook = reflect.ValueOf(value)
		v = reflect.TypeOf(value)
	}

	tool.Assert(v.AssignableTo(mocker.targetType), "value type: %s not match target type: %s", v.Name(), mocker.targetType.Name())
	mocker.patchValueLocked()
	return mocker
}

func (mocker *MockerVar) Patch() *MockerVar {
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()
	mocker.lock.Lock()
	defer mocker.lock.Unlock()
	mocker.patchValueLocked()
	return mocker
}

func (mocker *MockerVar) patchValueLocked() {
	if mocker.isPatched {
		return
	}

	assertNotInGlobal(mocker)
	mocker.origin = mocker.target.Interface()
	mocker.target.Set(mocker.hook)
	mocker.isPatched = true
	addToGlobal(mocker)
	mocker.outerCaller = tool.OuterCaller()
}

func (mocker *MockerVar) UnPatch() *MockerVar {
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()
	mocker.unPatchLocked()
	return mocker
}

func (mocker *MockerVar) unPatchLocked() {
	mocker.lock.Lock()
	defer mocker.lock.Unlock()
	if !mocker.isPatched {
		return
	}

	assertTopInGlobal(mocker)
	if mocker.origin == nil {
		mocker.target.Set(reflect.Zero(mocker.targetType))
	} else {
		mocker.target.Set(reflect.ValueOf(mocker.origin))
	}
	mocker.isPatched = false
	removeFromGlobal(mocker)
	mocker.origin = nil
}

func (mocker *MockerVar) identityKey() uintptr {
	return mocker.target.Addr().Pointer()
}

func (mocker *MockerVar) layerKey() uintptr {
	return mocker.identityKey()
}

func (mocker *MockerVar) name() string {
	if mocker.target.Kind() == reflect.String {
		return "<string Value>"
	}
	return mocker.target.String()
}

func (mocker *MockerVar) caller() tool.CallerInfo {
	return mocker.outerCaller
}
