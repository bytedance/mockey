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

package tool

import (
	"reflect"
)

func ReflectCallWithShiftOne(f reflect.Value, args []reflect.Value, shift bool) []reflect.Value {
	if shift {
		return ReflectCall(f, args[1:])
	}
	return ReflectCall(f, args)
}

func ReflectCall(f reflect.Value, args []reflect.Value) []reflect.Value {
	if f.Type().IsVariadic() {
		newArgs := make([]reflect.Value, 0)
		lastArg := args[len(args)-1]
		for i := 0; i < len(args)-1; i++ {
			newArgs = append(newArgs, args[i])
		}
		// FIXME: There are a few cases that lastArgs.cap == 0 but lastArg.len == MAX_INT (arm64).
		// 		Currently we have no idea how it happens.
		//      In that case we assume that i should less than lastArg.cap
		if lastArg.Len() < lastArg.Cap() {
			DebugPrintf("ReflectCall: broken variadic params: len is %d but cap is %d", lastArg.Len(), lastArg.Cap())
		}
		for i := 0; i < lastArg.Len() && i < lastArg.Cap(); i++ {
			newArgs = append(newArgs, lastArg.Index(i))
		}
		return f.Call(newArgs)
	} else {
		return f.Call(args)
	}
}
