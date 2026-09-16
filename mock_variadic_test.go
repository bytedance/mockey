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

package mockey

import "testing"

//go:noinline
func variadicSliceTarget(label string, values ...int) int {
	if len(values) == 0 {
		return len(label)
	}
	return values[0]
}

//go:noinline
func variadicSliceOriginTarget(values ...int) int {
	values[0]++
	return values[0]
}

// Keep the backing array on the heap so these tests isolate slice copying
// from the separate stack-pointer relocation issue.
//
//go:noinline
func variadicSliceValues() []int {
	return make([]int, 1, 2)
}

func TestMockVariadicSliceIdentity(t *testing.T) {
	t.Run("hook", func(t *testing.T) {
		m := Mock(variadicSliceTarget).To(func(label string, values ...int) int {
			values[0] = 42
			values = append(values, len(label))
			return values[0]
		}).Build()
		defer m.UnPatch()

		values := variadicSliceValues()
		got := variadicSliceTarget("label", values...)
		if got != 42 || values[0] != 42 || values[:cap(values)][1] != 5 {
			t.Fatalf("result=%d, backing array=%v; want 42 and [42 5]", got, values[:cap(values)])
		}
	})

	t.Run("condition", func(t *testing.T) {
		m := Mock(variadicSliceTarget).When(func(_ string, values ...int) bool {
			values[0] = 41
			return true
		}).To(func(_ string, values ...int) int {
			values[0]++
			return values[0]
		}).Build()
		defer m.UnPatch()

		values := variadicSliceValues()
		got := variadicSliceTarget("label", values...)
		if got != 42 || values[0] != 42 {
			t.Fatalf("result=%d, caller value=%d; want both 42", got, values[0])
		}
	})

	t.Run("condition miss", func(t *testing.T) {
		m := Mock(variadicSliceTarget).When(func(_ string, values ...int) bool {
			values[0] = 42
			return false
		}).Return(99).Build()
		defer m.UnPatch()

		values := variadicSliceValues()
		got := variadicSliceTarget("label", values...)
		if got != 42 || values[0] != 42 {
			t.Fatalf("result=%d, caller value=%d; want both 42", got, values[0])
		}
	})

	t.Run("origin", func(t *testing.T) {
		var original func(...int) int
		m := Mock(variadicSliceOriginTarget).Origin(&original).To(func(values ...int) int {
			return original(values...)
		}).Build()
		defer m.UnPatch()

		values := variadicSliceValues()
		values[0] = 41
		got := variadicSliceOriginTarget(values...)
		if got != 42 || values[0] != 42 {
			t.Fatalf("result=%d, caller value=%d; want both 42", got, values[0])
		}
	})
}

//go:noinline
func variadicNilTarget(values ...int) bool {
	return values == nil
}

func TestMockVariadicNilSlice(t *testing.T) {
	m := Mock(variadicNilTarget).To(func(values ...int) bool {
		return values == nil
	}).Build()
	defer m.UnPatch()

	if !variadicNilTarget() {
		t.Error("omitted variadic arguments should remain nil")
	}
	var nilValues []int
	if !variadicNilTarget(nilValues...) {
		t.Error("nil variadic arguments should remain nil")
	}
	if variadicNilTarget([]int{}...) {
		t.Error("non-nil empty variadic arguments should remain non-nil")
	}
}
