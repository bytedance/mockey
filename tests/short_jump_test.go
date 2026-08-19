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

// A function the legacy path has always handled, used as the negative control.
//
//go:noinline
func longEnoughTarget(a int) int { return a*3 + 1 }

// TestLegacyPathUnchanged is the negative control: a function the legacy path
// already handled must behave exactly as before, so a green run of the test
// above cannot be explained by the checks simply being loosened.
func TestLegacyPathUnchanged(t *testing.T) {
	var origin func(int) int
	mk := Mock(longEnoughTarget).To(func(a int) int { return origin(a) + 1000 }).Origin(&origin).Build()
	if got := longEnoughTarget(5); got != 1016 {
		t.Fatalf("hook+origin = %d, want 1016", got)
	}
	mk.UnPatch()
	if got := longEnoughTarget(5); got != 16 {
		t.Fatalf("after unpatch = %d, want 16", got)
	}
}
