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

package common

import (
	"reflect"
	"testing"
)

func TestAllocatePageNear(t *testing.T) {
	target := reflect.ValueOf(TestAllocatePageNear).Pointer()
	page, err := AllocatePageNear(target)
	if err != nil {
		t.Fatal(err)
	}
	defer ReleasePage(page)

	start := PtrOf(page)
	if !Rel32Reachable(target+5, start) || !Rel32Reachable(target+5, start+uintptr(len(page)-1)) {
		t.Fatalf("page [0x%x, 0x%x] is outside rel32 range of 0x%x", start, start+uintptr(len(page)-1), target)
	}
	page[0] = 1
	page[len(page)-1] = 2
}
