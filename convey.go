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
	"github.com/smartystreets/goconvey/convey"
)

var gMocker = make([]map[uintptr]mockerInstance, 0)

func init() {
	gMocker = append(gMocker, make(map[uintptr]mockerInstance))
}

func PatchConvey(items ...interface{}) {
	for i, item := range items {
		if reflect.TypeOf(item).Kind() == reflect.Func {
			items[i] = reflect.MakeFunc(reflect.TypeOf(item), func(args []reflect.Value) []reflect.Value {
				gMocker = append(gMocker, make(map[uintptr]mockerInstance))
				defer func() {
					for _, mocker := range gMocker[len(gMocker)-1] {
						mocker.unPatch()
					}
					gMocker = gMocker[:len(gMocker)-1]
				}()
				return tool.ReflectCall(reflect.ValueOf(item), args)
			}).Interface()
		}
	}

	convey.Convey(items...)
}

func addToGlobal(mocker mockerInstance) {
	tool.DebugPrintf("%v added\n", mocker.key())
	last, ok := gMocker[len(gMocker)-1][mocker.key()]
	if ok {
		tool.Assert(!ok, "re-mock %v, previous mock at: %v", mocker.name(), last.caller())
	}
	gMocker[len(gMocker)-1][mocker.key()] = mocker
}

func removeFromGlobal(mocker mockerInstance) {
	tool.DebugPrintf("%v removed\n", mocker.key())
	delete(gMocker[len(gMocker)-1], mocker.key())
}

// UnPatchAll unpatch all mocks in current 'PatchConvey' context
//
// If the caller is out of 'PatchConvey', it will unpatch all mocks
//
// For example:
//
//	Test1(t) {
//	    Mock(a).Build()
//		Mock(b).Build()
//
//	    // a and b will be unpatched
//		UnpatchAll()
//	}
//
//	Test2(t) {
//		Mock(a).Build()
//		PatchConvey(t,func(){
//		    Mock(b).Build()
//
//			// only b will be unpatched
//			UnpatchAll()
//		}
//	}
func UnPatchAll() {
	for _, mocker := range gMocker[len(gMocker)-1] {
		mocker.unPatch()
	}
}
