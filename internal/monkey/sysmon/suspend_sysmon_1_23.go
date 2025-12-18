//go:build go1.23
// +build go1.23

package sysmon

import (
	"reflect"
	"unsafe"

	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/linkname"
)

func init() {
	lockPC := linkname.FuncPCForName("runtime.lock")
	lock = fn.MakeFunc(reflect.TypeOf(lock), lockPC).Interface().(func(unsafe.Pointer))
	unlockPC := linkname.FuncPCForName("runtime.unlock")
	unlock = fn.MakeFunc(reflect.TypeOf(unlock), unlockPC).Interface().(func(unsafe.Pointer))
}
