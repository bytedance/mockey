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

// GetMethod resolve a certain public method from an instance.
func GetMethod(instance interface{}, methodName string) interface{} {
	if typ := reflect.TypeOf(instance); typ != nil {
		if m, ok := getNestedMethod(typ, methodName); ok {
			return m.Func.Interface()
		}
		if m, ok := typ.MethodByName(methodName); ok {
			return m.Func.Interface()
		}
	}
	tool.Assert(false, "can't reflect instance method :%v", methodName)
	return nil
}

// GetPrivateMethod ...
// Deprecated, use GetMethod instead.
func GetPrivateMethod(instance interface{}, methodName string) interface{} {
	m, ok := reflect.TypeOf(instance).MethodByName(methodName)
	if ok {
		return m.Func.Interface()
	}
	tool.Assert(false, "can't reflect instance method :%v", methodName)
	return nil
}

// GetNestedMethod resolves a certain public method in anonymous structs, it will
// look for the specific method in every anonymous struct field recursively.
// Deprecated, use GetMethod instead.
func GetNestedMethod(instance interface{}, methodName string) interface{} {
	if typ := reflect.TypeOf(instance); typ != nil {
		if m, ok := getNestedMethod(typ, methodName); ok {
			return m.Func.Interface()
		}
	}
	tool.Assert(false, "can't reflect instance method :%v", methodName)
	return nil
}

func getNestedMethod(typ reflect.Type, methodName string) (reflect.Method, bool) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Struct {
		for i := 0; i < typ.NumField(); i++ {
			if !typ.Field(i).Anonymous {
				// there is no need to acquire non-anonymous method
				continue
			}
			if m, ok := getNestedMethod(typ.Field(i).Type, methodName); ok {
				return m, true
			}
		}
	}
	// a struct receiver is prior to the corresponding pointer receiver
	if m, ok := typ.MethodByName(methodName); ok {
		return m, true
	}
	return reflect.PtrTo(typ).MethodByName(methodName)
}

// GetGoroutineId ...
// Deprecated
func GetGoroutineId() int64 {
	return tool.GetGoroutineID()
}
