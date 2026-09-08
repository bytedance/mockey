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

package mockey

import (
	"bytes"
	"reflect"
	"testing"
)

//go:noinline
func originalReadFrame(frame *[65536]byte, index int) int {
	return int(frame[index&65535])
}

//go:noinline
func originalLargeFrame(value int) int {
	var frame [65536]byte
	frame[value&65535] = byte(value)
	return originalReadFrame(&frame, value)
}

func TestOriginalStackGrowth(t *testing.T) {
	t.Run("decorator", func(t *testing.T) {
		var original func(int) int
		calls := 0
		mock := Mock(originalLargeFrame).Origin(&original).To(func(value int) int {
			calls++
			return original(value + 1)
		}).Build()
		defer mock.UnPatch()
		result := make(chan int)
		go func() { result <- originalLargeFrame(1) }()
		if got := <-result; got != 2 || calls != 1 || mock.Times() != 1 || mock.MockTimes() != 1 {
			t.Fatalf("result=%d calls=%d Times=%d MockTimes=%d", got, calls, mock.Times(), mock.MockTimes())
		}
	})
	t.Run("condition miss", func(t *testing.T) {
		conditions := 0
		mock := Mock(originalLargeFrame).When(func(int) bool {
			conditions++
			return false
		}).Return(99).Build()
		defer mock.UnPatch()
		result := make(chan int)
		go func() { result <- originalLargeFrame(1) }()
		if got := <-result; got != 1 || conditions != 1 || mock.Times() != 1 || mock.MockTimes() != 0 {
			t.Fatalf("result=%d conditions=%d Times=%d MockTimes=%d", got, conditions, mock.Times(), mock.MockTimes())
		}
	})
}

func originalDirectRecursive(depth int) int {
	if depth == 0 {
		return 0
	}
	return originalDirectRecursive(depth-1) + 1
}

func originalReflectRecursive(depth int) int {
	if depth == 0 {
		return 0
	}
	return int(reflect.ValueOf(originalReflectRecursive).Call([]reflect.Value{reflect.ValueOf(depth - 1)})[0].Int()) + 1
}

func TestOriginalRecursion(t *testing.T) {
	for _, test := range []struct {
		name   string
		target func(int) int
	}{
		{"direct", originalDirectRecursive},
		{"reflect", originalReflectRecursive},
	} {
		t.Run(test.name, func(t *testing.T) {
			var original func(int) int
			calls := 0
			mock := Mock(test.target).Origin(&original).To(func(depth int) int {
				calls++
				return original(depth)
			}).Build()
			defer mock.UnPatch()
			result := make(chan int)
			go func() { result <- test.target(20) }()
			if got := <-result; got != 20 || calls != 21 || mock.Times() != 21 || mock.MockTimes() != 21 {
				t.Fatalf("result=%d calls=%d Times=%d MockTimes=%d", got, calls, mock.Times(), mock.MockTimes())
			}
		})
	}
}

func originalNestedInner(value int) int { return value + 1 }
func originalNestedOuter(value int) int { return originalNestedInner(value) + 1 }

func TestOriginalNestedMocks(t *testing.T) {
	var original func(int) int
	outerCalls, innerCalls := 0, 0
	outer := Mock(originalNestedOuter).Origin(&original).To(func(value int) int {
		outerCalls++
		return original(value)
	}).Build()
	defer outer.UnPatch()
	inner := Mock(originalNestedInner).To(func(value int) int {
		innerCalls++
		return value + 10
	}).Build()
	defer inner.UnPatch()
	result := make(chan int)
	go func() { result <- originalNestedOuter(1) }()
	if got := <-result; got != 12 || outerCalls != 1 || innerCalls != 1 || outer.Times() != 1 || inner.Times() != 1 {
		t.Fatalf("result=%d outerCalls=%d innerCalls=%d outerTimes=%d innerTimes=%d", got, outerCalls, innerCalls, outer.Times(), inner.Times())
	}
}

func originalCompareBytes(first, second []byte) bool {
	return bytes.Equal(first, second)
}

func TestOriginalWithMockedBytesEqual(t *testing.T) {
	var original func([]byte, []byte) bool
	outer := Mock(originalCompareBytes).Origin(&original).To(func(first, second []byte) bool {
		return original(first, second)
	}).Build()
	defer outer.UnPatch()
	var equalOriginal func([]byte, []byte) bool
	equalCalls := 0
	equal := Mock(bytes.Equal).Origin(&equalOriginal).To(func(first, second []byte) bool {
		equalCalls++
		return equalOriginal(first, second)
	}).Build()
	defer equal.UnPatch()
	if !originalCompareBytes([]byte("match"), []byte("match")) {
		t.Fatal("original bytes.Equal result changed")
	}
	if equalCalls != 1 || equal.Times() != 1 || outer.Times() != 1 {
		t.Fatalf("equalCalls=%d equalTimes=%d outerTimes=%d", equalCalls, equal.Times(), outer.Times())
	}
}
