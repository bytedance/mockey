//go:build go1.18
// +build go1.18

/*
 * Copyright 2023 ByteDance Inc.
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

package exp

import (
	"bytes"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/bytedance/mockey/internal/tool"
	"github.com/smartystreets/goconvey/convey"
)

func TestGetPrivateMemberMethod(t *testing.T) {
	mockey.PatchConvey("FakeMethod", t, func() {
		convey.So(func() {
			GetPrivateMemberMethod[func()](&bytes.Buffer{}, "FakeMethod")
		}, convey.ShouldPanicWith, "can't reflect instance method :FakeMethod")
	})

	mockey.PatchConvey("OriginalGetMethod", t, func() {
		convey.So(func() {
			mockey.GetMethod(&bytes.Buffer{}, "empty")
		}, convey.ShouldPanicWith, "can't reflect instance method :empty")
	})

	mockey.PatchConvey("ExportFunc", t, func() {
		convey.So(func() {
			exportedFunc := GetPrivateMemberMethod[func(*bytes.Buffer) int](&bytes.Buffer{}, "Len")
			var mocked bool
			mockey.Mock(exportedFunc).To(func(buffer *bytes.Buffer) int {
				mocked = true
				return 0
			}).Build()
			_ = new(bytes.Buffer).Len()
			tool.Assert(mocked, "function should be mocked")
		}, convey.ShouldNotPanic)
	})

	mockey.PatchConvey("PrivateFunc", t, func() {
		convey.So(func() {
			privateFunc := GetPrivateMemberMethod[func(*bytes.Buffer) bool](&bytes.Buffer{}, "empty")
			var mocked bool
			mockey.Mock(privateFunc).To(func(buffer *bytes.Buffer) bool {
				mocked = true
				return true
			}).Build()
			_, _ = new(bytes.Buffer).ReadByte()
			tool.Assert(mocked, "function should be mocked")
		}, convey.ShouldNotPanic)
	})
}
