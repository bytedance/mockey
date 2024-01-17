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
	"golang.org/x/arch/x86/x86asm"
)

func calcFnAddrRange(name string, fn func()) (uintptr, uintptr) {
	v := reflect.ValueOf(fn)
	var start, end uintptr
	start = v.Pointer()
	maxScan := 2000
	code := common.BytesOf(v.Pointer(), 2000)
	pos := 0

	for pos < maxScan {
		inst, err := x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)

		args := []interface{}{name, inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("init: <%v>\t%v\t%v\t%v\t%v\t%v\t%v\n", args...)

		if inst.Op == x86asm.RET {
			end = start + uintptr(pos)
			tool.DebugPrintf("init: %v(%v,%v)\n", name, start, end)
			return start, end
		}

		pos += int(inst.Len)
	}
	tool.Assert(false, "%v ret not found", name)
	return 0, 0
}

func Disassemble(code []byte, required int, checkLen bool) int {
	var pos int
	var err error
	var inst x86asm.Inst

	for pos < required {
		inst, err = x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)
		tool.DebugPrintf("Disassemble: inst: %v\n", inst)
		tool.Assert(inst.Op != x86asm.RET || !checkLen, "function is too short to patch")
		pos += inst.Len
	}
	return pos
}

func GetGenericJumpAddr(addr uintptr, maxScan uint64) uintptr {
	code := common.BytesOf(addr, int(maxScan))
	var pos uint64
	var err error
	var inst x86asm.Inst

	allAddrs := []uintptr{}
	for pos < maxScan {
		inst, err = x86asm.Decode(code[pos:], 64)
		tool.Assert(err == nil, err)

		args := []interface{}{inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("%v\t%v\t%v\t%v\t%v\t%v\n", args...)

		if inst.Op == x86asm.RET {
			break
		}

		if inst.Op == x86asm.CALL {
			rel := int32(inst.Args[0].(x86asm.Rel))
			fnAddr := calcAddr(uintptr(unsafe.Pointer(&code[0]))+uintptr(pos+uint64(inst.Len)), rel)
			isExtraCall, extraName := isGenericProxyCallExtra(fnAddr)
			tool.DebugPrintf("found CALL, raw is: %x, rel: %v,  raw is: %x,  fnAddr: %v, isExtraCall: %v, extraName: %v\n", inst.String(), rel, fnAddr, isExtraCall, extraName)
			if !isExtraCall {
				allAddrs = append(allAddrs, fnAddr)
			}
		}
		pos += uint64(inst.Len)
	}
	tool.Assert(len(allAddrs) == 1, "invalid callAddr: %v", allAddrs)
	return allAddrs[0]
}

func calcAddr(from uintptr, rel int32) uintptr {
	tool.DebugPrintf("calc CALL addr, from: %x(%v) CALL: %x\n", from, from, rel)

	var dest uintptr
	if rel < 0 {
		dest = from - uintptr(uint32(-rel))
	} else {
		dest = from + uintptr(rel)
	}

	tool.DebugPrintf("L->H:%v rel: %v from: %x(%v) dest: %x(%v), distance: %v\n", rel > 0, rel, from, from, dest, dest, from-dest)
	return dest
}
