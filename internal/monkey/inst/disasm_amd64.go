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
	"github.com/bytedance/mockey/internal/tool"
	"golang.org/x/arch/x86/x86asm"
)

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
