//go:build !(darwin && arm64)

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

import "reflect"

// useShortPatch reports whether the target should be patched with a near
// trampoline. Platforms without a short-patch strategy keep the previous
// behaviour exactly: a function too short for the long jump still fails loudly
// in Disassemble.
func useShortPatch(addr uintptr, required int) bool {
	return false
}

// PatchValueShort is never called on platforms where shortPatchSupported is
// false; it exists so that the platform-independent dispatch in PatchValue
// compiles everywhere.
func PatchValueShort(target, hook, proxy reflect.Value, unsafe bool) *Patch {
	panic("mockey: short-function patching is not supported on this platform")
}
