//go:build !go1.19
// +build !go1.19

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

package common

import "unsafe"

/*
 * This may lead to runtime.ReadMemStats inaccurate
 * See https://github.com/bytedance/mockey/issues/13
 */

//go:linkname sysAlloc runtime.sysAlloc
func sysAlloc(n uintptr, sysStat *uint64) unsafe.Pointer

//go:linkname sysFree runtime.sysFree
func sysFree(v unsafe.Pointer, n uintptr, sysStat *uint64)
