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

package tests

import (
	"testing"

	. "github.com/bytedance/mockey"
)

// This file is amd64-only because the mechanism under test is: the five-byte
// E9 short branch and the relocation of the prefix it displaces exist on amd64
// alone. On arm64 the same source shape takes the long-jump path, which does
// not relocate, and calling origin dies with SIGILL -- a pre-existing failure
// reproducible on unmodified main with no short-branch code present, not
// something this feature introduced. See patch_optimized_amd64_test.go.

var shortJumpGlobal = 41

// The exact shape the short branch exists for: an early RET at nine bytes, then
// a RIP-relative load of a global. The legacy twelve-byte path refuses this
// ("function is too short to patch") because the tail is real code rather than
// 0xCC padding; the short branch copies five bytes instead and relocates them.
//
//go:noinline
func shortJumpTarget(p *int) int {
	if p == nil {
		return shortJumpGlobal
	}
	return *p
}

// TestShortBranchRescuesShortFunction is the end-to-end test for the feature:
// mock a function the baseline could not mock, call it, and call origin.
//
// It covers the whole chain at once -- five-byte E9, near-page relay,
// relocation of the copied prefix -- and it is the only test here that would
// notice the relocation being silently wrong: origin returns the global (41)
// only if the RIP-relative load was fixed up for its new address, and returns
// the argument only if the conditional branch was too.
func TestShortBranchRescuesShortFunction(t *testing.T) {
	var origin func(*int) int
	mk := Mock(shortJumpTarget).To(func(p *int) int { return origin(p) + 1000 }).Origin(&origin).Build()
	defer mk.UnPatch()

	if got := shortJumpTarget(nil); got != 1000+shortJumpGlobal {
		t.Fatalf("nil branch = %d, want %d", got, 1000+shortJumpGlobal)
	}
	v := 7
	if got := shortJumpTarget(&v); got != 1000+v {
		t.Fatalf("pointer branch = %d, want %d", got, 1000+v)
	}
}
