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

import "github.com/bytedance/mockey/internal/fn"

// MockGeneric mocks a generic function or method.
// From Go 1.20, Mock can automatically recognize generic functions and methods
// of generic receiver types.
// For Go 1.27 methods with their own type parameters, pass the declaring receiver's
// method expression explicitly, for example:
//
//	MockGeneric((*Service).Echo[int]).Return(42).Build()
//
// This uses the existing generic dictionary probe and receiver argument adapters.
// Mock(target, OptGeneric) and Mock(target, OptMethod) also support these method
// expressions. Automatic recognition without an option, bound method values and
// promoted method expressions are not supported for these methods.
func MockGeneric(target interface{}, opt ...mockOptionFn) *MockBuilder {
	return Mock(target, append(opt, OptGeneric)...)
}

type GenericInfo = fn.GenericInfo
