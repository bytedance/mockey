//go:build go1.20 && !go1.28
// +build go1.20,!go1.28

/*
 * Copyright 2026 ByteDance Inc.
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

package fn

import "reflect"

// genericMethodClosure describes Go 1.27's synthetic wrapper for a method
// with its own type parameters. The receiver and dictionary precede the
// explicit arguments in the shared implementation's ABI.
type genericMethodClosure struct {
	entry      uintptr
	dictionary GenericInfo
	receiver   reflect.Type
	bound      bool
}
