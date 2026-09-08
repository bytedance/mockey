//go:build go1.20
// +build go1.20

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

import "testing"

type (
	originalNamedInt        int
	originalGeneric[T ~int] struct{ value T }
)

func (receiver *originalGeneric[T]) Large(value int) int {
	var frame [65536]byte
	frame[value&65535] = byte(value) + byte(receiver.value)
	return originalReadFrame(&frame, value)
}

func TestOriginalSharedGenericStackGrowth(t *testing.T) {
	var firstOriginal func(*originalGeneric[int], int) int
	var secondOriginal func(*originalGeneric[originalNamedInt], int) int
	firstCalls, secondCalls := 0, 0
	firstMock := Mock((*originalGeneric[int]).Large).Origin(&firstOriginal).To(func(receiver *originalGeneric[int], value int) int {
		firstCalls++
		return firstOriginal(receiver, value+1)
	}).Build()
	defer firstMock.UnPatch()
	secondMock := Mock((*originalGeneric[originalNamedInt]).Large).Origin(&secondOriginal).To(func(receiver *originalGeneric[originalNamedInt], value int) int {
		secondCalls++
		return secondOriginal(receiver, value+2)
	}).Build()
	defer secondMock.UnPatch()

	result := make(chan int)
	go func() { result <- (&originalGeneric[int]{}).Large(1) }()
	if got := <-result; got != 2 || firstCalls != 1 || secondCalls != 0 || firstMock.Times() != 1 || secondMock.Times() != 0 {
		t.Fatalf("first result=%d firstCalls=%d secondCalls=%d firstTimes=%d secondTimes=%d", got, firstCalls, secondCalls, firstMock.Times(), secondMock.Times())
	}
	go func() { result <- (&originalGeneric[originalNamedInt]{}).Large(1) }()
	if got := <-result; got != 3 || firstCalls != 1 || secondCalls != 1 || firstMock.Times() != 1 || secondMock.Times() != 1 {
		t.Fatalf("second result=%d firstCalls=%d secondCalls=%d firstTimes=%d secondTimes=%d", got, firstCalls, secondCalls, firstMock.Times(), secondMock.Times())
	}
}

func TestOriginalStoredUnderNewerGenericLayer(t *testing.T) {
	var firstOriginal func(*originalGeneric[int], int) int
	firstCalls := 0
	firstMock := Mock((*originalGeneric[int]).Large).Origin(&firstOriginal).To(func(receiver *originalGeneric[int], value int) int {
		firstCalls++
		return firstOriginal(receiver, value+1)
	}).Build()
	defer firstMock.UnPatch()
	// Initialize Origin's captured generic dictionary before saving a call that
	// intentionally bypasses dispatch, then install a newer shared-body layer.
	receiver := &originalGeneric[int]{}
	receiver.Large(0)
	initialCalls, initialTimes := firstCalls, firstMock.Times()
	secondCalls := 0
	secondMock := Mock((*originalGeneric[originalNamedInt]).Large).To(func(*originalGeneric[originalNamedInt], int) int {
		secondCalls++
		return 999
	}).Build()
	defer secondMock.UnPatch()
	result := make(chan int)
	go func() { result <- firstOriginal(receiver, 1) }()
	if got := <-result; got != 1 || firstCalls != initialCalls || firstMock.Times() != initialTimes || secondCalls != 0 || secondMock.Times() != 0 {
		t.Fatalf("stored Origin result=%d firstCallDelta=%d firstTimeDelta=%d secondCalls=%d secondTimes=%d", got, firstCalls-initialCalls, firstMock.Times()-initialTimes, secondCalls, secondMock.Times())
	}
}
