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

package common

import (
	"reflect"
	"unsafe"
)

func PtrOf(val []byte) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(val)))
}

func PtrAt(val reflect.Value) uintptr {
	type value struct {
		_   uintptr
		ptr unsafe.Pointer
	}
	return uintptr((*value)(unsafe.Pointer(&val)).ptr)
}

func BytesOf(addr uintptr, size int) []byte {
	return unsafe.Slice((*byte)(pointerOf(addr)), size)
}

// pointerOf loads a raw virtual address through a pointer-typed view. Callers
// use BytesOf only for executable code or mmap-backed memory, whose addresses
// intentionally do not carry Go pointer provenance.
func pointerOf(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}
