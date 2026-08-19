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

import (
	"strings"
	"testing"
)

// TestDisassembleReturnsErrorNotPanic pins the one capability the split adds:
// every refusal reaches the caller as an error, with nothing panicking. This is
// what lets TryDisassemble avoid recover(), so if it regresses the probing path
// silently goes back to catching panics -- or to crashing on the ones it forgot.
func TestDisassembleReturnsErrorNotPanic(t *testing.T) {
	tests := []struct {
		name     string
		code     []byte
		required int
		checkLen bool
		wantErr  string
	}{
		{
			// Ends at a RET after one byte, and the tail is real code rather than
			// 0xCC padding, so the branch sequence would overwrite the next function.
			name:     "too short and tail is not padding",
			code:     []byte{ret, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8},
			required: branchLen,
			checkLen: true,
			wantErr:  errFunctionTooShort,
		},
		{
			// 0xff repeated is not a decodable instruction; this is the reason that
			// used to escape TryDisassemble and crash real repositories.
			name:     "undecodable instruction",
			code:     []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			required: branchLen,
			checkLen: true,
			wantErr:  "unrecognized instruction",
		},
		{
			// The unchecked (MockUnsafe) path skips the length check but still has
			// to decode, so an undecodable window is refused there too.
			name:     "undecodable instruction, unchecked path",
			code:     []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			required: branchLen,
			checkLen: false,
			wantErr:  "unrecognized instruction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				err       error
				panicked  interface{}
				completed bool
			)
			func() {
				defer func() { panicked = recover() }()
				_, err = disassemble(tt.code, tt.required, tt.checkLen)
				completed = true
			}()

			if !completed {
				t.Fatalf("disassemble panicked with %v; refusals must be returned", panicked)
			}
			if err == nil {
				t.Fatal("expected a refusal error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}

			// The same input must still make the panicking facade panic, with the
			// same text: Disassemble's contract is unchanged.
			got := recoverOf(func() { Disassemble(tt.code, tt.required, tt.checkLen) })
			if got == nil {
				t.Fatal("Disassemble must still panic on a refusal")
			}
			if msg, ok := got.(string); !ok || !strings.Contains(msg, tt.wantErr) {
				t.Fatalf("Disassemble panicked with %#v, want a string containing %q", got, tt.wantErr)
			}

			// And the probing entry point must report it as a plain false.
			if _, ok := TryDisassemble(tt.code, tt.required, tt.checkLen); ok {
				t.Fatal("TryDisassemble must report ok=false for a refusal")
			}
		})
	}
}

// TestDisassembleSuccessAgreesAcrossEntryPoints checks the happy path stays
// consistent: all three entry points must return the same cutting index.
func TestDisassembleSuccessAgreesAcrossEntryPoints(t *testing.T) {
	code := []byte{0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8, 0x48, 0x89, 0xc8}

	pos, err := disassemble(code, branchLen, true)
	if err != nil {
		t.Fatalf("disassemble returned %v, want success", err)
	}
	if got := Disassemble(code, branchLen, true); got != pos {
		t.Fatalf("Disassemble = %d, disassemble = %d", got, pos)
	}
	got, ok := TryDisassemble(code, branchLen, true)
	if !ok {
		t.Fatal("TryDisassemble reported failure on a patchable function")
	}
	if got != pos {
		t.Fatalf("TryDisassemble = %d, disassemble = %d", got, pos)
	}
}
