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
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// The callers below keep their callback and pointee on their own stack: the
// targets let neither argument escape, so escape analysis allows it. A mock
// hook receives those arguments through reflect, which holds them on the heap.
// If the goroutine stack moves while the hook runs (here: forced from a When
// condition, i.e. after the arguments were captured and before the hook body
// sees them), the hook must still reach the caller's live data, not the stale
// copy left behind on the old stack.

//go:noinline
func stackMoveCallback(f func(int)) {
	f(0)
}

//go:noinline
func stackMovePointer(p *int) {
	*p = 0
}

//go:noinline
func callStackMoveCallback() int {
	m := make(map[int]int) // does not escape: lives in this frame
	stackMoveCallback(func(v int) { m[0] = v })
	return m[0]
}

//go:noinline
func callStackMovePointer() int {
	x := -1 // does not escape: lives in this frame
	stackMovePointer(&x)
	return x
}

// growGoroutineStack forces at least one copystack by touching ~1 MiB of stack.
//
//go:noinline
func growGoroutineStack(n int) int {
	var buf [1024]byte
	buf[n%len(buf)] = byte(n)
	if n <= 0 {
		return int(buf[0])
	}
	return growGoroutineStack(n-1) + int(buf[(n*7)%len(buf)])
}

func TestHookArgsSurviveStackMove(t *testing.T) {
	PatchConvey("callback argument", t, func() {
		Mock(stackMoveCallback).When(func(f func(int)) bool {
			growGoroutineStack(1024)
			return true
		}).To(func(f func(int)) {
			f(42)
		}).Build()
		So(callStackMoveCallback(), ShouldEqual, 42)
	})

	PatchConvey("pointer argument", t, func() {
		Mock(stackMovePointer).When(func(p *int) bool {
			growGoroutineStack(1024)
			return true
		}).To(func(p *int) {
			*p = 42
		}).Build()
		So(callStackMovePointer(), ShouldEqual, 42)
	})

	PatchConvey("origin receives relocated arguments", t, func() {
		var origin func(func(int))
		Mock(stackMoveCallback).When(func(f func(int)) bool {
			growGoroutineStack(1024)
			return true
		}).To(func(f func(int)) {
			origin(func(v int) { f(v + 42) })
		}).Origin(&origin).Build()
		So(callStackMoveCallback(), ShouldEqual, 42)
	})

	// Regression: an Origin getter keeps the hook's []reflect.Value alive after
	// the hook returns. If that slice still points into the calling
	// goroutine's stack, the stack is freed when the goroutine exits and the
	// next GC aborts with "found bad pointer in Go heap".
	PatchConvey("arguments retained by the Origin getter do not pin a freed stack", t, func() {
		var origin func(func(int))
		mocker := Mock(stackMoveCallback).To(func(f func(int)) {
			origin(f)
		}).Origin(&origin).Build()
		done := make(chan int)
		go func() {
			growGoroutineStack(256) // non-standard stack size: freed on exit
			done <- callStackMoveCallback()
		}()
		So(<-done, ShouldEqual, 0)
		for i := 0; i < 3; i++ {
			runtime.GC()
		}
		runtime.KeepAlive(mocker)
		runtime.KeepAlive(origin)
	})
}
