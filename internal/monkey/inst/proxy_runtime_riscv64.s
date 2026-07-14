//go:build riscv64
// +build riscv64

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

DATA ·runtimeMorestackAddr+0(SB)/8, $runtime·morestack(SB)
GLOBL ·runtimeMorestackAddr(SB), RODATA|NOPTR, $8

DATA ·runtimeMorestackNoctxtAddr+0(SB)/8, $runtime·morestack_noctxt(SB)
GLOBL ·runtimeMorestackNoctxtAddr(SB), RODATA|NOPTR, $8
