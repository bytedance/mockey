//go:build go1.27
// +build go1.27

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
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"testing"

	methodsv1 "github.com/bytedance/mockey/internal/fn/type4test/genericmethods.v1"
)

type methodReceiver127 struct {
	label  string
	number int
}

// Allocate pointer receivers on the heap to keep these regressions separate
// from the existing stack-pointer limitation tracked in #130.
//
//go:noinline
func newMethodReceiver127(label string, number int) *methodReceiver127 {
	return &methodReceiver127{label: label, number: number}
}

func (r methodReceiver127) Echo[T any](value T) T         { return value }
func (r *methodReceiver127) PointerEcho[T any](value T) T { return value }

func (r methodReceiver127) func1[T any](value T) T { return value }

func (r methodReceiver127) func2(value int) int { return value + r.number }

type parameterizedReceiver127[T any] struct{ value T }

//go:noinline
func newParameterizedReceiver127[T any](value T) *parameterizedReceiver127[T] {
	return &parameterizedReceiver127[T]{value: value}
}

func (r parameterizedReceiver127[T]) Echo[U any](value U) U         { return value }
func (r *parameterizedReceiver127[T]) PointerEcho[U any](value U) U { return value }

type emptyMethodReceiver127 struct{}

type promotedMethodReceiver127 struct{ methodReceiver127 }

var pointerMethodExpression127 = (*methodReceiver127).PointerEcho[int]

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
		mock := MockGeneric(methodReceiver127.Echo[int]).Origin(&origin).To(func(got methodReceiver127, value int) int {
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
		receiver := newMethodReceiver127("receiver", 7)
		var origin func(int) int
		mock := Mock((*methodReceiver127).PointerEcho[int], OptMethod).Origin(&origin).To(func(value int) int { return origin(value + 1) }).Build()
		defer mock.UnPatch()
		if got := receiver.PointerEcho[int](3); got != 4 {
			t.Fatalf("got %d, want 4", got)
		}
	})
	t.Run("receiver and method type parameters", func(t *testing.T) {
		receiver := parameterizedReceiver127[string]{value: "a"}
		mock := MockGeneric(parameterizedReceiver127[string].Echo[int]).To(func(got parameterizedReceiver127[string], value int) int {
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
		receiver := newParameterizedReceiver127("a")
		mock := Mock((*parameterizedReceiver127[string]).PointerEcho[int], OptMethod).Return(99).Build()
		defer mock.UnPatch()
		if got := receiver.PointerEcho[int](3); got != 99 {
			t.Fatalf("got %d, want 99", got)
		}
		if got := receiver.PointerEcho[namedInt](3); got != 3 {
			t.Fatalf("other method instantiation got %d", got)
		}
		other := newParameterizedReceiver127(namedString("a"))
		if got := other.PointerEcho[int](3); got != 3 {
			t.Fatalf("other receiver instantiation got %d", got)
		}
	})
	t.Run("large receiver arguments and results", func(t *testing.T) {
		receiver := largeMethodValue127{}
		receiver.values[0], receiver.values[39] = "receiver first", "receiver last"
		argument := largeMethodValue127{}
		argument.values[0], argument.values[39] = "argument first", "argument last"
		mock := MockGeneric(largeMethodValue127.Echo[largeMethodValue127]).To(func(got, value largeMethodValue127) (largeMethodValue127, largeMethodValue127) {
			if got != receiver || value != argument {
				t.Fatal("large receiver or argument corrupted")
			}
			got.values[1] = "mocked"
			return got, value
		}).Build()
		defer mock.UnPatch()
		gotReceiver, gotArgument := receiver.Echo[largeMethodValue127](argument)
		receiver.values[1] = "mocked"
		if gotReceiver != receiver || gotArgument != argument {
			t.Fatal("large result corrupted")
		}
	})
	t.Run("variadic arguments", func(t *testing.T) {
		values := []string{"a", "b"}
		// Retain the backing array on the heap across reflective Origin calls.
		t.Cleanup(func() {
			if !reflect.DeepEqual(values, []string{"a", "b"}) {
				t.Fatalf("variadic input changed: %v", values)
			}
		})
		var origin func(methodReceiver127, string, ...string) []string
		mock := Mock(methodReceiver127.Variadic[string], OptGeneric).Origin(&origin).To(func(r methodReceiver127, prefix string, values ...string) []string {
			return origin(r, "mock "+prefix, values...)
		}).Build()
		defer mock.UnPatch()
		if got := (methodReceiver127{}).Variadic[string]("prefix", values...); !reflect.DeepEqual(got, []string{"mock prefix", "a", "b"}) {
			t.Fatalf("got %v", got)
		}
	})
}

func TestGenericMethodEmptyReceiver127(t *testing.T) {
	mock := Mock(emptyMethodReceiver127.Zero[int], OptMethod).Return(42).Build()
	defer mock.UnPatch()
	if got := (emptyMethodReceiver127{}).Zero[int](); got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestGenericMethodDynamicDictionary127(t *testing.T) {
	type namedString string
	receiver := newParameterizedReceiver127("receiver")
	other := newParameterizedReceiver127(namedString("other"))
	first := MockGeneric(genericMethodExpression127[string]()).Return(42).Build()
	t.Cleanup(func() { first.UnPatch() })
	if got := receiver.PointerEcho[int](3); got != 42 {
		t.Fatalf("first instantiation got %d, want 42", got)
	}
	if got := other.PointerEcho[int](3); got != 3 {
		t.Fatalf("unmocked instantiation got %d, want 3", got)
	}
	PatchRun(func() {
		Mock(genericMethodExpression127[namedString](), OptMethod).Return(99).Build()
		if got := receiver.PointerEcho[int](3); got != 42 {
			t.Fatalf("first instantiation got %d, want 42", got)
		}
		if got := other.PointerEcho[int](3); got != 99 {
			t.Fatalf("second instantiation got %d, want 99", got)
		}
	})
	if got := receiver.PointerEcho[int](3); got != 42 {
		t.Fatalf("first instantiation after unpatch got %d, want 42", got)
	}
	if got := other.PointerEcho[int](3); got != 3 {
		t.Fatalf("restored instantiation got %d, want 3", got)
	}
}

func TestGenericMethodGCLifetime127(t *testing.T) {
	receiver := newParameterizedReceiver127("receiver")
	mock := MockGeneric(genericMethodExpression127[string]()).
		To(func(got *parameterizedReceiver127[string], value int) int {
			return len(got.value) + value
		}).Build()
	defer mock.UnPatch()
	for i := 0; i < 3; i++ {
		runtime.GC()
		if got := receiver.PointerEcho[int](3); got != 11 {
			t.Fatalf("after GC got %d, want 11", got)
		}
	}
}

func (r *methodReceiver127) Counted[T any](value T) T {
	if r == nil || r.label == "" {
		panic("original method called with a probe receiver")
	}
	r.number++
	return value
}

func TestGenericMethodProbe127(t *testing.T) {
	receiver := newMethodReceiver127("receiver", 0)
	// Retain the receiver until cleanup, including across stack growth in Origin.
	t.Cleanup(func() {
		if receiver.number != 2 {
			t.Errorf("original body calls = %d, want 2", receiver.number)
		}
	})
	var origin func(*methodReceiver127, int) int
	// Resolving the dictionary must not execute the original method body.
	builder := MockGeneric((*methodReceiver127).Counted[int])
	mock := builder.Origin(&origin).
		When(func(r *methodReceiver127, value int) bool { return value == 3 }).
		To(func(r *methodReceiver127, value int) int { return origin(r, value+10) }).Build()
	defer mock.UnPatch()
	if got := receiver.Counted[int](3); got != 13 || receiver.number != 1 {
		t.Fatalf("matched call got %d, body calls %d; want 13, 1", got, receiver.number)
	}
	if got := receiver.Counted[int](4); got != 4 || receiver.number != 2 {
		t.Fatalf("unmatched call got %d, body calls %d; want 4, 2", got, receiver.number)
	}
	if mock.Times() != 2 || mock.MockTimes() != 1 {
		t.Fatalf("calls = %d, mocked calls = %d; want 2, 1", mock.Times(), mock.MockTimes())
	}
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
	mock := Mock((*rand.Rand).N[int], OptMethod).Return(42).Build()
	defer mock.UnPatch()
	if got := generator.N[int](100); got != 42 {
		t.Fatalf("Rand.N[int] = %d, want 42", got)
	}
}

func TestGenericMethodExplicitOption127(t *testing.T) {
	t.Run("method option discovers generic method", func(t *testing.T) {
		mock := Mock((*methodReceiver127).PointerEcho[int], OptMethod).Return(42).Build()
		defer mock.UnPatch()
		if got := newMethodReceiver127("receiver", 0).PointerEcho[int](3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("generic option discovers method receiver", func(t *testing.T) {
		receiver := methodReceiver127{label: "receiver", number: 7}
		mock := MockGeneric(methodReceiver127.Echo[int]).To(func(got methodReceiver127, value int) int {
			if got != receiver {
				t.Fatalf("receiver got %v, want %v", got, receiver)
			}
			return value + got.number
		}).Build()
		defer mock.UnPatch()
		if got := receiver.Echo[int](3); got != 10 {
			t.Fatalf("direct method call got %d, want 10", got)
		}
	})
	t.Run("anonymous-looking generic method name", func(t *testing.T) {
		mock := MockGeneric(methodReceiver127.func1[int]).Return(42).Build()
		defer mock.UnPatch()
		if got := (methodReceiver127{number: 7}).func1[int](3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("non-generic method keeps existing behavior", func(t *testing.T) {
		mock := Mock(methodReceiver127.func2, OptMethod).Return(42).Build()
		defer mock.UnPatch()
		if got := (methodReceiver127{number: 7}).func2(3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("both explicit options remain supported", func(t *testing.T) {
		mock := MockGeneric((*methodReceiver127).PointerEcho[int], OptMethod).Return(42).Build()
		defer mock.UnPatch()
		if got := newMethodReceiver127("receiver", 7).PointerEcho[int](3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("global method expression", func(t *testing.T) {
		mock := MockGeneric(pointerMethodExpression127).Return(42).Build()
		defer mock.UnPatch()
		if got := newMethodReceiver127("receiver", 7).PointerEcho[int](3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("generic info hook and origin", func(t *testing.T) {
		receiver := methodReceiver127{label: "receiver", number: 7}
		var origin func(GenericInfo, methodReceiver127, int) int
		mock := MockGeneric(methodReceiver127.Echo[int]).Origin(&origin).
			To(func(info GenericInfo, got methodReceiver127, value int) int {
				if info == 0 || got != receiver {
					t.Fatalf("got dictionary %x and receiver %v", info, got)
				}
				return origin(info, got, value+got.number)
			}).Build()
		defer mock.UnPatch()
		if got := receiver.Echo[int](3); got != 10 {
			t.Fatalf("direct method call got %d, want 10", got)
		}
	})
}

func TestGenericMethodUnsupportedReceiver127(t *testing.T) {
	receiver := newMethodReceiver127("receiver", 7)
	for _, tt := range []struct {
		name  string
		build func()
	}{
		{"bound value", func() { MockGeneric(receiver.Echo[int]) }},
		{"bound pointer", func() { MockGeneric(receiver.PointerEcho[int]) }},
		{"promoted value", func() { MockGeneric(promotedMethodReceiver127.Echo[int]) }},
		{"promoted pointer", func() { Mock((*promotedMethodReceiver127).PointerEcho[int], OptMethod) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				message, ok := recover().(string)
				if !ok || !strings.Contains(message, "declaring receiver's method expression") {
					t.Fatalf("got panic %q, want method-expression guidance", message)
				}
			}()
			tt.build()
		})
	}
	if got := receiver.Echo[int](3); got != 3 {
		t.Fatalf("value method changed after rejection: got %d, want 3", got)
	}
	if got := receiver.PointerEcho[int](3); got != 3 {
		t.Fatalf("pointer method changed after rejection: got %d, want 3", got)
	}
}

func TestGenericMethodVersionedPackage127(t *testing.T) {
	t.Run("generic method", func(t *testing.T) {
		mock := MockGeneric(methodsv1.Receiver.Echo[int]).Return(42).Build()
		defer mock.UnPatch()
		if got := (methodsv1.Receiver{}).Echo[int](3); got != 42 {
			t.Fatalf("direct method call got %d, want 42", got)
		}
	})
	t.Run("ordinary generic function", func(t *testing.T) {
		target := methodsv1.GenericFunction()
		mock := MockGeneric(target).Return(42).Build()
		defer mock.UnPatch()
		if got := target(3); got != 42 {
			t.Fatalf("generic function call got %d, want 42", got)
		}
	})
}
