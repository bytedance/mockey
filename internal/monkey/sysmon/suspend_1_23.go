//go:build !mockey_disable_ss && go1.23 && !go1.28
// +build !mockey_disable_ss,go1.23,!go1.28

/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to support Go 1.27 and the Windows sleep ABI.
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

package sysmon

import (
	"reflect"
	"runtime"
	"unsafe"

	"github.com/bytedance/mockey/internal/monkey/fn"
	"github.com/bytedance/mockey/internal/monkey/linkname"
)

func init() {
	usleepPC := linkname.FuncPCForName("runtime.usleep")
	if runtime.GOOS == "windows" {
		// Windows implements usleep as ordinary Go code with register arguments.
		// The stack-argument trampoline can otherwise pass an arbitrary delay,
		// particularly when race instrumentation changes the argument registers.
		usleep = fn.MakeFunc(reflect.TypeOf(usleep), usleepPC).Interface().(func(uint32))
	} else {
		usleep = func(usec uint32) { usleepTrampoline(usec, usleepPC) }
	}
	lockPC := linkname.FuncPCForName("runtime.lock")
	lock = fn.MakeFunc(reflect.TypeOf(lock), lockPC).Interface().(func(unsafe.Pointer))
	unlockPC := linkname.FuncPCForName("runtime.unlock")
	unlock = fn.MakeFunc(reflect.TypeOf(unlock), unlockPC).Interface().(func(unsafe.Pointer))
}

// usleepTrampoline calls the stack-argument runtime.usleep used outside Windows.
// This includes assembly implementations and functions marked go:cgo_unsafe_args.
// Windows uses the ordinary Go register ABI and is bound directly above.
func usleepTrampoline(usec uint32, pc uintptr)
