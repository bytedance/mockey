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

package mem

import (
	"syscall"

	"github.com/bytedance/mockey/internal/monkey/common"
)

func Write(target uintptr, data []byte) error {
	do_replace_code(target, common.PtrOf(data), uint64(len(data)), syscall.SYS_MPROTECT, syscall.PROT_READ|syscall.PROT_WRITE, syscall.PROT_READ|syscall.PROT_EXEC)
	return nil
}

func do_replace_code(
	_ uintptr, // void   *addr
	_ uintptr, // void   *data
	_ uint64, // size_t  size
	_ uint64, // int     mprotect
	_ uint64, // int     prot_rw
	_ uint64, // int     prot_rx
)
