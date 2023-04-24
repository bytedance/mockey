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
	"fmt"
	"reflect"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestGetNestedMethod(t *testing.T) {
	convey.Convey("TestGetNestedMethod", t, func() {
		convey.Convey("basic cases", func() {
			convey.Convey("case nil", func() {
				convey.So(func() { GetNestedMethod(nil, "unknown") }, convey.ShouldPanicWith, "can't reflect instance method :unknown")
			})

			convey.Convey("case testA", func() {
				instance := testA{}
				convey.So(func() { GetNestedMethod(instance, "FooB") }, convey.ShouldPanicWith, "can't reflect instance method :FooB")
				convey.So(func() { GetNestedMethod(instance, "FooA") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarA") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarC") }, convey.ShouldNotPanic)
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "shouldn't here")
				convey.So(func() {
					reflect.ValueOf(GetNestedMethod(instance, "FooC")).Call([]reflect.Value{reflect.ValueOf(instance.testC)})
				}, convey.ShouldNotPanic)
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "shouldn't here")
				convey.So(func() {
					reflect.ValueOf(GetNestedMethod(instance, "BarC")).Call([]reflect.Value{reflect.ValueOf(&instance.testC)})
				}, convey.ShouldNotPanic)
			})

			convey.Convey("case testB", func() {
				instance := testB{}
				convey.So(func() { GetNestedMethod(instance, "FooA") }, convey.ShouldPanicWith, "can't reflect instance method :FooA")
				convey.So(func() { GetNestedMethod(instance, "FooB") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarB") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarC") }, convey.ShouldNotPanic)
			})

			convey.Convey("case testD", func() {
				instance := testD{}
				convey.So(func() { GetNestedMethod(instance, "FooA") }, convey.ShouldPanicWith, "can't reflect instance method :FooA")
				convey.So(func() { GetNestedMethod(instance, "FooD") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarD") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "FooB") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarB") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetNestedMethod(instance, "BarC") }, convey.ShouldNotPanic)
			})
		})

		PatchConvey("patch cases", func() {
			PatchConvey("case testA", func() {
				instance := testA{}
				Mock(GetNestedMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetNestedMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "shouldn't here") // no effect, didn't call testC.FooC()
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "shouldn't here") // no effect, didn't call testC.BarC()
			})

			PatchConvey("case testB", func() {
				instance := testB{testC: &testC{}}
				Mock(GetNestedMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetNestedMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "should here")
			})

			PatchConvey("case testD", func() {
				instance := testD{testB: &testB{testC: &testC{}}}
				Mock(GetNestedMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetNestedMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "should here")
			})
		})
	})
}

type testA struct {
	testC
}

func (a testA) FooA() {}

func (a *testA) BarA() {}

func (a testA) FooC() {
	panic("shouldn't here")
}

func (a *testA) BarC() {
	panic("shouldn't here")
}

type testB struct {
	*testC
}

func (b testB) FooB() {}

func (b *testB) BarB() {}

type testC struct{}

func (s testC) FooC() {
	fmt.Print("")
}

func (s *testC) BarC() {
	fmt.Print("")
}

type testD struct {
	*testB
}

func (s testD) FooD() {}

func (s *testD) BarD() {}

type Fn func()

type testFuncField struct {
	Public  Fn
	private Fn
}

func NewTestFuncField() *testFuncField {
	return &testFuncField{
		Public:  func() { panic("shouldn't here") },
		private: func() { panic("shouldn't here") },
	}
}

func TestGetMethod(t *testing.T) {
	convey.Convey("TestGetMethod", t, func() {
		convey.Convey("basic cases", func() {
			convey.Convey("case nil", func() {
				convey.So(func() { GetMethod(nil, "unknown") }, convey.ShouldPanicWith, "can't reflect instance method :unknown")
			})

			convey.Convey("case testA", func() {
				instance := testA{}
				convey.So(func() { GetMethod(instance, "FooB") }, convey.ShouldPanicWith, "can't reflect instance method :FooB")
				convey.So(func() { GetMethod(instance, "FooA") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarA") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarC") }, convey.ShouldNotPanic)
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "shouldn't here")
				convey.So(func() {
					reflect.ValueOf(GetMethod(instance, "FooC")).Call([]reflect.Value{reflect.ValueOf(instance.testC)})
				}, convey.ShouldNotPanic)
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "shouldn't here")
				convey.So(func() {
					reflect.ValueOf(GetMethod(instance, "BarC")).Call([]reflect.Value{reflect.ValueOf(&instance.testC)})
				}, convey.ShouldNotPanic)
			})

			convey.Convey("case testB", func() {
				instance := testB{}
				convey.So(func() { GetMethod(instance, "FooA") }, convey.ShouldPanicWith, "can't reflect instance method :FooA")
				convey.So(func() { GetMethod(instance, "FooB") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarB") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarC") }, convey.ShouldNotPanic)
			})

			convey.Convey("case testD", func() {
				instance := testD{}
				convey.So(func() { GetMethod(instance, "FooA") }, convey.ShouldPanicWith, "can't reflect instance method :FooA")
				convey.So(func() { GetMethod(instance, "FooD") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarD") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "FooB") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarB") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "FooC") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "BarC") }, convey.ShouldNotPanic)
			})

			convey.Convey("case testFuncField", func() {
				var instance interface{} = NewTestFuncField()
				convey.So(func() { GetMethod(instance, "Public") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "private") }, convey.ShouldNotPanic)
				convey.So(func() { GetMethod(instance, "notExist") }, convey.ShouldPanicWith, "can't reflect instance method :notExist")
				convey.So(func() {
					reflect.ValueOf(GetMethod(instance, "Public")).Call([]reflect.Value{})
				}, convey.ShouldPanicWith, "shouldn't here")
				convey.So(func() {
					reflect.ValueOf(GetMethod(instance, "private")).Call([]reflect.Value{})
				}, convey.ShouldPanicWith, "shouldn't here")
			})
		})

		PatchConvey("patch cases", func() {
			PatchConvey("case testA", func() {
				instance := testA{}
				Mock(GetMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "shouldn't here") // no effect, didn't call testC.FooC()
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "shouldn't here") // no effect, didn't call testC.BarC()
			})

			PatchConvey("case testB", func() {
				instance := testB{testC: &testC{}}
				Mock(GetMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "should here")
			})

			PatchConvey("case testD", func() {
				instance := testD{testB: &testB{testC: &testC{}}}
				Mock(GetMethod(instance, "FooC")).To(func() { panic("should here") }).Build()
				Mock(GetMethod(instance, "BarC")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.FooC() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { instance.BarC() }, convey.ShouldPanicWith, "should here")
			})

			PatchConvey("case testFuncField", func() {
				instance := NewTestFuncField()
				caller := NewTestFuncField()
				Mock(GetMethod(instance, "Public")).To(func() { panic("should here") }).Build()
				Mock(GetMethod(instance, "private")).To(func() { panic("should here") }).Build()
				convey.So(func() { instance.Public() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { caller.Public() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { instance.private() }, convey.ShouldPanicWith, "should here")
				convey.So(func() { caller.private() }, convey.ShouldPanicWith, "should here")
			})
		})
	})
}

type testNested interface {
	FooNested()
}
type testOuter struct {
	testNested
}

type testInner struct {
	_ int
}

func (testInner) FooNested() {
	panic("not here")
}

type testInnerP struct {
	_ string
}

func (*testInnerP) FooNested() {
	panic("not here p")
}

func TestGetNested(t *testing.T) {
	PatchConvey("TestGetNested", t, func() {
		PatchConvey("instance implement", func() {
			var obj testNested
			PatchConvey("pointer container", func() {
				obj = &testOuter{testNested: testInner{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("struct container", func() {
				obj = testOuter{testNested: testInner{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("nested container", func() {
				obj = &testOuter{
					testNested: testOuter{
						testNested: &testOuter{
							testNested: testOuter{
								testNested: testInner{},
							},
						},
					},
				}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
		})
		PatchConvey("instance implement(but pointer field)", func() {
			var obj testNested
			PatchConvey("pointer container", func() {
				obj = &testOuter{testNested: &testInner{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("struct container", func() {
				obj = testOuter{testNested: &testInner{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("nested container", func() {
				obj = &testOuter{
					testNested: testOuter{
						testNested: &testOuter{
							testNested: testOuter{
								testNested: &testInner{},
							},
						},
					},
				}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
		})
		PatchConvey("pointer implement", func() {
			var obj testNested
			PatchConvey("pointer container", func() {
				obj = &testOuter{testNested: &testInnerP{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("struct container", func() {
				obj = testOuter{testNested: &testInnerP{}}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
			PatchConvey("nested container", func() {
				obj = &testOuter{
					testNested: testOuter{
						testNested: &testOuter{
							testNested: testOuter{
								testNested: &testInnerP{},
							},
						},
					},
				}
				convey.So(func() {
					Mock(GetMethod(obj, "FooNested")).Return().Build()
				}, convey.ShouldNotPanic)
				convey.So(func() {
					obj.FooNested()
				}, convey.ShouldNotPanic)
			})
		})
	})
}
