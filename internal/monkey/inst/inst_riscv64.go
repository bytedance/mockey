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
)

const (
	riscv64Zero = 0
	riscv64RA   = 1
	riscv64TMP  = 31 // X31 is reserved for assembler use by the Go RISC-V ABI.
	riscv64CTXT = 26 // S10, Go closure context register.
)

func BranchTo(to uintptr) (res []byte) {
	res = append(res, auipc(riscv64TMP, 0)...)             // AUIPC TMP, 0
	res = append(res, ld(riscv64TMP, riscv64TMP, 16)...)   // LD TMP, 16(TMP)
	res = append(res, jalr(riscv64Zero, riscv64TMP, 0)...) // JALR ZERO, 0(TMP)
	res = append(res, nop()...)
	res = appendUint64(res, uint64(to))
	return
}

func BranchInto(to uintptr) (res []byte) {
	res = append(res, auipc(riscv64CTXT, 0)...)            // AUIPC CTXT, 0
	res = append(res, ld(riscv64CTXT, riscv64CTXT, 16)...) // LD CTXT, 16(CTXT)
	res = append(res, ld(riscv64TMP, riscv64CTXT, 0)...)   // LD TMP, 0(CTXT)
	res = append(res, jalr(riscv64Zero, riscv64TMP, 0)...) // JALR ZERO, 0(TMP)
	res = appendUint64(res, uint64(to))
	return
}

func auipc(rd int, imm20 int32) []byte {
	return uint32ToBytes(uint32(imm20)<<12 | uint32(rd)<<7 | 0x17)
}

func ld(rd, rs1 int, imm12 int32) []byte {
	return uint32ToBytes((uint32(imm12)&0xfff)<<20 | uint32(rs1)<<15 | 0b011<<12 | uint32(rd)<<7 | 0x03)
}

func addi(rd, rs1 int, imm12 int32) []byte {
	return uint32ToBytes((uint32(imm12)&0xfff)<<20 | uint32(rs1)<<15 | uint32(rd)<<7 | 0x13)
}

func jalr(rd, rs1 int, imm12 int32) []byte {
	return uint32ToBytes((uint32(imm12)&0xfff)<<20 | uint32(rs1)<<15 | uint32(rd)<<7 | 0x67)
}

func jal(rd int, imm21 int32) []byte {
	imm := uint32(imm21)
	inst := ((imm >> 20) & 0x1 << 31) |
		((imm >> 1) & 0x3ff << 21) |
		((imm >> 11) & 0x1 << 20) |
		((imm >> 12) & 0xff << 12) |
		uint32(rd)<<7 |
		0x6f
	return uint32ToBytes(inst)
}

func nop() []byte {
	return uint32ToBytes(0x00000013)
}

func uint32ToBytes(v uint32) []byte {
	res := make([]byte, 4)
	binary.LittleEndian.PutUint32(res, v)
	return res
}

func appendUint64(res []byte, v uint64) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	return append(res, buf[:]...)
}

func add(rd, rs1, rs2 int) []byte {
	return uint32ToBytes(uint32(rs2)<<20 | uint32(rs1)<<15 | uint32(rd)<<7 | 0x33)
}

// cJ encodes C.J. The immediate bit order is
// [11|4|9:8|10|6|7|3:1|5] in instruction bits [12:2].
func cJ(imm12 int32) []byte {
	imm := uint32(imm12)
	inst := uint16(0xa001)
	inst |= uint16((imm>>11)&0x1) << 12
	inst |= uint16((imm>>4)&0x1) << 11
	inst |= uint16((imm>>8)&0x3) << 9
	inst |= uint16((imm>>10)&0x1) << 8
	inst |= uint16((imm>>6)&0x1) << 7
	inst |= uint16((imm>>7)&0x1) << 6
	inst |= uint16((imm>>1)&0x7) << 3
	inst |= uint16((imm>>5)&0x1) << 2
	return uint16ToBytes(inst)
}

// decodeCJOffset decodes the C.J immediate locally. x/arch v0.23 maps
// instruction bits [5:3] incorrectly, so relocation must not use its Simm.
func decodeCJOffset(enc []byte) int32 {
	instruction := binary.LittleEndian.Uint16(enc)
	var immediate int32
	immediate |= int32((instruction>>12)&0x1) << 11
	immediate |= int32((instruction>>11)&0x1) << 4
	immediate |= int32((instruction>>9)&0x3) << 8
	immediate |= int32((instruction>>8)&0x1) << 10
	immediate |= int32((instruction>>7)&0x1) << 6
	immediate |= int32((instruction>>6)&0x1) << 7
	immediate |= int32((instruction>>3)&0x7) << 1
	immediate |= int32((instruction>>2)&0x1) << 5
	return immediate << 20 >> 20
}

func uint16ToBytes(v uint16) []byte {
	res := make([]byte, 2)
	binary.LittleEndian.PutUint16(res, v)
	return res
}
