/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to verify hook lifetime during garbage collection.
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
	"runtime"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Target(in string) string {
	return strings.Repeat(in, 1)
}

func Hook(in string) string {
	return "MOCKED!"
}

func TestPatchFunc(t *testing.T) {
	Convey("TestPatchFunc", t, func() {
		Convey("normal", func() {
			var proxy func(string) string
			patch := PatchFunc(Target, Hook, &proxy, false)
			So(Target("anything"), ShouldEqual, "MOCKED!")
			So(proxy("anything"), ShouldEqual, "anything")
			patch.Unpatch()
			So(Target("anything"), ShouldEqual, "anything")
		})
		Convey("anonymous hook", func() {
			var proxy func(string) string
			patch := PatchFunc(Target, func(string) string { return "MOCKED!" }, &proxy, false)
			So(Target("anything"), ShouldEqual, "MOCKED!")
			So(proxy("anything"), ShouldEqual, "anything")
			patch.Unpatch()
			So(Target("anything"), ShouldEqual, "anything")
		})
		Convey("closure hook", func() {
			var proxy func(string) string
			hookBuilder := func(x string) func(string) string {
				return func(string) string { return x }
			}
			patch := PatchFunc(Target, hookBuilder("MOCKED!"), &proxy, false)
			So(Target("anything"), ShouldEqual, "MOCKED!")
			So(proxy("anything"), ShouldEqual, "anything")
			patch.Unpatch()
			So(Target("anything"), ShouldEqual, "anything")
		})
		Convey("reflect hook", func() {
			var proxy func(string) string
			hookVal := reflect.MakeFunc(reflect.TypeOf(Hook), func(args []reflect.Value) (results []reflect.Value) {
				return []reflect.Value{reflect.ValueOf("MOCKED!")}
			})
			patch := PatchFunc(Target, hookVal.Interface(), &proxy, false)
			So(Target("anything"), ShouldEqual, "MOCKED!")
			So(proxy("anything"), ShouldEqual, "anything")
			patch.Unpatch()
			So(Target("anything"), ShouldEqual, "anything")
		})
	})
}

func TestPatchKeepsHookAlive(t *testing.T) {
	collected := make(chan struct{}, 1)
	var proxy func(string) string
	patch := PatchValue(reflect.ValueOf(Target), temporaryReflectHook(collected), reflect.ValueOf(&proxy), false)
	defer patch.Unpatch()
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	select {
	case <-collected:
		t.Fatal("active patch lost the hook closure's GC root")
	default:
	}
	if got := Target("original"); got != "retained hook" {
		t.Fatalf("hook result = %q", got)
	}
}

func temporaryReflectHook(collected chan<- struct{}) reflect.Value {
	payload := &struct {
		text    string
		padding [64]byte
	}{text: "retained hook"}
	runtime.SetFinalizer(payload, func(interface{}) { collected <- struct{}{} })
	return reflect.MakeFunc(reflect.TypeOf(Target), func([]reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(payload.text)}
	})
}
