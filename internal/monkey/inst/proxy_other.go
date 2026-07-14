//go:build amd64 || arm64
// +build amd64 arm64

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

import "github.com/bytedance/mockey/internal/monkey/common"

const maxPatchSize = 64

func MaxPatchSize() int {
	return maxPatchSize
}

func AllocateProxy(_ uintptr) []byte {
	return common.AllocatePage()
}

func ReleaseProxy(proxy []byte) {
	common.ReleasePage(proxy)
}

func PatchSize(_ uintptr, code []byte, checkLen bool) int {
	return Disassemble(code, len(BranchInto(0)), checkLen)
}

func BuildPatch(targetAddr, proxyAddr, hookAddr uintptr, code []byte) (proxy, targetPatch []byte) {
	return BuildProxy(targetAddr, proxyAddr, code), BranchInto(hookAddr)
}

func BuildProxy(targetAddr, _ uintptr, code []byte) []byte {
	proxy := append([]byte(nil), code...)
	return append(proxy, BranchTo(targetAddr+uintptr(len(code)))...)
}
