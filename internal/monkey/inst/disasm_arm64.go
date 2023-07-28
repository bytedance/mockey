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
	"unsafe"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/arm64/arm64asm"
)

func Disassemble(code []byte, required int, checkLen bool) int {
	tool.Assert(len(code) > required, "function is too short to patch")
	return required
}

func GetGenericJumpAddr(addr uintptr, maxScan uint64) uintptr {
	code := common.BytesOf(addr, int(maxScan))
	var pos uint64
	var err error
	var inst arm64asm.Inst

	for pos < maxScan {
		inst, err = arm64asm.Decode(code[pos:])
		tool.Assert(err == nil, err)
		// if inst.Op == arm64asm.BL {
		args := []interface{}{inst.Op}
		for i := range inst.Args {
			args = append(args, inst.Args[i])
		}
		tool.DebugPrintf("%v\t%v\t%v\t%v\t%v\t%v\n", args...)

		if inst.Op == arm64asm.BL {
			tool.DebugPrintf("found: BL, raw is: %x\n", inst.Enc)
			return calcAddr(uintptr(unsafe.Pointer(&code[0]))+uintptr(pos), inst.Enc)
		}
		pos += uint64(unsafe.Sizeof(inst.Enc))
		tool.Assert(inst.Op != arm64asm.RET, "!!!FOUND RET!!!")
	}
	tool.Assert(false, "BL op not found")
	return 0
}

func calcAddr(from uintptr, bl uint32) uintptr {
	tool.DebugPrintf("calc BL addr, from: %x(%v) bl: %x\n", from, from, bl)
	offset := bl << 8 >> 8
	flag := (offset << 9 >> 9) == offset // 是否小于0

	var dest uintptr
	if flag {
		// L -> H
		// (dest - cur) / 4 = offset
		// dest = cur + offset * 4
		dest = from + uintptr(offset*4)
	} else {
		// H -> L
		// (cur - dest) / 4 = (0x00ffffff - offset + 1)
		// dest = cur -  (0x00ffffff - offset + 1) * 4
		dest = from - uintptr((0x00ffffff-offset+1)*4)
	}
	tool.DebugPrintf("2th complement, L->H:%v offset: %x from: %x(%v) dest: %x(%v), distance: %v\n", flag, offset, from, from, dest, dest, from-dest)
	return dest
}
