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

// errFunctionTooShort is the refusal Disassemble has always panicked with when
// a target ends before the branch sequence fits and the tail is not padding.
// Kept as a constant so the panicking and error-returning paths cannot drift.
const errFunctionTooShort = "function is too short to patch"

// TryDisassemble finds the cutting point, reporting ok=false instead of
// panicking when there is no usable one.
//
// It shares its implementation with Disassemble: both call disassemble, which
// returns a refusal as an error. Probing therefore cannot miss a refusal reason
// that gets added later, and -- unlike catching the panic -- a genuine bug in
// the disassembler is not silently reported as "not patchable", because only an
// explicitly returned error counts as a refusal here.
func TryDisassemble(code []byte, required int, checkLen bool) (int, bool) {
	pos, err := disassemble(code, required, checkLen)
	return pos, err == nil
}

// DisassembleErr is TryDisassemble that hands back the refusal itself, for
// callers that want to report why rather than merely whether. It exists so that
// a caller can add context (which function, what bytes) to the reason without
// having to restate the reason -- and therefore without being able to drift
// from it.
func DisassembleErr(code []byte, required int, checkLen bool) (int, error) {
	return disassemble(code, required, checkLen)
}
