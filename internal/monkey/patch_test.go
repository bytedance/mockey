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

package monkey

import (
	"reflect"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

//go:noinline
func Target() int {
	return 0
}

func Hook() int {
	return 2
}

func UnsafeTarget() {}

func TestPatchFunc(t *testing.T) {
	convey.Convey("TestPatchFunc", t, func() {
		convey.Convey("normal", func() {
			var proxy func() int
			patch := PatchFunc(Target, Hook, &proxy, false)
			convey.So(Target(), convey.ShouldEqual, 2)
			convey.So(proxy(), convey.ShouldEqual, 0)
			patch.Unpatch()
			convey.So(Target(), convey.ShouldEqual, 0)
		})
		convey.Convey("anonymous hook", func() {
			var proxy func() int
			patch := PatchFunc(Target, func() int { return 2 }, &proxy, false)
			convey.So(Target(), convey.ShouldEqual, 2)
			convey.So(proxy(), convey.ShouldEqual, 0)
			patch.Unpatch()
			convey.So(Target(), convey.ShouldEqual, 0)
		})
		convey.Convey("closure hook", func() {
			var proxy func() int
			hookBuilder := func(x int) func() int {
				return func() int { return x }
			}
			patch := PatchFunc(Target, hookBuilder(2), &proxy, false)
			convey.So(Target(), convey.ShouldEqual, 2)
			convey.So(proxy(), convey.ShouldEqual, 0)
			patch.Unpatch()
			convey.So(Target(), convey.ShouldEqual, 0)
		})
		convey.Convey("reflect hook", func() {
			var proxy func() int
			hookVal := reflect.MakeFunc(reflect.TypeOf(Hook), func(args []reflect.Value) (results []reflect.Value) { return []reflect.Value{reflect.ValueOf(2)} })
			patch := PatchFunc(Target, hookVal.Interface(), &proxy, false)
			convey.So(Target(), convey.ShouldEqual, 2)
			convey.So(proxy(), convey.ShouldEqual, 0)
			patch.Unpatch()
			convey.So(Target(), convey.ShouldEqual, 0)
		})
		convey.Convey("unsafe", func() {
			var proxy func()
			patch := PatchFunc(UnsafeTarget, func() { panic("good") }, &proxy, true)
			convey.So(func() { UnsafeTarget() }, convey.ShouldPanicWith, "good")
			convey.So(func() { proxy() }, convey.ShouldNotPanic)
			patch.Unpatch()
			convey.So(func() { UnsafeTarget() }, convey.ShouldNotPanic)
		})
	})
}
