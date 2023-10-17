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
	"unsafe"

	"github.com/bytedance/mockey/internal/tool"
	"github.com/bytedance/mockey/internal/unsafereflect"
)

// GetPrivateMemberMethod resolve a method from an instance, include private method.
//
// F must fit the shape of specific method, include receiver as the first argument.
// Especially, the receiver can be replaced as interface when F is declaring,
// this will be very useful when receiver type is not exported for other packages.
//
// for example:
//
//	GetPrivateMemberMethod[func(*bytes.Buffer) bool](&bytes.Buffer{}, "empty")
//	GetPrivateMemberMethod[func(hash.Hash) [sha256.Size]byte](sha256.New(), "checkSum")
func GetPrivateMemberMethod[F interface{}](instance interface{}, methodName string) interface{} {
	tfn, ok := unsafereflect.MethodByName(instance, methodName)
	if !ok {
		tool.Assert(false, "can't reflect instance method :%v", methodName)
		return nil
	}
	// return with (unsafe) function type cast
	return *(*F)(unsafe.Pointer(&tfn))
}
