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

package tool

import (
	"reflect"
	"unsafe"
)

// Why this file exists
//
// A mock hook is a reflect.MakeFunc. When the patched target is called,
// reflect.callReflect copies the call's arguments into a heap-allocated
// []reflect.Value, and every later use of them (When, the hook, the origin)
// goes through reflect.Value.Call.
//
// The caller, however, was compiled against the *target's* escape summary. If
// the target does not let a pointer argument escape (the typical case is a
// callback `f func(...)` that is only invoked), the caller is free to keep the
// pointee -- the closure, and everything the closure captured -- in its own
// stack frame. That is invisible until the goroutine stack moves (it grows, or
// the GC shrinks it): copystack fixes every pointer that lives on the stack,
// but nothing that lives on the heap, so the []reflect.Value keeps pointing at
// the old, freed stack. The hook then runs against a stale copy of the caller's
// data. Nothing crashes; writes simply land in memory nobody reads again.
//
// StackGuard closes that gap from the hook side: it records which stack the
// arguments were captured on and, right before each use, moves any argument
// word that still points into that old stack to the same offset in the current
// one -- the exact adjustment copystack applies to on-stack pointers.

// gStack mirrors runtime.stack, the first field of runtime.g in every Go
// release.
type gStack struct {
	lo, hi uintptr
}

func currentStack(g unsafe.Pointer) gStack {
	return *(*gStack)(g)
}

// stackReserve is the headroom made available before every reflect call that
// consumes guarded arguments. It must cover reflect.Value.Call up to the point
// where the arguments are copied into registers or the callee frame, which is
// on the order of a few KiB; the margin keeps that window free of growth.
const stackReserve = 32 << 10

// StackGuard tracks the stack a set of hook arguments was captured on.
type StackGuard struct {
	g      unsafe.Pointer
	lo, hi uintptr
}

// NewStackGuard snapshots the current goroutine's stack. Call it first thing in
// a reflect.MakeFunc body, before anything that could grow the stack.
func NewStackGuard() *StackGuard {
	g := getG()
	s := currentStack(g)
	return &StackGuard{g: g, lo: s.lo, hi: s.hi}
}

// Fix makes args safe to hand to a reflect call: it first reserves stack
// headroom (so the call setup itself cannot move the stack again), then
// relocates every argument word that points into the stack the arguments were
// captured on. Values are replaced in place; untouched values are kept as is.
func (sg *StackGuard) Fix(args []reflect.Value) {
	if sg == nil {
		return
	}
	if getG() == sg.g {
		ReserveStack()
	}
	cur := currentStack(sg.g)
	if cur.hi == sg.hi {
		return
	}
	delta := cur.hi - sg.hi
	shift := func(w uintptr) uintptr { return w + delta }
	for i := range args {
		walkValue(&args[i], sg.lo, sg.hi, shift)
	}
	DebugPrintf("StackGuard: stack moved [0x%x,0x%x) -> [0x%x,0x%x), relocated hook arguments\n", sg.lo, sg.hi, cur.lo, cur.hi)
	sg.lo, sg.hi = cur.lo, cur.hi
}

// Release is called when the hook returns. The []reflect.Value can outlive the
// call -- an Origin getter keeps it reachable -- and every word in it that
// points into this goroutine's stack would become a dangling pointer once the
// goroutine exits and its stack is freed; the GC aborts with "found bad pointer
// in Go heap" when it later scans such a word. Clear those words: nothing reads
// the arguments after the hook has returned.
func (sg *StackGuard) Release(args []reflect.Value) {
	if sg == nil {
		return
	}
	sg.Fix(args)
	cur := currentStack(sg.g)
	drop := func(uintptr) uintptr { return 0 }
	for i := range args {
		walkValue(&args[i], cur.lo, cur.hi, drop)
	}
}

// ReserveStack guarantees at least stackReserve bytes of free stack below the
// caller, growing the stack now if needed.
//
//go:noinline
func ReserveStack() {
	var probe byte
	sp := uintptr(unsafe.Pointer(&probe))
	if s := currentStack(getG()); sp-s.lo > stackReserve {
		return
	}
	growStack()
}

//go:noinline
func growStack() byte {
	var buf [stackReserve]byte
	return touchStack(buf[:])
}

//go:noinline
func touchStack(b []byte) byte {
	return b[len(b)-1]
}

// valueHeader mirrors reflect.Value: type, data pointer, flag.
type valueHeader struct {
	typ  unsafe.Pointer
	ptr  unsafe.Pointer
	flag uintptr
}

// flagIndir is reflect's flagIndir: ptr points at the value instead of being
// the value. Its bit position has not changed since reflect gained it.
const flagIndir uintptr = 1 << 7

// walkValue rewrites, in place, every word of *v that points into [lo, hi)
// to op(word). It never copies the value: a copy would push the stale words
// through write barriers (and into a fresh heap object the GC will scan),
// and a stale word may point into a stack span that has already been
// returned to the heap.
//
// In place is safe for the values mockey relocates: hook arguments and
// results are the only references to the memory reflect allocated for them.
func walkValue(v *reflect.Value, lo, hi uintptr, op func(uintptr) uintptr) {
	if !v.IsValid() || !mayHoldPointer(v.Type()) {
		return
	}
	h := (*valueHeader)(unsafe.Pointer(v))
	if h.flag&flagIndir == 0 {
		// Pointer-shaped kinds are stored directly in the header.
		rewriteWord(unsafe.Pointer(&h.ptr), lo, hi, op)
		return
	}
	walkMem(h.ptr, v.Type(), lo, hi, op)
}

func mayHoldPointer(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Map, reflect.Chan, reflect.Func,
		reflect.String, reflect.Slice, reflect.Interface:
		return true
	case reflect.Array:
		return t.Len() > 0 && mayHoldPointer(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if mayHoldPointer(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

// walkMem walks the memory of one value of type t at p (not following
// pointers: pointees on the stack are moved, and fixed, by copystack itself)
// and reports whether any word was changed.
func walkMem(p unsafe.Pointer, t reflect.Type, lo, hi uintptr, op func(uintptr) uintptr) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Map, reflect.Chan, reflect.Func,
		reflect.String, reflect.Slice:
		// The data pointer is the first word of string and slice headers.
		return rewriteWord(p, lo, hi, op)
	case reflect.Interface:
		// Type/itab word first, data word second.
		return rewriteWord(unsafe.Pointer(uintptr(p)+unsafe.Sizeof(uintptr(0))), lo, hi, op)
	case reflect.Array:
		changed := false
		if !mayHoldPointer(t.Elem()) {
			return false
		}
		for i := 0; i < t.Len(); i++ {
			if walkMem(unsafe.Pointer(uintptr(p)+uintptr(i)*t.Elem().Size()), t.Elem(), lo, hi, op) {
				changed = true
			}
		}
		return changed
	case reflect.Struct:
		changed := false
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if walkMem(unsafe.Pointer(uintptr(p)+f.Offset), f.Type, lo, hi, op) {
				changed = true
			}
		}
		return changed
	}
	return false
}

// rewriteWord replaces one pointer word that points into [lo, hi) by op(word).
//
// The word is stored as a uintptr on purpose. The old value points into a
// goroutine stack, and the new one either does too or is nil; the GC never
// reaches stacks through heap pointers (stacks are scanned as roots), so no
// write barrier is needed -- and must not run, since the old stack may already
// have been handed back to the heap. A uintptr store also keeps -race/checkptr
// from rejecting a pointer it cannot attribute to an allocation.
func rewriteWord(p unsafe.Pointer, lo, hi uintptr, op func(uintptr) uintptr) bool {
	w := *(*uintptr)(p)
	if w < lo || w >= hi {
		return false
	}
	*(*uintptr)(p) = op(w)
	return true
}
