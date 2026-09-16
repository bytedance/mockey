//go:build linux && riscv64
// +build linux,riscv64

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
	"golang.org/x/arch/riscv64/riscv64asm"
)

// extendTMPSequence prevents the proxy's tail jump from clobbering X31 in the
// middle of a multi-instruction expansion emitted by the Go assembler.
func extendTMPSequence(code []byte, limit int) int {
	active := false
	maxEnd := limit
	var scan reachableCodeScan
	for pos := 0; ; {
		tool.Assert(pos < len(code), "RISC-V TMP sequence exceeds patch scan buffer")
		instruction, err := riscv64asm.Decode(code[pos:])
		tool.Assert(err == nil, "decode TMP sequence at +%d: %v", pos, err)
		scan.observe(pos, instruction, code[pos:])

		writesTMP := instructionWritesTMP(instruction)
		readsTMP := instructionReadsTMP(instruction, writesTMP)
		switch {
		case writesTMP:
			active = true
		case active && readsTMP:
			active = false
		}

		end := pos + instruction.Len
		maxEnd = max(maxEnd, end)
		if active {
			_, conditional := conditionalBranchTarget(pos, instruction)
			tool.Assert(
				!conditional && instruction.Op != riscv64asm.JAL && instruction.Op != riscv64asm.JALR,
				"RISC-V TMP sequence crosses control flow at +%d",
				pos,
			)
			pos = end
			continue
		}

		nextPos, stop := scan.advance(pos, instruction, limit)
		if stop {
			return maxEnd
		}
		pos = nextPos
		if pos >= limit {
			return max(maxEnd, pos)
		}
	}
}

func instructionWritesTMP(instruction riscv64asm.Inst) bool {
	if !firstRegisterIsTMP(instruction) {
		return false
	}
	switch instruction.Op {
	case riscv64asm.BEQ, riscv64asm.BNE, riscv64asm.BLT, riscv64asm.BGE,
		riscv64asm.BLTU, riscv64asm.BGEU,
		riscv64asm.SB, riscv64asm.SH, riscv64asm.SW, riscv64asm.SD,
		riscv64asm.FSW, riscv64asm.FSD:
		return false
	default:
		return true
	}
}

func instructionReadsTMP(instruction riscv64asm.Inst, firstIsDestination bool) bool {
	for index, argument := range instruction.Args {
		if firstIsDestination && index == 0 {
			continue
		}
		switch value := argument.(type) {
		case riscv64asm.Reg:
			if value == riscv64asm.X31 {
				return true
			}
		case riscv64asm.RegOffset:
			if value.OfsReg == riscv64asm.X31 {
				return true
			}
		}
	}
	return false
}

func firstRegisterIsTMP(instruction riscv64asm.Inst) bool {
	register, ok := instruction.Args[0].(riscv64asm.Reg)
	return ok && register == riscv64asm.X31
}
