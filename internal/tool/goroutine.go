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
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

// The `goid` field offset in the struct g in the runtime package
var gGoroutineIDOffset uintptr

func init() {
	gGoroutineIDOffset = 152 // default offset for Go1.22-

	vcode := strings.Split(strings.TrimPrefix(runtime.Version(), "go"), ".")
	if vcode[0] != "1" || len(vcode) < 2 {
		DebugPrintf("invalid go version: %s", runtime.Version())
		return
	}
	subv, err := strconv.ParseInt(vcode[1], 10, 64)
	if err != nil {
		DebugPrintf("invalid go version: %s", runtime.Version())
		return
	}
	if subv >= 23 && subv < 25 { // Go1.23+
		gGoroutineIDOffset = 160 // Go1.23 introduced the `syscallbp` field before goid
	} else if subv >= 25 { // Go1.25+
		gGoroutineIDOffset = 152 // Go1.25 removed the `gobuf.ret` field before goid
	}
}

func GetGoroutineID() int64 {
	g := getG()
	p := (*int64)(unsafe.Pointer(uintptr(g) + gGoroutineIDOffset))
	return *p
}

func getG() unsafe.Pointer
