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

func CheckReturnType(fn interface{}, results ...interface{}) {
	t := reflect.TypeOf(fn)
	Assert(t.Kind() == reflect.Func, "Param a is not a func")
	Assert(t.NumOut() == len(results), "Return Num of Func a does not match: %v, origin ret count: %d, input param count: %d", t, t.NumOut(), len(results))
	for i := 0; i < t.NumOut(); i++ {
		if results[i] == nil {
			continue
		}
		Assert(reflect.TypeOf(results[i]).ConvertibleTo(t.Out(i)), "Return value idx %d of rets %v can not convertible to %v", i, reflect.TypeOf(results[i]), t.Out(i))
	}
}

func CheckFuncArgs(a, b reflect.Type, shiftA, shiftB int) bool {
	if a.NumIn()-shiftA == b.NumIn()-shiftB {
		for indexA, indexB := shiftA, shiftB; indexA < a.NumIn(); indexA, indexB = indexA+1, indexB+1 {
			if a.In(indexA) != b.In(indexB) {
				return false
			}
		}
		return true
	}
	return false
}
