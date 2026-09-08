//go:build race && go1.27 && !go1.28
// +build race,go1.27,!go1.28

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

package inst

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func raceGenericIdentity[T any](value T) T {
	return value
}

type raceGenericValue[T any] struct {
	value T
}

func (value raceGenericValue[T]) Value() T {
	return value.value
}

func TestGetGenericAddrRace(t *testing.T) {
	for _, test := range []struct {
		name       string
		target     interface{}
		calleeName string
	}{
		{"scalar", raceGenericIdentity[int], ".raceGenericIdentity["},
		{"large value", raceGenericIdentity[[32]uint64], ".raceGenericIdentity["},
		// The pointer wrapper reads the receiver before calling the value method.
		// Its raceread call uses ABIInternal, even when an ABI0 entry with the
		// same runtime function name also exists in the executable.
		{"pointer to value method", (*raceGenericValue[string]).Value, ".raceGenericValue["},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrapperPC := reflect.ValueOf(test.target).Pointer()
			calleePC, _ := GetGenericAddr(wrapperPC, 10000)
			callee := runtime.FuncForPC(calleePC)
			if callee == nil {
				t.Fatalf("generic wrapper resolved to unknown function at %#x", calleePC)
			}
			if calleePC == wrapperPC || !strings.Contains(callee.Name(), test.calleeName) {
				t.Fatalf("generic wrapper resolved to %q, want callee containing %q", callee.Name(), test.calleeName)
			}
		})
	}
}
