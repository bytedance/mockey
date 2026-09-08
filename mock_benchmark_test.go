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
	"sync/atomic"
	"testing"
)

var benchmarkCallResult int64

//go:noinline
func benchmarkCallTarget(value int) int {
	return value + 1
}

// BenchmarkMockCall excludes patch construction and removal. Run with
// -gcflags="all=-N -l", as for the test suite. Origin measures calls through the
// saved original function directly, isolating its bookkeeping from a decorator.
func BenchmarkMockCall(b *testing.B) {
	b.Run("Direct", func(b *testing.B) {
		result := 0
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result = benchmarkCallTarget(i)
		}
		b.StopTimer()
		benchmarkCallResult = int64(result)
	})
	b.Run("Return", func(b *testing.B) {
		mock := Mock(benchmarkCallTarget).Return(7).Build()
		defer mock.UnPatch()
		if got := benchmarkCallTarget(41); got != 7 {
			b.Fatalf("mock returned %d, want 7", got)
		}
		result := 0
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result = benchmarkCallTarget(i)
		}
		b.StopTimer()
		benchmarkCallResult = int64(result)
	})
	for _, parallel := range []bool{false, true} {
		name := "Origin"
		if parallel {
			name = "OriginParallel"
		}
		b.Run(name, func(b *testing.B) {
			var original func(int) int
			mock := Mock(benchmarkCallTarget).Origin(&original).Return(7).Build()
			defer mock.UnPatch()
			if got := benchmarkCallTarget(41); got != 7 {
				b.Fatalf("mock returned %d, want 7", got)
			}
			if got := original(41); got != 42 {
				b.Fatalf("original returned %d, want 42", got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					result := 0
					for pb.Next() {
						result = original(result & 255)
					}
					atomic.AddInt64(&benchmarkCallResult, int64(result))
				})
			} else {
				result := 0
				for i := 0; i < b.N; i++ {
					result = original(i)
				}
				benchmarkCallResult = int64(result)
			}
			b.StopTimer()
		})
	}
}
