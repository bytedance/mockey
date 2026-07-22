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
	"strings"
	"testing"

	"github.com/bytedance/mockey/internal/tool"
	"github.com/smartystreets/goconvey/convey"
)

//go:noinline
func Fun0() string {
	ok1 := Fun1()
	fmt.Printf("Fun0: call Fun1, %v\n", ok1)
	if !ok1 {
		return "exit"
	}
	ok2 := Fun2()
	fmt.Printf("Fun0: call Fun2, %v\n", ok2)
	if ok2 {
		return "fun2"
	}
	ok3 := Fun3()
	fmt.Printf("Fun0: call Fun3, %v\n", ok3)
	if ok3 {
		return "fun3"
	}
	return "xxx"
}

//go:noinline
func Fun1() bool {
	fmt.Println("Fun1")
	return false
}

//go:noinline
func Fun2() bool {
	fmt.Println("Fun2")
	return false
}

//go:noinline
func Fun3() bool {
	fmt.Println("Fun3")
	return false
}

func TestPatchConvey(t *testing.T) {
	PatchConvey("test", t, func() {
		Mock(Fun1).Return(true).Build()

		PatchConvey("test case 2", func() {
			m2 := Mock(Fun2).Return(true).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			convey.So(r, convey.ShouldEqual, "fun2")
			convey.So(m2.Times(), convey.ShouldEqual, 1)
			convey.So(m3.Times(), convey.ShouldEqual, 0)
		})

		PatchConvey("test case 3", func() {
			m2 := Mock(Fun2).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			convey.So(r, convey.ShouldEqual, "fun3")
			convey.So(m2.Times(), convey.ShouldEqual, 1)
			convey.So(m3.Times(), convey.ShouldEqual, 1)
		})

		PatchConvey("test case mock times", func() {
			m2 := Mock(Fun2).When(func() bool { return false }).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			convey.So(r, convey.ShouldEqual, "fun3")
			convey.So(m2.Times(), convey.ShouldEqual, 1)
			convey.So(m2.MockTimes(), convey.ShouldEqual, 0)
			convey.So(m3.Times(), convey.ShouldEqual, 1)
		})
	})
}

func TestUnpatchAll_Convey(t *testing.T) {
	fn1 := func() string {
		return "fn1"
	}
	fn2 := func() string {
		return "fn2"
	}
	fn3 := func() string {
		return "fn3"
	}

	Mock(fn1).Return("mocked").Build()
	if fn1() != "mocked" {
		t.Error("mock fn1 failed")
	}

	PatchConvey("UnpatchAll_Convey", t, func() {
		Mock(fn2).Return("mocked").Build()
		Mock(fn3).Return("mocked").Build()
		convey.So(fn1(), convey.ShouldEqual, "mocked")
		convey.So(fn2(), convey.ShouldEqual, "mocked")
		convey.So(fn3(), convey.ShouldEqual, "mocked")

		UnPatchAll()

		convey.So(fn1(), convey.ShouldEqual, "mocked")
		convey.So(fn2(), convey.ShouldEqual, "fn2")
		convey.So(fn3(), convey.ShouldEqual, "fn3")
	})

	r1, r2, r3 := fn1(), fn2(), fn3()
	if r1 != "mocked" || r2 != "fn2" || r3 != "fn3" {
		t.Error("mock failed", r1, r2, r3)
	}

	UnPatchAll()

	r1, r2, r3 = fn1(), fn2(), fn3()
	if r1 != "fn1" || r2 != "fn2" || r3 != "fn3" {
		t.Error("mock failed", r1, r2, r3)
	}
}

func TestPatchRun(t *testing.T) {
	PatchRun(func() {
		Mock(Fun1).Return(true).Build()

		PatchRun(func() {
			m2 := Mock(Fun2).Return(true).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			if r != "fun2" {
				t.Errorf("expected 'fun2', got '%s'", r)
			}
			if m2.Times() != 1 {
				t.Errorf("expected m2.Times() == 1, got %d", m2.Times())
			}
			if m3.Times() != 0 {
				t.Errorf("expected m3.Times() == 0, got %d", m3.Times())
			}
		})

		PatchRun(func() {
			m2 := Mock(Fun2).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			if r != "fun3" {
				t.Errorf("expected 'fun3', got '%s'", r)
			}
			if m2.Times() != 1 {
				t.Errorf("expected m2.Times() == 1, got %d", m2.Times())
			}
			if m3.Times() != 1 {
				t.Errorf("expected m3.Times() == 1, got %d", m3.Times())
			}
		})

		PatchRun(func() {
			m2 := Mock(Fun2).When(func() bool { return false }).Build()
			m3 := Mock(Fun3).Return(true).Build()

			r := Fun0()
			if r != "fun3" {
				t.Errorf("expected 'fun3', got '%s'", r)
			}
			if m2.Times() != 1 {
				t.Errorf("expected m2.Times() == 1, got %d", m2.Times())
			}
			if m2.MockTimes() != 0 {
				t.Errorf("expected m2.MockTimes() == 0, got %d", m2.MockTimes())
			}
			if m3.Times() != 1 {
				t.Errorf("expected m3.Times() == 1, got %d", m3.Times())
			}
		})
	})
}

// TestUnpatchAll_PatchRun tests UnpatchAll functionality within PatchRun
func TestUnpatchAll_PatchRun(t *testing.T) {
	fn1 := func() string {
		return "fn1"
	}
	fn2 := func() string {
		return "fn2"
	}
	fn3 := func() string {
		return "fn3"
	}

	// Mock outside PatchRun
	Mock(fn1).Return("mocked").Build()
	if fn1() != "mocked" {
		t.Error("mock fn1 failed outside PatchRun")
	}

	PatchRun(func() {
		// Mock inside PatchRun
		Mock(fn2).Return("mocked").Build()
		Mock(fn3).Return("mocked").Build()

		// All should be mocked
		if fn1() != "mocked" {
			t.Error("fn1 should be mocked inside PatchRun")
		}
		if fn2() != "mocked" {
			t.Error("fn2 should be mocked inside PatchRun")
		}
		if fn3() != "mocked" {
			t.Error("fn3 should be mocked inside PatchRun")
		}

		// UnpatchAll should only remove mocks from current PatchRun
		UnPatchAll()

		// fn1 should still be mocked (from outside), fn2 and fn3 should be restored
		if fn1() != "mocked" {
			t.Error("fn1 should still be mocked after UnPatchAll in PatchRun")
		}
		if fn2() != "fn2" {
			t.Error("fn2 should be restored after UnPatchAll in PatchRun")
		}
		if fn3() != "fn3" {
			t.Error("fn3 should be restored after UnPatchAll in PatchRun")
		}
	})

	// After PatchRun, fn1 should still be mocked, fn2 and fn3 should be original
	r1, r2, r3 := fn1(), fn2(), fn3()
	if r1 != "mocked" || r2 != "fn2" || r3 != "fn3" {
		t.Errorf("mock state incorrect after PatchRun: fn1=%q, fn2=%q, fn3=%q", r1, r2, r3)
	}

	// Clean up the remaining mock
	UnPatchAll()

	// All should be original now
	r1, r2, r3 = fn1(), fn2(), fn3()
	if r1 != "fn1" || r2 != "fn2" || r3 != "fn3" {
		t.Errorf("mock state incorrect after final UnPatchAll: fn1=%q, fn2=%q, fn3=%q", r1, r2, r3)
	}
}

type lifecycleOrderMocker struct {
	id    int
	order *[]int
}

func (mocker *lifecycleOrderMocker) identityKey() uintptr {
	return uintptr(mocker.id)
}

func (mocker *lifecycleOrderMocker) layerKey() uintptr {
	return mocker.identityKey()
}

func (mocker *lifecycleOrderMocker) name() string {
	return fmt.Sprintf("lifecycle mocker %d", mocker.id)
}

func (mocker *lifecycleOrderMocker) unPatchLocked() {
	*mocker.order = append(*mocker.order, mocker.id)
	removeFromGlobal(mocker)
}

func (mocker *lifecycleOrderMocker) caller() tool.CallerInfo {
	return tool.CallerInfo{}
}

func TestPatchRunCleanupUsesLIFOOrder(t *testing.T) {
	var order []int
	PatchRun(func() {
		mockLifecycleMu.Lock()
		defer mockLifecycleMu.Unlock()

		addToGlobal(&lifecycleOrderMocker{id: 1, order: &order})
		addToGlobal(&lifecycleOrderMocker{id: 2, order: &order})
		addToGlobal(&lifecycleOrderMocker{id: 3, order: &order})
	})

	if got := fmt.Sprint(order); got != "[3 2 1]" {
		t.Fatalf("cleanup order = %s, want [3 2 1]", got)
	}
}

//go:noinline
func concurrentBuildTarget() int {
	return 0
}

func TestConcurrentBuildHasSingleWinner(t *testing.T) {
	const workers = 8
	builders := make([]*MockBuilder, workers)
	for i := range builders {
		builders[i] = Mock(concurrentBuildTarget).Return(i + 1)
	}

	PatchRun(func() {
		start := make(chan struct{})
		results := make(chan int, workers)
		for i, builder := range builders {
			go func(value int, builder *MockBuilder) {
				<-start
				defer func() {
					if recover() != nil {
						results <- 0
					}
				}()
				builder.Build()
				results <- value
			}(i+1, builder)
		}

		close(start)
		winner := 0
		successCount := 0
		for range workers {
			if value := <-results; value != 0 {
				successCount++
				winner = value
			}
		}
		if successCount != 1 {
			t.Fatalf("successful builds = %d, want 1", successCount)
		}
		if got := concurrentBuildTarget(); got != winner {
			t.Fatalf("patched target = %d, want winner %d", got, winner)
		}
		UnPatchAll()
		if got := concurrentBuildTarget(); got != 0 {
			t.Fatalf("target after cleanup = %d, want 0", got)
		}
	})
}

func TestConcurrentRootBuildHasSingleWinner(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	const workers = 8
	builders := make([]*MockBuilder, workers)
	for i := range builders {
		builders[i] = Mock(concurrentBuildTarget).Return(i + 1)
	}

	start := make(chan struct{})
	results := make(chan int, workers)
	for i, builder := range builders {
		go func(value int, builder *MockBuilder) {
			<-start
			defer func() {
				if recover() != nil {
					results <- 0
				}
			}()
			builder.Build()
			results <- value
		}(i+1, builder)
	}

	close(start)
	winner := 0
	successCount := 0
	for range workers {
		if value := <-results; value != 0 {
			successCount++
			winner = value
		}
	}
	if successCount != 1 {
		t.Fatalf("successful root-scope builds = %d, want 1", successCount)
	}
	if got := concurrentBuildTarget(); got != winner {
		t.Fatalf("patched root-scope target = %d, want winner %d", got, winner)
	}
	UnPatchAll()
	if got := concurrentBuildTarget(); got != 0 {
		t.Fatalf("root-scope target after cleanup = %d, want 0", got)
	}
}

//go:noinline
func layeredProxyTarget(value int) int {
	return value + 1
}

func TestLayeredFunctionProxyChain(t *testing.T) {
	var firstOrigin func(int) int

	PatchRun(func() {
		first := Mock(layeredProxyTarget).
			To(func(value int) int {
				return firstOrigin(value) + 10
			}).
			Origin(&firstOrigin).
			Build()
		if got := layeredProxyTarget(1); got != 12 {
			t.Fatalf("first layer result = %d, want 12", got)
		}

		PatchRun(func() {
			var secondOrigin func(int) int
			second := Mock(layeredProxyTarget).
				To(func(value int) int {
					return secondOrigin(value) * 2
				}).
				Origin(&secondOrigin).
				Build()
			if got := layeredProxyTarget(1); got != 24 {
				t.Fatalf("second layer result = %d, want 24", got)
			}
			second.UnPatch()
			if got := layeredProxyTarget(1); got != 12 {
				t.Fatalf("result after second unpatch = %d, want 12", got)
			}
		})
		if got := layeredProxyTarget(1); got != 12 {
			t.Fatalf("result after inner cleanup = %d, want 12", got)
		}
		first.UnPatch()
		if got := layeredProxyTarget(1); got != 2 {
			t.Fatalf("result after first unpatch = %d, want 2", got)
		}
	})
	if got := layeredProxyTarget(1); got != 2 {
		t.Fatalf("result after PatchRun = %d, want 2", got)
	}
}

func runBlockedPatchScope(target *int, mocked int, entered chan<- struct{}, release <-chan struct{}, done chan<- interface{}) {
	var recovered interface{}
	func() {
		defer func() { recovered = recover() }()
		PatchRun(func() {
			MockValue(target).To(mocked)
			close(entered)
			<-release
		})
	}()
	done <- recovered
}

func TestInterleavedPatchRunScopesKeepOwnership(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	first := 1
	second := 2
	root := 3

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan interface{}, 1)
	go runBlockedPatchScope(&first, 10, firstEntered, releaseFirst, firstDone)
	<-firstEntered

	secondEntered := make(chan struct{})
	releaseSecond := make(chan struct{})
	secondDone := make(chan interface{}, 1)
	go runBlockedPatchScope(&second, 20, secondEntered, releaseSecond, secondDone)
	<-secondEntered

	MockValue(&root).To(30)

	close(releaseFirst)
	firstPanic := <-firstDone
	firstAfterExit := first
	secondDuringFirstExit := second
	rootDuringScopeCleanup := root
	close(releaseSecond)
	secondPanic := <-secondDone
	secondAfterExit := second

	if firstPanic != nil {
		t.Fatalf("first scope cleanup panicked: %v", firstPanic)
	}
	if secondPanic != nil {
		t.Fatalf("second scope cleanup panicked: %v", secondPanic)
	}
	if firstAfterExit != 1 {
		t.Fatalf("first value after its scope exit = %d, want 1", firstAfterExit)
	}
	if secondDuringFirstExit != 20 {
		t.Fatalf("second value during first scope exit = %d, want 20", secondDuringFirstExit)
	}
	if rootDuringScopeCleanup != 30 {
		t.Fatalf("ambiguous root value during scope cleanup = %d, want 30", rootDuringScopeCleanup)
	}
	if secondAfterExit != 2 {
		t.Fatalf("second value after its scope exit = %d, want 2", secondAfterExit)
	}

	UnPatchAll()
	if root != 3 {
		t.Fatalf("root value after root cleanup = %d, want 3", root)
	}
}

func TestNestedScopeChildJoinsInnermostOwnerScope(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	value := 1
	duringInner := 0
	afterInner := 0
	var childPanic interface{}

	PatchRun(func() {
		PatchRun(func() {
			done := make(chan interface{}, 1)
			go func() {
				defer func() { done <- recover() }()
				MockValue(&value).To(2)
			}()
			childPanic = <-done
			duringInner = value
		})
		afterInner = value
	})

	if childPanic != nil {
		t.Fatalf("child mock panicked: %v", childPanic)
	}
	if duringInner != 2 {
		t.Fatalf("value during inner scope = %d, want 2", duringInner)
	}
	if afterInner != 1 {
		t.Fatalf("value after inner scope = %d, want 1", afterInner)
	}
	if value != 1 {
		t.Fatalf("value after outer scope = %d, want 1", value)
	}
}

func TestUnPatchAllWithoutLocalScopeOnlyCleansRoot(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	value := 1
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan interface{}, 1)
	go runBlockedPatchScope(&value, 2, entered, release, done)
	<-entered

	UnPatchAll()
	afterUnPatchAll := value
	close(release)
	scopePanic := <-done

	if scopePanic != nil {
		t.Fatalf("scope cleanup panicked: %v", scopePanic)
	}
	if afterUnPatchAll != 2 {
		t.Fatalf("foreign scope value after UnPatchAll = %d, want 2", afterUnPatchAll)
	}
	if value != 1 {
		t.Fatalf("value after scope exit = %d, want 1", value)
	}
}

func TestRootUnPatchAllSkipsShadowedMocks(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	safe := 10
	shadowed := 1
	MockValue(&safe).To(20)
	MockValue(&shadowed).To(2)

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan interface{}, 1)
	go runBlockedPatchScope(&shadowed, 3, entered, release, done)
	<-entered

	var cleanupPanic interface{}
	func() {
		defer func() { cleanupPanic = recover() }()
		UnPatchAll()
	}()
	safeAfterCleanup := safe
	shadowedDuringChild := shadowed

	close(release)
	childPanic := <-done
	shadowedAfterChild := shadowed

	UnPatchAll()
	shadowedAfterRootCleanup := shadowed

	if cleanupPanic != nil {
		t.Fatalf("root cleanup panicked on a shadowed mock: %v", cleanupPanic)
	}
	if safeAfterCleanup != 10 {
		t.Fatalf("safe root value after cleanup = %d, want 10", safeAfterCleanup)
	}
	if shadowedDuringChild != 3 {
		t.Fatalf("shadowed value during child layer = %d, want 3", shadowedDuringChild)
	}
	if childPanic != nil {
		t.Fatalf("child scope cleanup panicked: %v", childPanic)
	}
	if shadowedAfterChild != 2 {
		t.Fatalf("shadowed value after child cleanup = %d, want root layer 2", shadowedAfterChild)
	}
	if shadowedAfterRootCleanup != 1 {
		t.Fatalf("shadowed value after later root cleanup = %d, want 1", shadowedAfterRootCleanup)
	}
	if safe != 10 {
		t.Fatalf("safe value after final cleanup = %d, want 10", safe)
	}
}

type lifecycleGenericMap[K comparable, V any] struct {
	values map[K]V
}

func (m *lifecycleGenericMap[K, V]) Get(key K) V {
	return m.values[key]
}

func TestSameOwnerGenericGCShapeLayers(t *testing.T) {
	intResult := 11
	stringResult := "mock"
	firstBuilder := Mock((*lifecycleGenericMap[int32, *int]).Get).Return(&intResult)
	secondBuilder := Mock((*lifecycleGenericMap[int32, *string]).Get).Return(&stringResult)
	firstPatchKey := firstBuilder.analyzer.RuntimeTargetValue().Pointer()
	secondPatchKey := secondBuilder.analyzer.RuntimeTargetValue().Pointer()
	if firstPatchKey != secondPatchKey {
		t.Fatalf("generic fixtures do not share a gcshape target: 0x%x != 0x%x", firstPatchKey, secondPatchKey)
	}

	PatchRun(func() {
		first := firstBuilder.Build()
		second := secondBuilder.Build()
		if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != &intResult {
			t.Fatalf("first generic layer result = %p, want %p", got, &intResult)
		}
		if got := new(lifecycleGenericMap[int32, *string]).Get(1); got != &stringResult {
			t.Fatalf("second generic layer result = %p, want %p", got, &stringResult)
		}

		second.UnPatch()
		if got := new(lifecycleGenericMap[int32, *string]).Get(1); got != nil {
			t.Fatalf("second generic result after unpatch = %p, want nil", got)
		}
		if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != &intResult {
			t.Fatalf("first generic layer after second unpatch = %p, want %p", got, &intResult)
		}

		first.UnPatch()
		if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != nil {
			t.Fatalf("first generic result after unpatch = %p, want nil", got)
		}
	})
}

func TestSameRootOwnerGenericGCShapeLayers(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	intResult := 11
	stringResult := "mock"
	firstBuilder := Mock((*lifecycleGenericMap[int32, *int]).Get).Return(&intResult)
	secondBuilder := Mock((*lifecycleGenericMap[int32, *string]).Get).Return(&stringResult)
	if firstBuilder.analyzer.RuntimeTargetValue().Pointer() != secondBuilder.analyzer.RuntimeTargetValue().Pointer() {
		t.Fatal("generic fixtures do not share a gcshape target")
	}

	first := firstBuilder.Build()
	second := secondBuilder.Build()
	if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != &intResult {
		t.Fatalf("first root generic layer result = %p, want %p", got, &intResult)
	}
	if got := new(lifecycleGenericMap[int32, *string]).Get(1); got != &stringResult {
		t.Fatalf("second root generic layer result = %p, want %p", got, &stringResult)
	}

	second.UnPatch()
	if got := new(lifecycleGenericMap[int32, *string]).Get(1); got != nil {
		t.Fatalf("second root generic result after unpatch = %p, want nil", got)
	}
	if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != &intResult {
		t.Fatalf("first root generic layer after second unpatch = %p, want %p", got, &intResult)
	}

	first.UnPatch()
	if got := new(lifecycleGenericMap[int32, *int]).Get(1); got != nil {
		t.Fatalf("first root generic result after unpatch = %p, want nil", got)
	}
}

func TestConcurrentRootOwnersRejectSharedGenericGCShape(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	intResult := 11
	stringResult := "mock"
	originResult := "origin"
	secondOrigin := func(*lifecycleGenericMap[int32, *string], int32) *string {
		return &originResult
	}
	firstBuilder := Mock((*lifecycleGenericMap[int32, *int]).Get).Return(&intResult)
	secondBuilder := Mock((*lifecycleGenericMap[int32, *string]).Get).Return(&stringResult).Origin(&secondOrigin)
	if firstBuilder.analyzer.RuntimeTargetValue().Pointer() != secondBuilder.analyzer.RuntimeTargetValue().Pointer() {
		t.Fatal("generic fixtures do not share a gcshape target")
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan interface{}, 1)
	go func() {
		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			first := firstBuilder.Build()
			close(entered)
			<-release
			first.UnPatch()
		}()
		firstDone <- recovered
	}()
	<-entered

	rejected := make(chan interface{}, 1)
	go func() {
		defer func() { rejected <- recover() }()
		secondBuilder.Build()
	}()
	patchPanic := <-rejected
	firstDuringOwner := new(lifecycleGenericMap[int32, *int]).Get(1)
	secondDuringOwner := new(lifecycleGenericMap[int32, *string]).Get(1)
	originAfterRejected := secondOrigin(nil, 0)

	close(release)
	firstOwnerPanic := <-firstDone
	firstAfterCleanup := new(lifecycleGenericMap[int32, *int]).Get(1)
	secondAfterCleanup := new(lifecycleGenericMap[int32, *string]).Get(1)

	if patchPanic == nil {
		t.Fatal("cross-owner root same-gcshape generic patch did not panic")
	}
	if !strings.Contains(fmt.Sprint(patchPanic), "concurrent mock scope owners") {
		t.Fatalf("cross-owner root generic panic = %v, want owner rejection", patchPanic)
	}
	if firstDuringOwner != &intResult {
		t.Fatalf("active root generic layer after rejection = %p, want %p", firstDuringOwner, &intResult)
	}
	if secondDuringOwner != nil {
		t.Fatalf("rejected root generic instance was mutated: result = %p, want nil", secondDuringOwner)
	}
	if originAfterRejected != &originResult {
		t.Fatalf("rejected root generic build changed Origin: result = %p, want %p", originAfterRejected, &originResult)
	}
	if firstOwnerPanic != nil {
		t.Fatalf("first root generic owner cleanup panicked: %v", firstOwnerPanic)
	}
	if firstAfterCleanup != nil {
		t.Fatalf("first root generic result after cleanup = %p, want nil", firstAfterCleanup)
	}
	if secondAfterCleanup != nil {
		t.Fatalf("second root generic result after cleanup = %p, want nil", secondAfterCleanup)
	}
}

func TestConcurrentOwnersRejectSharedGenericGCShape(t *testing.T) {
	intResult := 11
	stringResult := "mock"
	firstBuilder := Mock((*lifecycleGenericMap[int32, *int]).Get).Return(&intResult)
	secondBuilder := Mock((*lifecycleGenericMap[int32, *string]).Get).Return(&stringResult)
	if firstBuilder.analyzer.RuntimeTargetValue().Pointer() != secondBuilder.analyzer.RuntimeTargetValue().Pointer() {
		t.Fatal("generic fixtures do not share a gcshape target")
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan interface{}, 1)
	go func() {
		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			PatchRun(func() {
				firstBuilder.Build()
				close(entered)
				<-release
			})
		}()
		firstDone <- recovered
	}()
	<-entered

	rejected := make(chan interface{}, 1)
	go func() {
		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			PatchRun(func() {
				secondBuilder.Build()
			})
		}()
		rejected <- recovered
	}()
	patchPanic := <-rejected
	firstDuringOwner := new(lifecycleGenericMap[int32, *int]).Get(1)
	secondDuringOwner := new(lifecycleGenericMap[int32, *string]).Get(1)

	close(release)
	firstOwnerPanic := <-firstDone
	firstAfterCleanup := new(lifecycleGenericMap[int32, *int]).Get(1)
	secondAfterCleanup := new(lifecycleGenericMap[int32, *string]).Get(1)

	if patchPanic == nil {
		t.Fatal("cross-owner same-gcshape generic patch did not panic")
	}
	if !strings.Contains(fmt.Sprint(patchPanic), "concurrent mock scope owners") {
		t.Fatalf("cross-owner generic panic = %v, want owner rejection", patchPanic)
	}
	if firstDuringOwner != &intResult {
		t.Fatalf("active generic layer after rejection = %p, want %p", firstDuringOwner, &intResult)
	}
	if secondDuringOwner != nil {
		t.Fatalf("rejected generic instance was mutated: result = %p, want nil", secondDuringOwner)
	}
	if firstOwnerPanic != nil {
		t.Fatalf("first generic owner cleanup panicked: %v", firstOwnerPanic)
	}
	if firstAfterCleanup != nil {
		t.Fatalf("first generic result after cleanup = %p, want nil", firstAfterCleanup)
	}
	if secondAfterCleanup != nil {
		t.Fatalf("second generic result after cleanup = %p, want nil", secondAfterCleanup)
	}
}
