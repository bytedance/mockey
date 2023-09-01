//go:build go1.18
// +build go1.18

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
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func sum[T int | float64](l, r T) T {
	return l + r
}

type generic[T int | string] struct {
	a T
}

func (g generic[T]) Value() T {
	return g.a
}

func (g generic[T]) Value2() T {
	return g.a + g.a
}

func TestGeneric(t *testing.T) {
	PatchConvey("generic", t, func() {
		PatchConvey("func", func() {
			MockGeneric(sum[int]).To(func(a, b int) int {
				return 999
			}).Build()
			MockGeneric(sum[float64]).Return(888).Build()
			convey.So(sum[int](1, 2), convey.ShouldEqual, 999)
			convey.So(sum[float64](1, 2), convey.ShouldEqual, 888)
		})
		PatchConvey("type", func() {
			Mock((generic[int]).Value, OptGeneric).Return(999).Build()
			Mock(GetMethod(generic[string]{}, "Value2"), OptGeneric).To(func() string {
				return "mock"
			}).Build()
			gi := generic[int]{a: 123}
			gs := generic[string]{a: "abc"}
			convey.So(gi.Value(), convey.ShouldEqual, 999)
			convey.So(gs.Value2(), convey.ShouldEqual, "mock")
		})
	})
}

type Large15[T any] struct {
	_, _, _, _, _, _, _, _, _, _, _, _, _, _, _, _ T
}

func TestGenericArg(t *testing.T) {
	PatchConvey("args", t, func() {
		GenericArgRunner[uint8]("uint8")
		GenericArgRunner[uint16]("uint16")
		GenericArgRunner[uint32]("uint32")
		GenericArgRunner[uint64]("uint64")
		GenericArgRunner[Large15[int]]("large15[int]")
		GenericArgRunner[Large15[Large15[int]]]("Large15[Large15[int]")
		GenericArgRunner[Large15[Large15[[100]byte]]]("Large15[Large15[[100]byte]")
	})
}

func TestGenericRet(t *testing.T) {
	PatchConvey("rets", t, func() {
		GenericRetRunner[uint8]("uint8")
		GenericRetRunner[uint16]("uint16")
		GenericRetRunner[uint32]("uint32")
		GenericRetRunner[uint64]("uint64")
		GenericRetRunner[Large15[int]]("large15[int]")
		GenericRetRunner[Large15[Large15[int]]]("Large15[Large15[int]")
		GenericRetRunner[Large15[Large15[[100]byte]]]("Large15[Large15[[100]byte]")
	})
}

func TestGenericArgRet(t *testing.T) {
	PatchConvey("args-rets", t, func() {
		GenericArgRetRunner[uint8]("uint8")
		GenericArgRetRunner[uint16]("uint16")
		GenericArgRetRunner[uint32]("uint32")
		GenericArgRetRunner[uint64]("uint64")
		GenericArgRetRunner[Large15[int]]("large15[int]")
		GenericArgRetRunner[Large15[Large15[int]]]("Large15[Large15[int]")
		GenericArgRetRunner[Large15[Large15[[100]byte]]]("Large15[Large15[[100]byte]")
	})
}

func GenericsArg0[T any]()                                            { panic("0") }
func GenericsArg1[T any](_ T)                                         { panic("1") }
func GenericsArg2[T any](_, _ T)                                      { panic("2") }
func GenericsArg3[T any](_, _, _ T)                                   { panic("3") }
func GenericsArg4[T any](_, _, _, _ T)                                { panic("4") }
func GenericsArg5[T any](_, _, _, _, _ T)                             { panic("5") }
func GenericsArg6[T any](_, _, _, _, _, _ T)                          { panic("6") }
func GenericsArg7[T any](_, _, _, _, _, _, _ T)                       { panic("7") }
func GenericsArg8[T any](_, _, _, _, _, _, _, _ T)                    { panic("8") }
func GenericsArg9[T any](_, _, _, _, _, _, _, _, _ T)                 { panic("9") }
func GenericsArg10[T any](_, _, _, _, _, _, _, _, _, _ T)             { panic("10") }
func GenericsArg11[T any](_, _, _, _, _, _, _, _, _, _, _ T)          { panic("11") }
func GenericsArg12[T any](_, _, _, _, _, _, _, _, _, _, _, _ T)       { panic("12") }
func GenericsArg13[T any](_, _, _, _, _, _, _, _, _, _, _, _, _ T)    { panic("13") }
func GenericsArg14[T any](_, _, _, _, _, _, _, _, _, _, _, _, _, _ T) { panic("14") }
func GenericsArg15[T any](_, _, _, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("15")
}

func GenericArgRunner[T any](name string) {
	// type T = context.Context
	var arg T

	PatchConvey(name, func() {
		MockGeneric(GenericsArg0[T]).Return().Build()
		MockGeneric(GenericsArg1[T]).Return().Build()
		MockGeneric(GenericsArg2[T]).Return().Build()
		MockGeneric(GenericsArg3[T]).Return().Build()
		MockGeneric(GenericsArg4[T]).Return().Build()
		MockGeneric(GenericsArg5[T]).Return().Build()
		MockGeneric(GenericsArg6[T]).Return().Build()
		MockGeneric(GenericsArg7[T]).Return().Build()
		MockGeneric(GenericsArg8[T]).Return().Build()
		MockGeneric(GenericsArg9[T]).Return().Build()
		MockGeneric(GenericsArg10[T]).Return().Build()
		MockGeneric(GenericsArg11[T]).Return().Build()
		MockGeneric(GenericsArg12[T]).Return().Build()
		MockGeneric(GenericsArg13[T]).Return().Build()
		MockGeneric(GenericsArg14[T]).Return().Build()
		MockGeneric(GenericsArg15[T]).Return().Build()
		convey.So(func() { GenericsArg0[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg1(arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg2(arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg3(arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg4(arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg5(arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg6(arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg7(arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg8(arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg9(arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg10(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg11(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg12(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg13(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg14(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArg15(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
	})

	convey.So(func() { GenericsArg0[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsArg1(arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg2(arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg3(arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg4(arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg5(arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg6(arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg7(arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg8(arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg9(arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg10(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg11(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg12(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg13(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg14(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg15(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
}

func GenericsRet0[T any]()                                            { panic("0") }
func GenericsRet1[T any]() (_ T)                                      { panic("1") }
func GenericsRet2[T any]() (_, _ T)                                   { panic("2") }
func GenericsRet3[T any]() (_, _, _ T)                                { panic("3") }
func GenericsRet4[T any]() (_, _, _, _ T)                             { panic("4") }
func GenericsRet5[T any]() (_, _, _, _, _ T)                          { panic("5") }
func GenericsRet6[T any]() (_, _, _, _, _, _ T)                       { panic("6") }
func GenericsRet7[T any]() (_, _, _, _, _, _, _ T)                    { panic("7") }
func GenericsRet8[T any]() (_, _, _, _, _, _, _, _ T)                 { panic("8") }
func GenericsRet9[T any]() (_, _, _, _, _, _, _, _, _ T)              { panic("9") }
func GenericsRet10[T any]() (_, _, _, _, _, _, _, _, _, _ T)          { panic("10") }
func GenericsRet11[T any]() (_, _, _, _, _, _, _, _, _, _, _ T)       { panic("11") }
func GenericsRet12[T any]() (_, _, _, _, _, _, _, _, _, _, _, _ T)    { panic("12") }
func GenericsRet13[T any]() (_, _, _, _, _, _, _, _, _, _, _, _, _ T) { panic("13") }
func GenericsRet14[T any]() (_, _, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("14")
}

func GenericsRet15[T any]() (_, _, _, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("15")
}

func GenericRetRunner[T any](name string) {
	var arg T
	PatchConvey(name, func() {
		MockGeneric(GenericsRet0[T]).Return().Build()
		MockGeneric(GenericsRet1[T]).Return(arg).Build()
		MockGeneric(GenericsRet2[T]).Return(arg, arg).Build()
		MockGeneric(GenericsRet3[T]).Return(arg, arg, arg).Build()
		MockGeneric(GenericsRet4[T]).Return(arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet5[T]).Return(arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet6[T]).Return(arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet7[T]).Return(arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet8[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet9[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet10[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet11[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet12[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet13[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet14[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsRet15[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		convey.So(func() { GenericsRet0[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet1[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet2[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet3[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet4[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet5[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet6[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet7[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet8[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet9[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet10[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet11[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet12[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet13[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet14[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsRet15[T]() }, convey.ShouldNotPanic)
	})

	convey.So(func() { GenericsRet0[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet1[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet2[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet3[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet4[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet5[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet6[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet7[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet8[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet9[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet10[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet11[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet12[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet13[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet14[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsRet15[T]() }, convey.ShouldPanic)
}

func GenericsArgRet0[T any]()                                        { panic("0") }
func GenericsArgRet1[T any](_ T) (_ T)                               { panic("1") }
func GenericsArgRet2[T any](_, _ T) (_, _ T)                         { panic("2") }
func GenericsArgRet3[T any](_, _, _ T) (_, _, _ T)                   { panic("3") }
func GenericsArgRet4[T any](_, _, _, _ T) (_, _, _, _ T)             { panic("4") }
func GenericsArgRet5[T any](_, _, _, _, _ T) (_, _, _, _, _ T)       { panic("5") }
func GenericsArgRet6[T any](_, _, _, _, _, _ T) (_, _, _, _, _, _ T) { panic("6") }
func GenericsArgRet7[T any](_, _, _, _, _, _, _ T) (_, _, _, _, _, _, _ T) {
	panic("7")
}

func GenericsArgRet8[T any](_, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _ T) {
	panic("8")
}

func GenericsArgRet9[T any](_, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _ T) {
	panic("9")
}

func GenericsArgRet10[T any](_, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _ T) {
	panic("10")
}

func GenericsArgRet11[T any](_, _, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _, _ T) {
	panic("11")
}

func GenericsArgRet12[T any](_, _, _, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("12")
}

func GenericsArgRet13[T any](_, _, _, _, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("13")
}

func GenericsArgRet14[T any](_, _, _, _, _, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("14")
}

func GenericsArgRet15[T any](_, _, _, _, _, _, _, _, _, _, _, _, _, _, _ T) (_, _, _, _, _, _, _, _, _, _, _, _, _, _, _ T) {
	panic("15")
}

func GenericArgRetRunner[T any](name string) {
	var arg T
	PatchConvey(name, func() {
		MockGeneric(GenericsArgRet0[T]).Return().Build()
		MockGeneric(GenericsArgRet1[T]).Return(arg).Build()
		MockGeneric(GenericsArgRet2[T]).Return(arg, arg).Build()
		MockGeneric(GenericsArgRet3[T]).Return(arg, arg, arg).Build()
		MockGeneric(GenericsArgRet4[T]).Return(arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet5[T]).Return(arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet6[T]).Return(arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet7[T]).Return(arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet8[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet9[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet10[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet11[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet12[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet13[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet14[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		MockGeneric(GenericsArgRet15[T]).Return(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg).Build()
		convey.So(func() { GenericsArgRet0[T]() }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet1(arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet2(arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet3(arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet4(arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet5(arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet6(arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet7(arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet8(arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet9(arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet10(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet11(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet12(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet13(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet14(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
		convey.So(func() { GenericsArgRet15(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldNotPanic)
	})

	convey.So(func() { GenericsArgRet0[T]() }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet1(arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet2(arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet3(arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet4(arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet5(arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet6(arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet7(arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet8(arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet9(arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet10(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet11(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet12(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet13(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArgRet14(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
	convey.So(func() { GenericsArg15(arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg, arg) }, convey.ShouldPanic)
}
