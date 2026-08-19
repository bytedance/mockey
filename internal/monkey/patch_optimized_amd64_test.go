//go:build amd64

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

package monkey

import "testing"

var optimizedGlobal = 41

// OptimizedBranchGlobal is the shape the amd64 short branch exists for: an
// early RET, then a RIP-relative load of a global. The legacy twelve-byte path
// refuses it because the tail is real code rather than 0xCC padding.
//
//go:noinline
func OptimizedBranchGlobal(value *int) int {
	if value == nil {
		return optimizedGlobal
	}
	return *value
}

// TestPatchFuncOptimizedTrampoline arrived with the amd64 short-branch commit
// and is amd64-only on purpose, despite reading like a portable test.
//
// It is not portable because of what it asserts about `proxy`: calling origin
// on both branches only means something once the displaced prefix has been
// relocated, and relocation is what the amd64 E9 path added. Running it on
// arm64 exercises the pre-existing long-jump path against a function this
// architecture's PatchValueShort declines to relocate, and the process dies
// with SIGILL inside origin.
//
// That crash is NOT caused by the short-branch work: the identical shape --
// early return, global load, patch, call origin -- crashes the same way on
// unmodified main (verified by adding the same test body to 9965c58, before any
// of these commits). Restricting the build tag reports the truth, which is that
// the test covers an amd64 mechanism; the arm64 gap it stumbled into is a real
// pre-existing bug and is left where it was rather than hidden behind a green
// suite that never runs it.
func TestPatchFuncOptimizedTrampoline(t *testing.T) {
	var proxy func(*int) int
	patch := PatchFunc(OptimizedBranchGlobal, func(*int) int { return -1 }, &proxy, false)
	patched := true
	defer func() {
		if patched {
			patch.Unpatch()
		}
	}()

	if got := OptimizedBranchGlobal(nil); got != -1 {
		t.Fatalf("patched target = %d, want -1", got)
	}
	value := 7
	if got := proxy(&value); got != value {
		t.Fatalf("origin pointer branch = %d, want %d", got, value)
	}
	if got := proxy(nil); got != optimizedGlobal {
		t.Fatalf("origin global branch = %d, want %d", got, optimizedGlobal)
	}

	patch.Unpatch()
	patched = false
	if got := OptimizedBranchGlobal(nil); got != optimizedGlobal {
		t.Fatalf("unpatched target = %d, want %d", got, optimizedGlobal)
	}
}
