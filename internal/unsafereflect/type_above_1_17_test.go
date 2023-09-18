//go:build go1.17
// +build go1.17

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

package unsafereflect_test

import (
	"crypto/sha256"
	"hash"
	"reflect"
	"testing"
	"unsafe"

	"github.com/bytedance/mockey"
	"github.com/bytedance/mockey/internal/tool"
	"github.com/bytedance/mockey/internal/unsafereflect"
)

func TestMethodByNameV17(t *testing.T) {
	// private structure private method: *sha256.digest.checkSum
	tfn, ok := unsafereflect.MethodByName(sha256.New(), "checkSum")
	tool.Assert(ok, "private member of private structure is allowed")
	// type of `func(*sha256.digest, []byte) [32]byte`
	pFn := unsafe.Pointer(&tfn)

	mockey.PatchConvey("InterfaceFuncReturn", t, func() {
		fn := *(*func(hash.Hash) [sha256.Size]byte)(pFn)
		// Interface to fit the function shape is allowed here
		mockey.Mock(fn).Return([sha256.Size]byte{1: 1}).Build()
		rets := sha256.New().Sum(nil)
		want := make([]byte, sha256.Size)
		want[1] = 1
		tool.Assert(reflect.DeepEqual(want, rets), "the method should be mocked")
	})

	mockey.PatchConvey("InterfaceFuncTo", t, func() {
		fn := *(*func(hash.Hash) [sha256.Size]byte)(pFn)
		// Interface to fit the function shape is allowed here,
		// since the receiver's type is interface, To API can be used here
		mockey.Mock(fn).To(func(hash.Hash) [sha256.Size]byte {
			return [sha256.Size]byte{1: 1}
		}).Build()
		rets := sha256.New().Sum(nil)
		want := make([]byte, sha256.Size)
		want[1] = 1
		tool.Assert(reflect.DeepEqual(want, rets), "the method should be mocked")
	})
}
