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
	"reflect"
	"testing"
	"unsafe"

	"github.com/bytedance/mockey"
	"github.com/bytedance/mockey/internal/tool"
	"github.com/bytedance/mockey/internal/unsafereflect"
)

func TestMethodByName(t *testing.T) {
	// private structure private method: *sha256.digest.checkSum
	tfn, ok := unsafereflect.MethodByName(sha256.New(), "checkSum")
	tool.Assert(ok, "private member of private structure is allowed")
	// type of `func(*sha256.digest, []byte) [32]byte`
	pFn := unsafe.Pointer(&tfn)

	mockey.PatchConvey("ReflectFuncReturn", t, func() {
		f := reflect.FuncOf([]reflect.Type{reflect.TypeOf(sha256.New())},
			[]reflect.Type{reflect.TypeOf([sha256.Size]byte{})}, false)
		fn := reflect.NewAt(f, pFn).Elem().Interface()
		// Such function cannot be exported as `(*sha256.digest).checkSum`,
		// since the receiver's type is *sha256.digest, only Return API can be used
		mockey.Mock(fn).Return([sha256.Size]byte{1: 1}).Build()
		rets := sha256.New().Sum(nil)
		want := make([]byte, sha256.Size)
		want[1] = 1
		tool.Assert(reflect.DeepEqual(want, rets), "the method should be mocked")
	})

	// See also TestMethodByNameV17 while go version above 1.17
}
