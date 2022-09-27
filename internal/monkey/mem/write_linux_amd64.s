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

#include "textflag.h"

#define NOP8 BYTE $0x90; BYTE $0x90; BYTE $0x90; BYTE $0x90; BYTE $0x90; BYTE $0x90; BYTE $0x90; BYTE $0x90;
#define NOP64 NOP8; NOP8; NOP8; NOP8; NOP8; NOP8; NOP8; NOP8;
#define NOP512 NOP64; NOP64; NOP64; NOP64; NOP64; NOP64; NOP64; NOP64;
#define NOP4096 NOP512; NOP512; NOP512; NOP512; NOP512; NOP512; NOP512; NOP512;

#define addr        arg + 0x00(FP)
#define data        arg + 0x08(FP)
#define size        arg + 0x10(FP)
#define mprotect    arg + 0x18(FP)
#define prot_rw     arg + 0x20(FP)
#define prot_rx     arg + 0x28(FP)

#define CMOVNEQ_AX_CX   \
    BYTE $0x48          \
    BYTE $0x0f          \
    BYTE $0x45          \
    BYTE $0xc8

TEXT ·do_replace_code(SB), NOSPLIT, $0x30 - 0
    JMP START
    NOP4096
START:
    MOVQ    addr, DI
    MOVQ    size, SI
    MOVQ    DI, AX
    ANDQ    $0x0fff, AX
    ANDQ    $~0x0fff, DI
    ADDQ    AX, SI
    MOVQ    SI, CX
    ANDQ    $0x0fff, CX
    MOVQ    $0x1000, AX
    SUBQ    CX, AX
    TESTQ   CX, CX
    CMOVNEQ_AX_CX
    ADDQ    CX, SI
    MOVQ    DI, R8
    MOVQ    SI, R9
    MOVQ    mprotect , AX
    MOVQ    prot_rw  , DX
    SYSCALL
    MOVQ    addr, DI
    MOVQ    data, SI
    MOVQ    size, CX
    REP
    MOVSB
    MOVQ    R8, DI
    MOVQ    R9, SI
    MOVQ    mprotect , AX
    MOVQ    prot_rx  , DX
    SYSCALL
    JMP     RETURN
    NOP4096
RETURN:
    RET
