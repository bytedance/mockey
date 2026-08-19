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

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

//go:noinline
func diagnosticTarget() int { return 3 }

// A refusal to patch must name the target and quote what was read there.
//
// This is not decoration. "unrecognized instruction" on its own is the one
// mockey failure that can be caused by an *earlier* mock rather than by the
// call site in the stack trace, so without an address there is nothing to
// correlate against and diagnosis means bisecting test order by hand. That is
// exactly what happened in the report this test comes from.
func TestPatchRefusalNamesTheTarget(t *testing.T) {
	addr := reflect.ValueOf(diagnosticTarget).Pointer()

	// A deliberately mangled entry: an E9 whose tail is 0x37 (AAA, which does
	// not exist in 64-bit mode). Fed to the same helper PatchValue uses, so the
	// message under test is the real one.
	mangled := []byte{0xe9, 0x11, 0x22, 0x33, 0x44, 0x37, 0x55, 0x48, 0x89, 0xe5}

	var got string
	func() {
		defer func() {
			if r := recover(); r != nil {
				got = fmt.Sprint(r)
			}
		}()
		disassembleOrExplain(addr, mangled, 12, false)
	}()

	if got == "" {
		t.Skip("this entry decodes on this architecture; nothing to report here")
	}
	for _, want := range []string{
		"diagnosticTarget",        // which function
		fmt.Sprintf("0x%x", addr), // the address, matching the MOCKEY_DEBUG ledger
		"e9 11 22 33 44 37",       // the bytes actually read
		"already patched",         // the likely cause, stated
	} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal does not mention %q; message was:\n%s", want, got)
		}
	}
}

// The happy path must be untouched: a normal target still yields a cutting
// point, not a diagnostic.
func TestPatchRefusalSilentOnSuccess(t *testing.T) {
	var proxy func() int
	p := PatchFunc(diagnosticTarget, func() int { return 4 }, &proxy, false)
	defer p.Unpatch()
	if got := diagnosticTarget(); got != 4 {
		t.Fatalf("patch not effective: %d", got)
	}
}
