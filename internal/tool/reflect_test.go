/*
 * Copyright 2026 ByteDance Inc.
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
	"testing"
)

func TestReflectCallVariadicSliceState(t *testing.T) {
	tests := []struct {
		name  string
		input []int
	}{
		{name: "nil", input: nil},
		{name: "empty", input: []int{}},
		{name: "empty with capacity", input: make([]int, 0, 4)},
		{name: "values with capacity", input: make([]int, 2, 4)},
	}
	f := reflect.ValueOf(func(prefix string, values ...int) (string, bool, int, int) {
		return prefix, values == nil, len(values), cap(values)
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ReflectCall(f, []reflect.Value{reflect.ValueOf("prefix"), reflect.ValueOf(test.input)})
			if got[0].String() != "prefix" {
				t.Errorf("fixed argument = %q, want prefix", got[0].String())
			}
			if got[1].Bool() != (test.input == nil) {
				t.Errorf("nil = %v, want %v", got[1].Bool(), test.input == nil)
			}
			if got[2].Int() != int64(len(test.input)) || got[3].Int() != int64(cap(test.input)) {
				t.Errorf("length/capacity = %d/%d, want %d/%d", got[2].Int(), got[3].Int(), len(test.input), cap(test.input))
			}
		})
	}
}

func TestReflectCallVariadicSliceAliasing(t *testing.T) {
	backing := []int{1, 2, 3, 4, 5}
	input := backing[1:3:5]
	f := reflect.ValueOf(func(fixed []int, values ...int) (bool, []int) {
		values[0] = 42
		aliased := &fixed[0] == &values[0] && fixed[0] == 42
		return aliased, append(values, 99)
	})
	got := ReflectCall(f, []reflect.Value{reflect.ValueOf(input), reflect.ValueOf(input)})
	if !got[0].Bool() {
		t.Error("fixed and variadic arguments no longer share their backing array")
	}
	if want := []int{1, 42, 3, 99, 5}; !reflect.DeepEqual(backing, want) {
		t.Errorf("caller backing array = %v, want %v", backing, want)
	}
	result := got[1].Interface().([]int)
	if len(result) != 3 || cap(result) != 4 || &result[0] != &input[0] {
		t.Errorf("returned slice = %v (len=%d cap=%d), want shared slice with len=3 cap=4", result, len(result), cap(result))
	}
	// Passing a slice copies its header; append must not change the caller's length.
	if len(input) != 2 {
		t.Errorf("caller slice length = %d, want 2", len(input))
	}
}

func TestReflectCallNonVariadic(t *testing.T) {
	input := []int{1, 2}
	f := reflect.ValueOf(func(prefix string, values []int) (string, int) {
		values[0] = 42
		return prefix, values[1]
	})
	got := ReflectCall(f, []reflect.Value{reflect.ValueOf("fixed"), reflect.ValueOf(input)})
	if got[0].String() != "fixed" || got[1].Int() != 2 || input[0] != 42 {
		t.Errorf("non-variadic call = (%q, %d), input = %v", got[0].String(), got[1].Int(), input)
	}
}
