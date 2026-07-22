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
	"errors"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/monkey/mem"
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

func LifecycleTarget(in string) string {
	return strings.Repeat(in, 1)
}

func catchPanic(f func()) (recovered interface{}) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}

func catchPanicState(f func()) (panicked bool, recovered interface{}) {
	panicked = true
	defer func() {
		recovered = recover()
	}()
	f()
	panicked = false
	return
}

func cleanupPoisonedTarget(t *testing.T, target uintptr) {
	t.Helper()

	patchRegistry.Lock()
	failed, ok := patchRegistry.failedByTarget[target]
	if !ok {
		patchRegistry.Unlock()
		t.Fatalf("target %#x is not quarantined", target)
		return
	}
	patch := failed.patch
	mem.WriteWithSTW(patch.base, patch.original[:patch.size])
	patch.proxy.Elem().Set(patch.previousProxy)
	inst.ReleaseProxy(patch.code)
	delete(patchRegistry.failedByTarget, target)
	stack := patchRegistry.byTarget[target]
	if len(stack) > 0 && stack[len(stack)-1] == patch {
		if len(stack) == 1 {
			delete(patchRegistry.byTarget, target)
		} else {
			patchRegistry.byTarget[target] = stack[:len(stack)-1]
		}
	}
	patch.code = nil
	patch.installed = nil
	patch.hook = reflect.Value{}
	patch.proxy = reflect.Value{}
	patch.previousProxy = reflect.Value{}
	patchRegistry.Unlock()
}

func TestPatchInstallFailureRestoresProxy(t *testing.T) {
	previousProxy := func(in string) string { return "previous:" + in }
	proxy := previousProxy
	injectedErr := errors.New("injected install failure")
	releases := 0

	recovered := catchPanic(func() {
		patchValue(
			reflect.ValueOf(LifecycleTarget),
			reflect.ValueOf(Hook),
			reflect.ValueOf(&proxy),
			false,
			func(uintptr, []byte, []byte) (mem.ReplaceState, error) {
				return mem.ReplaceRestored, injectedErr
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})

	if recovered == nil {
		t.Fatal("patchValue did not panic")
	}
	if releases != 1 {
		t.Fatalf("proxy releases = %d, want 1", releases)
	}
	if got := proxy("value"); got != "previous:value" {
		t.Fatalf("proxy was not restored: got %q", got)
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("target changed after restored failure: got %q", got)
	}
	target := reflect.ValueOf(LifecycleTarget).Pointer()
	patchRegistry.Lock()
	_, registered := patchRegistry.byTarget[target]
	_, poisoned := patchRegistry.failedByTarget[target]
	patchRegistry.Unlock()
	if registered || poisoned {
		t.Fatalf("restored failure left registry state: registered=%v poisoned=%v", registered, poisoned)
	}
}

func TestPatchInstallUncertainQuarantinesReferences(t *testing.T) {
	var proxy func(string) string
	injectedErr := errors.New("injected uncertain install")
	releases := 0
	target := reflect.ValueOf(LifecycleTarget).Pointer()
	cleaned := false
	defer func() {
		if !cleaned {
			cleanupPoisonedTarget(t, target)
		}
	}()

	recovered := catchPanic(func() {
		patchValue(
			reflect.ValueOf(LifecycleTarget),
			reflect.ValueOf(Hook),
			reflect.ValueOf(&proxy),
			false,
			func(uintptr, []byte, []byte) (mem.ReplaceState, error) {
				return mem.ReplaceUncertain, injectedErr
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})

	if recovered == nil {
		t.Fatal("patchValue did not panic")
	}
	if releases != 0 {
		t.Fatalf("uncertain proxy releases = %d, want 0", releases)
	}
	patchRegistry.Lock()
	failed := patchRegistry.failedByTarget[target]
	patchRegistry.Unlock()
	if failed == nil || !failed.patch.hook.IsValid() || !failed.patch.proxy.IsValid() {
		t.Fatal("uncertain patch did not retain hook and proxy")
	}

	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	if proxy == nil {
		t.Fatal("uncertain patch did not retain the injected proxy")
	}
	if got := proxy("value"); got != "value" {
		t.Fatalf("retained proxy returned %q, want original value", got)
	}
	var secondProxy func(string) string
	if catchPanic(func() {
		PatchFunc(LifecycleTarget, Hook, &secondProxy, false)
	}) == nil {
		t.Fatal("poisoned target accepted another patch")
	}

	cleanupPoisonedTarget(t, target)
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore the previous nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("cleanup did not restore target: got %q", got)
	}
}

func TestUnpatchFailureKeepsPatchRetryable(t *testing.T) {
	var proxy func(string) string
	patch := PatchFunc(LifecycleTarget, Hook, &proxy, false)
	injectedErr := errors.New("injected restore failure")
	releases := 0

	recovered := catchPanic(func() {
		patch.unpatch(
			func(uintptr, []byte, []byte) (mem.ReplaceState, error) {
				return mem.ReplaceRestored, injectedErr
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})
	if recovered == nil {
		t.Fatal("Unpatch did not panic")
	}
	if releases != 0 {
		t.Fatalf("proxy releases = %d, want 0", releases)
	}
	if got := LifecycleTarget("value"); got != "MOCKED!" {
		t.Fatalf("patch was not kept active: got %q", got)
	}
	if proxy == nil {
		t.Fatal("proxy was cleared after retryable failure")
	}

	patch.Unpatch()
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("retry did not restore target: got %q", got)
	}
	if proxy != nil {
		t.Fatal("successful retry did not restore nil proxy")
	}
}

func TestUnpatchUncertainPoisonsTarget(t *testing.T) {
	var proxy func(string) string
	patch := PatchFunc(LifecycleTarget, Hook, &proxy, false)
	target := patch.Base()
	injectedErr := errors.New("injected uncertain restore")
	cleaned := false
	defer func() {
		if !cleaned {
			cleanupPoisonedTarget(t, target)
		}
	}()

	if catchPanic(func() {
		patch.unpatch(
			func(uintptr, []byte, []byte) (mem.ReplaceState, error) {
				return mem.ReplaceUncertain, injectedErr
			},
			inst.ReleaseProxy,
		)
	}) == nil {
		t.Fatal("uncertain Unpatch did not panic")
	}
	if got := LifecycleTarget("value"); got != "MOCKED!" {
		t.Fatalf("uncertain Unpatch unexpectedly disabled patch: got %q", got)
	}
	if catchPanic(patch.Unpatch) == nil {
		t.Fatal("poisoned target allowed another Unpatch")
	}

	cleanupPoisonedTarget(t, target)
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("cleanup did not restore target: got %q", got)
	}
}

func TestPatchLayersRequireLIFO(t *testing.T) {
	var firstProxy, secondProxy func(string) string
	first := PatchFunc(
		LifecycleTarget,
		func(string) string { return "first" },
		&firstProxy,
		false,
	)
	second := PatchFunc(
		LifecycleTarget,
		func(string) string { return "second" },
		&secondProxy,
		false,
	)
	firstActive, secondActive := true, true
	defer func() {
		if secondActive {
			second.Unpatch()
		}
		if firstActive {
			first.Unpatch()
		}
	}()

	if got := LifecycleTarget("value"); got != "second" {
		t.Fatalf("top patch returned %q, want second", got)
	}
	if catchPanic(first.Unpatch) == nil {
		t.Fatal("non-LIFO Unpatch did not panic")
	}
	if got := LifecycleTarget("value"); got != "second" {
		t.Fatalf("non-LIFO attempt changed target: got %q", got)
	}

	second.Unpatch()
	secondActive = false
	if got := LifecycleTarget("value"); got != "first" {
		t.Fatalf("removing top patch returned %q, want first", got)
	}

	first.Unpatch()
	firstActive = false
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("removing all patches returned %q, want original", got)
	}
}

func installClosurePatch() (*Patch, *func(string) string) {
	proxy := new(func(string) string)
	prefix := "retained:"
	patch := PatchFunc(
		LifecycleTarget,
		func(in string) string { return prefix + in },
		proxy,
		false,
	)
	return patch, proxy
}

func TestPatchKeepsClosureHookAlive(t *testing.T) {
	patch, proxy := installClosurePatch()
	active := true
	defer func() {
		if active {
			patch.Unpatch()
		}
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
	}
	if got := LifecycleTarget("value"); got != "retained:value" {
		t.Fatalf("hook after GC returned %q", got)
	}
	if *proxy == nil || (*proxy)("value") != "value" {
		t.Fatal("origin proxy was not kept alive")
	}

	patch.Unpatch()
	active = false
	if *proxy != nil {
		t.Fatal("Unpatch did not restore the previous nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("Unpatch returned %q, want original", got)
	}
}

func TestPatchInstallPanicQuarantinesMutatedTarget(t *testing.T) {
	var proxy func(string) string
	panicValue := &struct{ operation string }{operation: "install"}
	releases := 0
	target := reflect.ValueOf(LifecycleTarget).Pointer()
	cleaned := false
	defer func() {
		if !cleaned {
			cleanupPoisonedTarget(t, target)
		}
	}()

	recovered := catchPanic(func() {
		patchValue(
			reflect.ValueOf(LifecycleTarget),
			reflect.ValueOf(func(in string) string { return "panic:" + in }),
			reflect.ValueOf(&proxy),
			false,
			func(target uintptr, replacement, _ []byte) (mem.ReplaceState, error) {
				mem.WriteWithSTW(target, replacement)
				panic(panicValue)
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})

	if recovered != panicValue {
		t.Fatalf("panic = %#v, want original value %#v", recovered, panicValue)
	}
	if releases != 0 {
		t.Fatalf("proxy releases = %d, want 0", releases)
	}
	patchRegistry.Lock()
	failed := patchRegistry.failedByTarget[target]
	patchRegistry.Unlock()
	if failed == nil {
		t.Fatal("panic after target mutation was not quarantined")
	}
	if failed.cause != panicValue {
		t.Fatalf("quarantine cause = %#v, want %#v", failed.cause, panicValue)
	}
	if !failed.patch.hook.IsValid() || !failed.patch.proxy.IsValid() {
		t.Fatal("quarantine did not retain hook and proxy")
	}

	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	if got := LifecycleTarget("value"); got != "panic:value" {
		t.Fatalf("quarantined hook after GC returned %q", got)
	}
	if proxy == nil || proxy("value") != "value" {
		t.Fatal("quarantined origin proxy was not executable")
	}
	var secondProxy func(string) string
	if catchPanic(func() {
		PatchFunc(LifecycleTarget, Hook, &secondProxy, false)
	}) == nil {
		t.Fatal("quarantined target accepted another patch")
	}

	cleanupPoisonedTarget(t, target)
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("cleanup returned %q, want original", got)
	}
}

func TestUnpatchPanicQuarantinesMutatedTarget(t *testing.T) {
	var proxy func(string) string
	patch := PatchFunc(
		LifecycleTarget,
		func(in string) string { return "patched:" + in },
		&proxy,
		false,
	)
	target := patch.Base()
	panicValue := &struct{ operation string }{operation: "unpatch"}
	releases := 0
	cleaned := false
	defer func() {
		if !cleaned {
			cleanupPoisonedTarget(t, target)
		}
	}()

	recovered := catchPanic(func() {
		patch.unpatch(
			func(target uintptr, original, _ []byte) (mem.ReplaceState, error) {
				mem.WriteWithSTW(target, original)
				panic(panicValue)
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})

	if recovered != panicValue {
		t.Fatalf("panic = %#v, want original value %#v", recovered, panicValue)
	}
	if releases != 0 {
		t.Fatalf("proxy releases = %d, want 0", releases)
	}
	patchRegistry.Lock()
	failed := patchRegistry.failedByTarget[target]
	stack := patchRegistry.byTarget[target]
	patchRegistry.Unlock()
	if failed == nil || failed.patch != patch {
		t.Fatal("panic during Unpatch was not quarantined")
	}
	if failed.cause != panicValue {
		t.Fatalf("quarantine cause = %#v, want %#v", failed.cause, panicValue)
	}
	if len(stack) == 0 || stack[len(stack)-1] != patch {
		t.Fatal("panic during Unpatch popped the active patch")
	}

	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("fake Unpatch did not mutate target: got %q", got)
	}
	if proxy == nil || proxy("value") != "value" {
		t.Fatal("panic during Unpatch released or reset proxy")
	}
	if catchPanic(patch.Unpatch) == nil {
		t.Fatal("quarantined patch allowed another Unpatch")
	}
	var secondProxy func(string) string
	if catchPanic(func() {
		PatchFunc(LifecycleTarget, Hook, &secondProxy, false)
	}) == nil {
		t.Fatal("quarantined target accepted another patch")
	}

	cleanupPoisonedTarget(t, target)
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("cleanup returned %q, want original", got)
	}
}

const panicNilChildEnv = "MOCKEY_PANIC_NIL_CHILD"

func targetIsQuarantined(target uintptr) bool {
	patchRegistry.Lock()
	defer patchRegistry.Unlock()
	_, quarantined := patchRegistry.failedByTarget[target]
	return quarantined
}

func TestReplaceTargetNilPanicIsQuarantined(t *testing.T) {
	if os.Getenv(panicNilChildEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestReplaceTargetNilPanicIsQuarantined$")
		environment := make([]string, 0, len(os.Environ())+2)
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, "GODEBUG=") ||
				strings.HasPrefix(entry, panicNilChildEnv+"=") {
				continue
			}
			environment = append(environment, entry)
		}
		cmd.Env = append(environment, "GODEBUG=panicnil=1", panicNilChildEnv+"=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("panic(nil) child process failed: %v\n%s", err, output)
		}
		return
	}

	panicked, recovered := catchPanicState(func() {
		panic(nil)
	})
	if !panicked || recovered != nil {
		t.Fatalf("GODEBUG=panicnil=1 is not active: panicked=%v recovered=%#v", panicked, recovered)
	}

	t.Run("install", testPatchInstallNilPanicQuarantines)
	t.Run("unpatch", testUnpatchNilPanicQuarantines)
}

func testPatchInstallNilPanicQuarantines(t *testing.T) {
	var proxy func(string) string
	var installed *Patch
	target := reflect.ValueOf(LifecycleTarget).Pointer()
	releases := 0
	cleaned := false
	defer func() {
		if cleaned {
			return
		}
		if targetIsQuarantined(target) {
			cleanupPoisonedTarget(t, target)
		} else if installed != nil {
			installed.Unpatch()
		}
	}()

	panicked, recovered := catchPanicState(func() {
		installed = patchValue(
			reflect.ValueOf(LifecycleTarget),
			reflect.ValueOf(func(in string) string { return "nil-panic:" + in }),
			reflect.ValueOf(&proxy),
			false,
			func(target uintptr, replacement, _ []byte) (mem.ReplaceState, error) {
				mem.WriteWithSTW(target, replacement)
				panic(nil)
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
	})

	if !panicked {
		t.Fatal("panic(nil) during patch installation was treated as ReplaceApplied")
	}
	if recovered != nil {
		t.Fatalf("recovered panic value = %#v, want nil", recovered)
	}
	if installed != nil {
		t.Fatal("patch installation returned a patch after panic(nil)")
	}
	if releases != 0 {
		t.Fatalf("proxy releases = %d, want 0", releases)
	}

	patchRegistry.Lock()
	failed := patchRegistry.failedByTarget[target]
	_, registered := patchRegistry.byTarget[target]
	patchRegistry.Unlock()
	if failed == nil {
		t.Fatal("panic(nil) during patch installation was not quarantined")
	}
	if failed.cause != nil {
		t.Fatalf("quarantine cause = %#v, want nil", failed.cause)
	}
	if registered {
		t.Fatal("panicking patch installation was registered as applied")
	}
	if !failed.patch.hook.IsValid() || !failed.patch.proxy.IsValid() {
		t.Fatal("quarantine did not retain the hook and proxy")
	}
	if got := LifecycleTarget("value"); got != "nil-panic:value" {
		t.Fatalf("mutated target returned %q, want quarantined hook", got)
	}
	if proxy == nil || proxy("value") != "value" {
		t.Fatal("quarantined origin proxy is not executable")
	}

	var secondProxy func(string) string
	secondPanicked, _ := catchPanicState(func() {
		PatchFunc(LifecycleTarget, Hook, &secondProxy, false)
	})
	if !secondPanicked {
		t.Fatal("quarantined target accepted another patch")
	}

	cleanupPoisonedTarget(t, target)
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore the nil proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("cleanup returned %q, want original", got)
	}
}

func testUnpatchNilPanicQuarantines(t *testing.T) {
	var proxy func(string) string
	patch := PatchFunc(
		LifecycleTarget,
		func(in string) string { return "patched:" + in },
		&proxy,
		false,
	)
	target := patch.Base()
	releases := 0
	active := true
	cleaned := false
	defer func() {
		if cleaned {
			return
		}
		if targetIsQuarantined(target) {
			cleanupPoisonedTarget(t, target)
		} else if active {
			patch.Unpatch()
		}
	}()

	panicked, recovered := catchPanicState(func() {
		patch.unpatch(
			func(target uintptr, replacement, _ []byte) (mem.ReplaceState, error) {
				mem.WriteWithSTW(target, replacement)
				panic(nil)
			},
			func(code []byte) {
				releases++
				inst.ReleaseProxy(code)
			},
		)
		active = false
	})

	if !panicked {
		t.Fatal("panic(nil) during Unpatch was treated as ReplaceApplied")
	}
	if recovered != nil {
		t.Fatalf("recovered panic value = %#v, want nil", recovered)
	}
	if releases != 0 {
		t.Fatalf("proxy releases = %d, want 0", releases)
	}

	patchRegistry.Lock()
	failed := patchRegistry.failedByTarget[target]
	stack := patchRegistry.byTarget[target]
	patchRegistry.Unlock()
	if failed == nil || failed.patch != patch {
		t.Fatal("panic(nil) during Unpatch was not quarantined")
	}
	if failed.cause != nil {
		t.Fatalf("quarantine cause = %#v, want nil", failed.cause)
	}
	if len(stack) == 0 || stack[len(stack)-1] != patch {
		t.Fatal("panic(nil) during Unpatch popped the active patch")
	}
	if proxy == nil || proxy("value") != "value" {
		t.Fatal("panic(nil) during Unpatch released or reset the proxy")
	}
	if got := LifecycleTarget("value"); got != "value" {
		t.Fatalf("fake Unpatch returned %q, want original", got)
	}

	secondPanicked, _ := catchPanicState(patch.Unpatch)
	if !secondPanicked {
		t.Fatal("quarantined patch allowed another Unpatch")
	}

	cleanupPoisonedTarget(t, target)
	active = false
	cleaned = true
	if proxy != nil {
		t.Fatal("cleanup did not restore the nil proxy")
	}
}
