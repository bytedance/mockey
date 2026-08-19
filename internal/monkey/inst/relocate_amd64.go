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
	"encoding/binary"
	"fmt"

	"golang.org/x/arch/x86/x86asm"
)

type decodedInst struct {
	oldOffset int
	newOffset int
	newLen    int
	inst      x86asm.Inst
}

func HasPCRelative(code []byte) bool {
	for pos := 0; pos < len(code); {
		decoded, err := x86asm.Decode(code[pos:], 64)
		if err != nil || decoded.Len == 0 || pos+decoded.Len > len(code) {
			return true
		}
		if decoded.PCRel != 0 {
			return true
		}
		pos += decoded.Len
	}
	return false
}

func Relocate(code []byte, oldAddr, newAddr uintptr) ([]byte, error) {
	decoded, newSize, err := decodeForRelocation(code)
	if err != nil {
		return nil, err
	}
	offsets := make(map[int]int, len(decoded))
	for _, item := range decoded {
		offsets[item.oldOffset] = item.newOffset
	}

	result := make([]byte, newSize)
	for _, item := range decoded {
		oldBytes := code[item.oldOffset : item.oldOffset+item.inst.Len]
		newBytes := result[item.newOffset : item.newOffset+item.newLen]
		if item.inst.PCRel == 0 {
			copy(newBytes, oldBytes)
			continue
		}

		displacement, err := readSigned(oldBytes[item.inst.PCRelOff:], item.inst.PCRel)
		if err != nil {
			return nil, err
		}
		oldNext := oldAddr + uintptr(item.oldOffset+item.inst.Len)
		target, ok := addSigned(oldNext, displacement)
		if !ok {
			return nil, fmt.Errorf("pc-relative target overflows address space")
		}

		isControl := hasRelativeControl(item.inst)
		if isControl && target >= oldAddr && target < oldAddr+uintptr(len(code)) {
			mapped, exists := offsets[int(target-oldAddr)]
			if !exists {
				return nil, fmt.Errorf("relative branch targets the middle of a copied instruction")
			}
			target = newAddr + uintptr(mapped)
		}

		newNext := newAddr + uintptr(item.newOffset+item.newLen)
		newDisplacement, ok := subtractAddress(target, newNext)
		if !ok {
			return nil, fmt.Errorf("relocated target is outside the supported address range")
		}

		if item.inst.PCRel == 1 && isControl {
			opcodeOffset := item.inst.PCRelOff - 1
			if opcodeOffset < 0 {
				return nil, fmt.Errorf("unsupported rel8 instruction %s", item.inst.Op)
			}
			switch oldBytes[opcodeOffset] {
			case 0xeb:
				if opcodeOffset != 0 {
					return nil, fmt.Errorf("unsupported prefixed short jump %s", item.inst.Op)
				}
				newBytes[0] = 0xe9
				if !writeSigned(newBytes[1:], 4, newDisplacement) {
					return nil, fmt.Errorf("relocated short jump is outside rel32 range")
				}
			default:
				if oldBytes[opcodeOffset] < 0x70 || oldBytes[opcodeOffset] > 0x7f {
					return nil, fmt.Errorf("unsupported rel8 instruction %s", item.inst.Op)
				}
				copy(newBytes, oldBytes[:opcodeOffset])
				newBytes[opcodeOffset] = 0x0f
				newBytes[opcodeOffset+1] = 0x80 | (oldBytes[opcodeOffset] & 0x0f)
				if !writeSigned(newBytes[opcodeOffset+2:], 4, newDisplacement) {
					return nil, fmt.Errorf("relocated short conditional jump is outside rel32 range")
				}
			}
			continue
		}

		copy(newBytes, oldBytes)
		if !writeSigned(newBytes[item.inst.PCRelOff:], item.inst.PCRel, newDisplacement) {
			return nil, fmt.Errorf("relocated %s is outside rel%d range", item.inst.Op, item.inst.PCRel*8)
		}
	}
	return result, nil
}

func decodeForRelocation(code []byte) ([]decodedInst, int, error) {
	var result []decodedInst
	oldOffset, newOffset := 0, 0
	for oldOffset < len(code) {
		decoded, err := x86asm.Decode(code[oldOffset:], 64)
		if err != nil || decoded.Len == 0 || oldOffset+decoded.Len > len(code) {
			return nil, 0, fmt.Errorf("decode instruction at offset %d: %v", oldOffset, err)
		}
		newLen := decoded.Len
		if decoded.PCRel == 1 && hasRelativeControl(decoded) {
			encoded := code[oldOffset : oldOffset+decoded.Len]
			opcodeOffset := decoded.PCRelOff - 1
			if opcodeOffset < 0 {
				return nil, 0, fmt.Errorf("unsupported rel8 instruction %s", decoded.Op)
			}
			switch {
			case opcodeOffset == 0 && encoded[opcodeOffset] == 0xeb:
				newLen = 5
			case encoded[opcodeOffset] >= 0x70 && encoded[opcodeOffset] <= 0x7f:
				newLen = decoded.Len + 4
			default:
				return nil, 0, fmt.Errorf("unsupported rel8 instruction %s", decoded.Op)
			}
		}
		result = append(result, decodedInst{
			oldOffset: oldOffset,
			newOffset: newOffset,
			newLen:    newLen,
			inst:      decoded,
		})
		oldOffset += decoded.Len
		newOffset += newLen
	}
	return result, newOffset, nil
}

func hasRelativeControl(inst x86asm.Inst) bool {
	for _, arg := range inst.Args {
		if _, ok := arg.(x86asm.Rel); ok {
			return true
		}
	}
	return false
}

func readSigned(data []byte, size int) (int64, error) {
	if len(data) < size {
		return 0, fmt.Errorf("pc-relative displacement is truncated")
	}
	switch size {
	case 1:
		return int64(int8(data[0])), nil
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	default:
		return 0, fmt.Errorf("unsupported pc-relative displacement width %d", size)
	}
}

func writeSigned(data []byte, size int, value int64) bool {
	if len(data) < size {
		return false
	}
	switch size {
	case 1:
		if value < -1<<7 || value > 1<<7-1 {
			return false
		}
		data[0] = byte(int8(value))
	case 2:
		if value < -1<<15 || value > 1<<15-1 {
			return false
		}
		binary.LittleEndian.PutUint16(data, uint16(int16(value)))
	case 4:
		if value < -1<<31 || value > 1<<31-1 {
			return false
		}
		binary.LittleEndian.PutUint32(data, uint32(int32(value)))
	default:
		return false
	}
	return true
}

func addSigned(base uintptr, delta int64) (uintptr, bool) {
	if delta < 0 {
		magnitude := uintptr(-delta)
		if magnitude > base {
			return 0, false
		}
		return base - magnitude, true
	}
	result := base + uintptr(delta)
	return result, result >= base
}

func subtractAddress(target, base uintptr) (int64, bool) {
	if target >= base {
		delta := target - base
		if delta > uintptr(^uint64(0)>>1) {
			return 0, false
		}
		return int64(delta), true
	}
	delta := base - target
	if delta > uintptr(^uint64(0)>>1) {
		return 0, false
	}
	return -int64(delta), true
}
