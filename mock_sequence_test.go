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

package mockey

import (
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSequenceOpt(t *testing.T) {
	PatchConvey("test sequenceOpt", t, func() {
		fn := func() (string, int) {
			fmt.Println("original fn")
			return "fn: not here", -1
		}

		tests := []struct {
			Value1 string
			Value2 int
			Times  int
		}{
			{"Alice", 2, 3},
			{"Bob", 3, 1},
			{"Tom", 4, 1},
			{"Jerry", 5, 2},
		}

		seq := Sequence("Admin", 1)
		for _, r := range tests {
			seq.Then(r.Value1, r.Value2).Times(r.Times)
		}
		Mock(fn).Return(seq).Build()

		for repeat := 3; repeat != 0; repeat-- {
			v1, v2 := fn()
			convey.So(v1, convey.ShouldEqual, "Admin")
			convey.So(v2, convey.ShouldEqual, 1)
			for _, r := range tests {
				for rIndex := 0; rIndex < r.Times; rIndex++ {
					v1, v2 := fn()
					convey.So(v1, convey.ShouldEqual, r.Value1)
					convey.So(v2, convey.ShouldEqual, r.Value2)
				}
			}
		}
	})
}
