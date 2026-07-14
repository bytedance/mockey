//go:build riscv64
// +build riscv64

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
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

const proxyPointerStackChild = "MOCKEY_PROXY_POINTER_STACK_CHILD"

//go:noinline
func originMorestackTarget(value int) int {
	var padding [128 << 10]byte
	padding[0] = byte(value)
	padding[len(padding)-1] = byte(value >> 8)
	runtime.KeepAlive(&padding)
	return value + 1 + int(padding[0]) + int(padding[len(padding)-1])
}

func TestProxyRelocatesMorestackCall(t *testing.T) {
	var proxy func(int) int
	patch := PatchFunc(
		originMorestackTarget,
		func(int) int { return -1 },
		&proxy,
		false,
	)
	defer patch.Unpatch()

	input := 0x123
	result := make(chan int, 1)
	go func() {
		result <- proxy(input)
	}()

	select {
	case got := <-result:
		want := input + 1 + int(byte(input)) + int(byte(input>>8))
		if got != want {
			t.Fatalf("origin proxy result: got %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("origin proxy did not return after stack growth")
	}
}

// hideStackPointer hides pointer provenance from escape analysis so the
// regression exercises a stack pointer.
//
//go:noescape
func hideStackPointer(pointer unsafe.Pointer) unsafe.Pointer

//go:noinline
func originMorestackPointerTarget(pointer *int) uintptr {
	var padding [128 << 10]byte
	padding[0] = byte(*pointer)
	padding[len(padding)-1] = byte(*pointer)
	runtime.KeepAlive(&padding)
	return uintptr(unsafe.Pointer(pointer))
}

func TestProxyMorestackAdjustsPointerArgument(t *testing.T) {
	if os.Getenv(proxyPointerStackChild) != "1" {
		cmd := exec.Command(
			os.Args[0],
			"-test.run=^TestProxyMorestackAdjustsPointerArgument$",
		)
		cmd.Env = append(os.Environ(), proxyPointerStackChild+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("pointer stack-growth child failed: %v\n%s", err, output)
		}
		return
	}

	var proxy func(*int) uintptr
	patch := PatchFunc(
		originMorestackPointerTarget,
		func(*int) uintptr { return 0 },
		&proxy,
		false,
	)
	defer patch.Unpatch()

	type result struct {
		before uintptr
		got    uintptr
		after  uintptr
	}
	results := make(chan result, 1)
	go func() {
		value := 123
		before := uintptr(unsafe.Pointer(&value))
		pointer := (*int)(hideStackPointer(unsafe.Pointer(&value)))
		got := proxy(pointer)
		results <- result{
			before: before,
			got:    got,
			after:  uintptr(unsafe.Pointer(&value)),
		}
	}()

	select {
	case got := <-results:
		if got.before == got.after {
			t.Fatal("test did not move the goroutine stack")
		}
		if got.got != got.after {
			t.Fatalf(
				"pointer argument was not adjusted: before=%#x got=%#x after=%#x",
				got.before,
				got.got,
				got.after,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("origin proxy did not return after pointer-bearing stack growth")
	}
}
