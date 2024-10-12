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

package inst

import (
	"reflect"
	"unsafe"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/arm64/arm64asm"
)

func calcFnAddrRange(name string, fn func()) (uintptr, uintptr) {
	v := reflect.ValueOf(fn)
	var start, end uintptr
	start = v.Pointer()
	maxScan := 2000
	code := common.BytesOf(start, 2000)
	pos := 0
	for pos < maxScan {
		inst, err := arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)

		args := []interface{}{name, inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}

		if inst.Op == arm64asm.RET {
			end = start + uintptr(pos)
			return start, end
		}

		pos += int(unsafe.Sizeof(inst.Enc))
	}
	tool.Assert(false, "%v end not found", name)
	return 0, 0
}

func Disassemble(code []byte, required int, checkLen bool) int {
	tool.Assert(len(code) > required, "function is too short to patch")
	return required
}

func GetGenericJumpAddr(addr uintptr, maxScan uint64) uintptr {
	code := common.BytesOf(addr, int(maxScan))
	var pos uint64
	var err error
	var inst arm64asm.Inst

	allAddrs := []uintptr{}
	for pos < maxScan {
		inst, err = arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)
		args := []interface{}{inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("%v\t%v\t%v\t%v\t%v\t%v\n", args...)

		if inst.Op == arm64asm.RET {
			break
		}

		if inst.Op == arm64asm.BL {
			fnAddr := calcAddr(uintptr(unsafe.Pointer(&code[0]))+uintptr(pos), inst)
			isExtraCall, extraName := isGenericProxyCallExtra(fnAddr)
			tool.DebugPrintf("found BL, raw is: %x,  fnAddr: %v, isExtraCall: %v, extraName: %v\n", inst.String(), fnAddr, isExtraCall, extraName)
			if !isExtraCall {
				allAddrs = append(allAddrs, fnAddr)
			}
		}
		pos += uint64(unsafe.Sizeof(inst.Enc))
	}
	tool.Assert(len(allAddrs) == 1, "invalid callAddr: %v", allAddrs)
	return allAddrs[0]
}

func calcAddr(from uintptr, bl arm64asm.Inst) uintptr {
	distance := int64(bl.Args[0].(arm64asm.PCRel))
	if distance < 0 {
		return from - uintptr(-distance)
	} else {
		return from + uintptr(distance)
	}
}
