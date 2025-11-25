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

package fn

import (
	"reflect"
)

type Adapter interface {
	// Generic returns if the target is generic.
	Generic() bool

	// ExtendedTargetType returns the actual type of the target function at runtime. It contains generic type info if the
	// target is generic:
	//  1. function:
	//     a. non-generic function: func(inArgs) outArgs
	//     b. generic function: func(GenericInfo, inArgs) outArgs
	//  2. method:
	//     a. non-generic method: func(self *struct, inArgs) outArgs
	//     b. generic method:
	//     - BEFORE go1.20: func(GenericInfo, self *struct, inArgs) outArgs
	//     - AFTER go1.20: func(self *struct, GenericInfo, inArgs) outArgs
	ExtendedTargetType() reflect.Type

	// InputAdapter generates an adapter function to adapt the input arguments of the ExtendedTargetType() to the inputType.
	// These inputTypes are valid:
	//  1. function:
	//     a. non-generic function: func(inArgs) outArgs
	//     b. generic function: func(info GenericInfo, inArgs) outArgs OR func(inArgs) outArgs
	//  2. method:
	//     a. non-generic method: func(self *struct, inArgs) outArgs OR func(inArgs) outArgs
	//     b. generic method: func(GenericInfo, self *struct, inArgs) outArgs OR func(self *struct, inArgs) outArgs OR func(inArgs) outArgs
	InputAdapter(inputName string, inputType reflect.Type) func([]reflect.Value) []reflect.Value

	// ReversedInputAdapter generates an adapter function to adapt the input arguments of the inputType to the ExtendedTargetType().
	// These inputTypes are valid:
	//  1. function:
	//     a. non-generic function: func(inArgs) outArgs
	//     b. generic function: func(info GenericInfo, inArgs) outArgs OR func(inArgs) outArgs
	//  2. method:
	//     a. non-generic method: func(self *struct, inArgs) outArgs OR func(inArgs) outArgs
	//     b. generic method: func(GenericInfo, self *struct, inArgs) outArgs OR func(self *struct, inArgs) outArgs OR func(inArgs) outArgs
	ReversedInputAdapter(inputName string, inputType reflect.Type) func(inputArgs, extraArgs []reflect.Value) []reflect.Value

	// CheckReturnArgs checks if the return arguments of the inputType are valid. It should be consistent with the ExtendedTargetType().
	CheckReturnArgs(inputName string, inputType reflect.Type)
}
