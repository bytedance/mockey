//go:build go1.26 && !go1.28
// +build go1.26,!go1.28

/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to support Go 1.27.
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

package inst

import (
	"reflect"

	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/linkname"
)

func initDuffFunc() {
	if duffcopyPC := linkname.FuncPCForName("runtime.duffcopy"); duffcopyPC > 0 {
		duffcopy = fn.MakeFunc(reflect.TypeOf(duffcopy), duffcopyPC).Interface().(func())
	}
	if duffzeroPC := linkname.FuncPCForName("runtime.duffzero"); duffzeroPC > 0 {
		duffzero = fn.MakeFunc(reflect.TypeOf(duffzero), duffzeroPC).Interface().(func())
	}
}
