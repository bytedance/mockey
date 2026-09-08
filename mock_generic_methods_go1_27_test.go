//go:build go1.27
// +build go1.27

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

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

type methodReceiver127 struct {
	label  string
	number int
}

func (r methodReceiver127) Echo[T any](value T) T         { return value }
func (r *methodReceiver127) PointerEcho[T any](value T) T { return value }

type parameterizedReceiver127[T any] struct{ value T }

func (r parameterizedReceiver127[T]) Echo[U any](value U) U         { return value }
func (r *parameterizedReceiver127[T]) PointerEcho[U any](value U) U { return value }
func (r *parameterizedReceiver127[T]) Pair[U any](value U) (T, U)   { return r.value, value }

type emptyMethodReceiver127 struct{}

func (r emptyMethodReceiver127) Zero[T any]() T { var value T; return value }

func genericMethodExpression127[T any]() func(*parameterizedReceiver127[T], int) int {
	return (*parameterizedReceiver127[T]).PointerEcho[int]
}

type largeMethodValue127 struct{ values [40]string }

func (r largeMethodValue127) Echo[T any](value T) (largeMethodValue127, T) {
	return r, value
}

func (r methodReceiver127) Variadic[T any](prefix T, values ...T) []T {
	return append([]T{prefix}, values...)
}

func TestGenericMethodExpressions127(t *testing.T) {
	t.Run("value receiver arguments and origin", func(t *testing.T) {
		receiver := methodReceiver127{label: "receiver", number: 7}
		var origin func(methodReceiver127, int) int
		mock := Mock(methodReceiver127.Echo[int]).Origin(&origin).To(func(got methodReceiver127, value int) int {
			if got != receiver || value != 3 {
				t.Fatalf("got receiver %v, argument %d", got, value)
			}
			return origin(got, value+got.number)
		}).Build()
		defer mock.UnPatch()
		if got := receiver.Echo[int](3); got != 10 {
			t.Fatalf("got %d, want 10", got)
		}
		if got := methodReceiver127.Echo[int](receiver, 3); got != 10 {
			t.Fatalf("method expression got %d, want 10", got)
		}
	})
	t.Run("pointer receiver without receiver hook argument", func(t *testing.T) {
		receiver := &methodReceiver127{number: 7}
		var origin func(int) int
		mock := Mock((*methodReceiver127).PointerEcho[int]).Origin(&origin).To(func(value int) int { return origin(value + 1) }).Build()
		defer mock.UnPatch()
		if got := receiver.PointerEcho[int](3); got != 4 {
			t.Fatalf("got %d, want 4", got)
		}
	})
	t.Run("receiver and method type parameters", func(t *testing.T) {
		receiver := parameterizedReceiver127[string]{value: "a"}
		mock := Mock(parameterizedReceiver127[string].Echo[int]).To(func(got parameterizedReceiver127[string], value int) int {
			if got != receiver {
				t.Fatalf("got receiver %v", got)
			}
			return value + 10
		}).Build()
		defer mock.UnPatch()
		if got := receiver.Echo[int](3); got != 13 {
			t.Fatalf("got %d, want 13", got)
		}
	})
	t.Run("dictionary separates instantiations sharing a shape", func(t *testing.T) {
		type namedInt int
		type namedString string
		receiver := &parameterizedReceiver127[string]{value: "a"}
		mock := Mock((*parameterizedReceiver127[string]).PointerEcho[int]).Return(99).Build()
		defer mock.UnPatch()
		if got := receiver.PointerEcho[int](3); got != 99 {
			t.Fatalf("got %d, want 99", got)
		}
		if got := receiver.PointerEcho[namedInt](3); got != 3 {
			t.Fatalf("other method instantiation got %d", got)
		}
		other := &parameterizedReceiver127[namedString]{value: "a"}
		if got := other.PointerEcho[int](3); got != 3 {
			t.Fatalf("other receiver instantiation got %d", got)
		}
	})
	t.Run("large receiver arguments and results", func(t *testing.T) {
		receiver := largeMethodValue127{}
		receiver.values[0], receiver.values[39] = "receiver first", "receiver last"
		argument := largeMethodValue127{}
		argument.values[0], argument.values[39] = "argument first", "argument last"
		var origin func(largeMethodValue127, largeMethodValue127) (largeMethodValue127, largeMethodValue127)
		mock := Mock(largeMethodValue127.Echo[largeMethodValue127]).Origin(&origin).To(func(got, value largeMethodValue127) (largeMethodValue127, largeMethodValue127) {
			if got != receiver || value != argument {
				t.Fatal("large receiver or argument corrupted")
			}
			got.values[1] = "mocked"
			return origin(got, value)
		}).Build()
		defer mock.UnPatch()
		gotReceiver, gotArgument := receiver.Echo[largeMethodValue127](argument)
		receiver.values[1] = "mocked"
		if gotReceiver != receiver || gotArgument != argument {
			t.Fatal("large result corrupted")
		}
	})
	t.Run("variadic arguments", func(t *testing.T) {
		var origin func(methodReceiver127, string, ...string) []string
		mock := Mock(methodReceiver127.Variadic[string]).Origin(&origin).To(func(r methodReceiver127, prefix string, values ...string) []string {
			return origin(r, "mock "+prefix, values...)
		}).Build()
		defer mock.UnPatch()
		if got := (methodReceiver127{}).Variadic[string]("prefix", "a", "b"); !reflect.DeepEqual(got, []string{"mock prefix", "a", "b"}) {
			t.Fatalf("got %v", got)
		}
	})
}

func TestGenericPointerMethodValues127(t *testing.T) {
	receiver := &parameterizedReceiver127[string]{value: "receiver"}
	method := receiver.Pair[int]
	var origin func(int) (string, int)
	mock := Mock(method).Origin(&origin).To(func(value int) (string, int) { return origin(value + 10) }).Build()
	defer mock.UnPatch()
	if gotReceiver, got := method(3); gotReceiver != "receiver" || got != 13 {
		t.Fatalf("method value got %q, %d", gotReceiver, got)
	}
	other := &parameterizedReceiver127[string]{value: "other receiver"}
	if gotReceiver, got := other.Pair[int](3); gotReceiver != "other receiver" || got != 13 {
		t.Fatalf("direct method got %q, %d", gotReceiver, got)
	}
}

func TestGenericMethodEmptyReceiver127(t *testing.T) {
	mock := Mock(emptyMethodReceiver127.Zero[int]).Return(42).Build()
	defer mock.UnPatch()
	if got := (emptyMethodReceiver127{}).Zero[int](); got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestGenericMethodDynamicDictionary127(t *testing.T) {
	mock := Mock(genericMethodExpression127[string]()).Return(42).Build()
	defer mock.UnPatch()
	if got := (&parameterizedReceiver127[string]{}).PointerEcho[int](3); got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestGenericMethodUnsupportedValue127(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("bound value method must fail before patching; use a method expression")
		}
	}()
	Mock((methodReceiver127{}).Echo[int])
}

func TestGenericMethodUserClosure127(t *testing.T) {
	// A normal closure can have exactly the same call graph and signature as
	// the generated method-expression wrapper; it must not patch the method.
	wrapper := func(r methodReceiver127, value int) int { return r.Echo[int](value) }
	mock := Mock(wrapper).Return(99).Build()
	defer mock.UnPatch()
	if got := wrapper(methodReceiver127{}, 3); got != 99 {
		t.Fatalf("closure got %d, want 99", got)
	}
	if got := (methodReceiver127{}).Echo[int](3); got != 3 {
		t.Fatalf("unmocked method got %d, want 3", got)
	}
}

func TestGenericStandardLibraryMethod127(t *testing.T) {
	generator := rand.New(rand.NewPCG(1, 2))
	mock := Mock((*rand.Rand).N[int]).Return(42).Build()
	defer mock.UnPatch()
	if got := generator.N[int](100); got != 42 {
		t.Fatalf("Rand.N[int] = %d, want 42", got)
	}
}
