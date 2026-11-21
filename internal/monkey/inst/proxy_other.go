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

// BuildProxy copies the first instructions of the target into the proxy and
// jumps back to the rest of the target. A direct branch in the copied code is
// PC-relative, so if it leaves the copied code (for example the stack check's
// jump to the morestack call at the end of a Go function) it would land in the
// proxy page instead of the target. Such branches are redirected to absolute
// jumps appended to the proxy.
func BuildProxy(targetAddr, _ uintptr, code []byte) []byte {
	proxy, branches := relocateBranches(targetAddr, code)
	proxy = append(proxy, BranchTo(targetAddr+uintptr(len(code)))...)
	for _, branch := range branches {
		setBranchTarget(proxy, branch.pos, len(proxy))
		proxy = append(proxy, BranchTo(branch.target)...)
	}
	return proxy
}

// proxyBranch is a direct branch in the proxy whose destination lies outside
// the copied code.
type proxyBranch struct {
	pos    int     // offset of the branch instruction in the proxy
	target uintptr // absolute destination of the branch in the target
}
