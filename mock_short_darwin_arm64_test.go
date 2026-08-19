//go:build darwin && arm64

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
	"reflect"
	"sync"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/inst"
	. "github.com/smartystreets/goconvey/convey"
)

// The functions below are deliberately one-liners: with inlining disabled but
// optimisation left on (-gcflags="all=-l"), each compiles to a body far shorter
// than the 24 bytes the long-jump entry sequence needs. Before near-trampoline
// support they all failed with "function is too short to patch".
//
// Under -gcflags="all=-N" the compiler also spills locals to the stack, which
// pads most of these bodies past the threshold and routes them back to the
// default long-jump path. Tests that are specifically about the short path
// therefore check takesShortPath first and skip when the compiler has made the
// target long: asserting there would exercise the default path, not this one.
// shortNop is a single RET under every flag combination, so it always runs.

//go:noinline
func shortNop() {}

//go:noinline
func shortNop2() {}

//go:noinline
func shortIdent(i int) int { return i }

//go:noinline
func shortConstString() string { return "original-constant" }

//go:noinline
func shortAdd(a, b int) int { return a + b }

// takesShortPath reports whether f is small enough that PatchValue will choose
// the near-trampoline strategy, i.e. whether this build actually exercises the
// code under test.
func takesShortPath(f interface{}) bool {
	addr := reflect.ValueOf(f).Pointer()
	return inst.TooShortToPatch(common.BytesOf(addr, 64), len(inst.BranchInto(0)))
}

func skipUnlessShort(t *testing.T, f interface{}, name string) {
	if !takesShortPath(f) {
		t.Skipf("%s is not short in this build (e.g. -gcflags=-N pads it); short path not exercised", name)
	}
}

// TestShortFunctionPatchNop is the load-bearing case: an empty function is a
// single RET under every optimisation setting, so this exercises the
// near-trampoline path even under the officially recommended -N -l.
func TestShortFunctionPatchNop(t *testing.T) {
	PatchConvey("a 4-byte function can be mocked", t, func() {
		So(takesShortPath(shortNop), ShouldBeTrue)

		called := false
		Mock(shortNop).To(func() { called = true }).Build()
		shortNop()
		So(called, ShouldBeTrue)
	})
}

func TestShortFunctionPatch(t *testing.T) {
	skipUnlessShort(t, shortIdent, "shortIdent")

	PatchConvey("a function too short for the long jump can still be mocked", t, func() {
		PatchConvey("plain mock", func() {
			Mock(shortIdent).Return(42).Build()
			So(shortIdent(1), ShouldEqual, 42)
		})

		PatchConvey("Origin still reaches the real function", func() {
			var origin func(int) int
			Mock(shortIdent).To(func(i int) int { return origin(i) * 10 }).Origin(&origin).Build()
			So(shortIdent(3), ShouldEqual, 30)
		})
	})
}

// TestShortFunctionPatchRelocatesADRP covers the reason RelocateFirstInst
// exists: the copied entry instruction lands on a different page, so an ADRP
// must have its page offset recomputed or Origin computes a wrong address.
//
// Skipped under -N, where this target is no longer short. That is not a hidden
// coverage gap: under -N the same function is patched by the default path,
// which copies the prologue verbatim and does not relocate ADRP at all. That
// is a separate, pre-existing defect and is out of scope for this change.
func TestShortFunctionPatchRelocatesADRP(t *testing.T) {
	skipUnlessShort(t, shortConstString, "shortConstString")

	PatchConvey("a function whose first instruction is PC-relative (ADRP)", t, func() {
		var origin func() string
		Mock(shortConstString).To(func() string { return "[" + origin() + "]" }).Origin(&origin).Build()
		So(shortConstString(), ShouldEqual, "[original-constant]")
	})
}

func TestShortFunctionUnpatchRestoresBytes(t *testing.T) {
	skipUnlessShort(t, shortAdd, "shortAdd")

	PatchConvey("repeated patch/unpatch leaves the entry byte-identical", t, func() {
		So(shortAdd(2, 3), ShouldEqual, 5)
		for i := 0; i < 50; i++ {
			mocker := Mock(shortAdd).Return(-1).Build()
			So(shortAdd(2, 3), ShouldEqual, -1)
			mocker.UnPatch()
			So(shortAdd(2, 3), ShouldEqual, 5)
		}
	})
}

func TestShortFunctionConcurrent(t *testing.T) {
	skipUnlessShort(t, shortIdent, "shortIdent")

	PatchConvey("a short-patched function is safe under concurrent calls", t, func() {
		Mock(shortIdent).To(func(i int) int { return i + 1 }).Build()

		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 10000; i++ {
					if shortIdent(i) != i+1 {
						panic("short-patched function returned a wrong value")
					}
				}
			}()
		}
		wg.Wait()
	})
}

// TestShortFunctionPatchWhileRunning is the regression test for trampoline
// slots sharing a page. Installing patch N writes into the same page that
// already holds the live trampoline for patch N-1, so that write must use the
// stop-the-world path. Toggling page permissions instead (RW to write, back to
// RX) makes the page briefly non-executable and any thread running an earlier
// trampoline dies with SIGILL or SIGBUS.
func TestShortFunctionPatchWhileRunning(t *testing.T) {
	PatchConvey("patching more short functions is safe while earlier ones run", t, func() {
		Mock(shortNop).To(func() {}).Build()

		stop := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						shortNop()
					}
				}
			}()
		}

		// churn: every Build takes another slot from the same near page the
		// goroutines above are currently executing out of.
		for i := 0; i < 200; i++ {
			m := Mock(shortNop2).To(func() {}).Build()
			shortNop2()
			m.UnPatch()
		}

		close(stop)
		wg.Wait()
	})
}
