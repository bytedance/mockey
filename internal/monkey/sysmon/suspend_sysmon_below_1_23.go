//go:build !mockey_disable_ss && !go1.23
// +build !mockey_disable_ss,!go1.23

package sysmon

import (
	"unsafe"
)

func init() {
	lock = lock0
	unlock = unlock0
}

//go:linkname lock0 runtime.lock
func lock0(unsafe.Pointer)

//go:linkname unlock0 runtime.unlock
func unlock0(unsafe.Pointer)
