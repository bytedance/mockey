/*
 * Copyright 2023 ByteDance Inc.
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
 *
 * Unsafe reflect package for mockey, copy most code from go/src/reflect/type.go,
 * allow to export the address of private member methods.
 */

package unsafereflect

import (
	"reflect"
	"unsafe"
)

func MethodByName(target interface{}, name string) (fn unsafe.Pointer, ok bool) {
	r := castRType(target)
	rt := toRType(r)
	if r.Kind() == reflect.Interface {
		return funcPointer(r.MethodByName(name))
	}

	for _, p := range rt.methods() {
		if rt.nameOff(p.name).name() == name {
			return rt.Method(p), true
		}
	}
	return nil, false
}

// rtype is the common implementation of most values.
// It is embedded in other struct types.
//
// rtype must be kept in sync with src/runtime/type.go:/^type._type.
type rtype struct {
	size       uintptr
	ptrdata    uintptr // number of bytes in the type that can contain pointers
	hash       uint32  // hash of type; avoids computation in hash tables
	tflag      tflag   // extra type information flags
	align      uint8   // alignment of variable with this type
	fieldAlign uint8   // alignment of struct field with this type
	kind       uint8   // enumeration for C
	// function for comparing objects of this type
	// (ptr to object A, ptr to object B) -> ==?
	equal     func(unsafe.Pointer, unsafe.Pointer) bool
	gcdata    *byte   // garbage collection data
	str       nameOff // string form
	ptrToThis typeOff // type for pointer to this type, may be zero
}

func castRType(val interface{}) reflect.Type {
	if rTypeVal, ok := val.(reflect.Type); ok {
		return rTypeVal
	}
	return reflect.TypeOf(val)
}

func toRType(t reflect.Type) *rtype {
	i := *(*funcValue)(unsafe.Pointer(&t))
	r := (*rtype)(i.p)
	return r
}

type funcValue struct {
	_ uintptr
	p unsafe.Pointer
}

func funcPointer(v reflect.Method, ok bool) (unsafe.Pointer, bool) {
	return (*funcValue)(unsafe.Pointer(&v.Func)).p, ok
}

func (t *rtype) Method(p method) (fn unsafe.Pointer) {
	tfn := t.textOff(p.tfn)
	fn = unsafe.Pointer(&tfn)
	return
}

const kindMask = (1 << 5) - 1

func (t *rtype) Kind() reflect.Kind { return reflect.Kind(t.kind & kindMask) }

type tflag uint8
type nameOff int32 // offset to a name
type typeOff int32 // offset to an *rtype
type textOff int32 // offset from top of text section

// resolveNameOff resolves a name offset from a base pointer.
// The (*rtype).nameOff method is a convenience wrapper for this function.
// Implemented in the runtime package.
//
//go:linkname resolveNameOff reflect.resolveNameOff
func resolveNameOff(unsafe.Pointer, int32) unsafe.Pointer

func (t *rtype) nameOff(off nameOff) name {
	return name{(*byte)(resolveNameOff(unsafe.Pointer(t), int32(off)))}
}

// resolveTextOff resolves a function pointer offset from a base type.
// The (*rtype).textOff method is a convenience wrapper for this function.
// Implemented in the runtime package.
//
//go:linkname resolveTextOff reflect.resolveTextOff
func resolveTextOff(unsafe.Pointer, int32) unsafe.Pointer

func (t *rtype) textOff(off textOff) unsafe.Pointer {
	return resolveTextOff(unsafe.Pointer(t), int32(off))
}

const tflagUncommon tflag = 1 << 0

// uncommonType is present only for defined types or types with methods
type uncommonType struct {
	pkgPath nameOff // import path; empty for built-in types like int, string
	mcount  uint16  // number of methods
	xcount  uint16  // number of exported methods
	moff    uint32  // offset from this uncommontype to [mcount]method
	_       uint32  // unused
}

// ptrType represents a pointer type.
type ptrType struct {
	rtype
	elem *rtype // pointer element (pointed at) type
}

// funcType represents a function type.
type funcType struct {
	rtype
	inCount  uint16
	outCount uint16 // top bit is set if last input parameter is ...
}

func (t *funcType) in() []*rtype {
	uadd := unsafe.Sizeof(*t)
	if t.tflag&tflagUncommon != 0 {
		uadd += unsafe.Sizeof(uncommonType{})
	}
	if t.inCount == 0 {
		return nil
	}
	return (*[1 << 20]*rtype)(add(unsafe.Pointer(t), uadd, "t.inCount > 0"))[:t.inCount:t.inCount]
}

func (t *funcType) out() []*rtype {
	uadd := unsafe.Sizeof(*t)
	if t.tflag&tflagUncommon != 0 {
		uadd += unsafe.Sizeof(uncommonType{})
	}
	outCount := t.outCount & (1<<15 - 1)
	if outCount == 0 {
		return nil
	}
	return (*[1 << 20]*rtype)(add(unsafe.Pointer(t), uadd, "outCount > 0"))[t.inCount : t.inCount+outCount : t.inCount+outCount]
}

func (t *rtype) IsVariadic() bool {
	tt := (*funcType)(unsafe.Pointer(t))
	return tt.outCount&(1<<15) != 0
}

func add(p unsafe.Pointer, x uintptr, whySafe string) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + x)
}

// interfaceType represents an interface type.
type interfaceType struct {
	rtype
	pkgPath name      // import path
	methods []imethod // sorted by hash
}

type imethod struct {
	name nameOff // name of method
	typ  typeOff // .(*FuncType) underneath
}

func (t *rtype) methods() []method {
	if t.tflag&tflagUncommon == 0 {
		return nil
	}
	switch t.Kind() {
	case reflect.Ptr:
		type u struct {
			ptrType
			u uncommonType
		}
		return (*u)(unsafe.Pointer(t)).u.methods()
	case reflect.Func:
		type u struct {
			funcType
			u uncommonType
		}
		return (*u)(unsafe.Pointer(t)).u.methods()
	case reflect.Interface:
		type u struct {
			interfaceType
			u uncommonType
		}
		return (*u)(unsafe.Pointer(t)).u.methods()
	case reflect.Struct:
		type u struct {
			structType
			u uncommonType
		}
		return (*u)(unsafe.Pointer(t)).u.methods()
	default:
		return nil
	}
}

// Method on non-interface type
type method struct {
	name nameOff // name of method
	mtyp typeOff // method type (without receiver), not valid for private methods
	ifn  textOff // fn used in interface call (one-word receiver)
	tfn  textOff // fn used for normal method call
}

func (t *uncommonType) methods() []method {
	if t.mcount == 0 {
		return nil
	}
	return (*[1 << 16]method)(add(unsafe.Pointer(t), uintptr(t.moff), "t.mcount > 0"))[:t.mcount:t.mcount]
}

// Struct field
type structField struct {
	name   name    // name is always non-empty
	typ    *rtype  // type of field
	offset uintptr // byte offset of field
}

// structType
type structType struct {
	rtype
	pkgPath name
	fields  []structField // sorted by offset
}

type _String struct {
	Data unsafe.Pointer
	Len  int
}
