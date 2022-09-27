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
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

//go:noinline
func A() int {
	return 0
}

func Proxy() int {
	return 1
}

func Hook() int {
	return 2
}

func TestPatchFunc(t *testing.T) {
	convey.Convey("TestPatchFunc", t, func() {
		fun := Proxy
		patch := PatchFunc(A, Hook, &fun)
		convey.So(A(), convey.ShouldEqual, 2)
		convey.So(fun(), convey.ShouldEqual, 0)
		patch.Unpatch()
		convey.So(A(), convey.ShouldEqual, 0)
	})
}
