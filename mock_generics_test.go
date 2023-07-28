//go:build go1.18
// +build go1.18

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
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func sum[T int | float64](l, r T) T {
	return l + r
}

type generic[T int | string] struct {
	a T
}

func (g generic[T]) Value() T {
	return g.a
}

func (g generic[T]) Value2() T {
	return g.a + g.a
}

func TestGeneric(t *testing.T) {
	PatchConvey("generic", t, func() {
		PatchConvey("func", func() {
			MockGeneric(sum[int]).To(func(a, b int) int {
				return 999
			}).Build()
			MockGeneric(sum[float64]).Return(888).Build()
			convey.So(sum[int](1, 2), convey.ShouldEqual, 999)
			convey.So(sum[float64](1, 2), convey.ShouldEqual, 888)
		})
		PatchConvey("type", func() {
			Mock((generic[int]).Value, OptGeneric).Return(999).Build()
			Mock(GetMethod(generic[string]{}, "Value2"), OptGeneric).To(func() string {
				return "mock"
			}).Build()
			gi := generic[int]{a: 123}
			gs := generic[string]{a: "abc"}
			convey.So(gi.Value(), convey.ShouldEqual, 999)
			convey.So(gs.Value2(), convey.ShouldEqual, "mock")
		})
	})
}
