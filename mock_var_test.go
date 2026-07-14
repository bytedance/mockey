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

	. "github.com/smartystreets/goconvey/convey"
)

func TestVarPatchConvey(t *testing.T) {
	b := 1
	a := 10
	PatchConvey("test mock2", t, func() {
		PatchConvey("test mock3", func() {
			MockValue(&a).To(20)
			So(a, ShouldEqual, 20)
			PatchConvey("test mock4", func() {
				MockValue(&a).To(30)
				MockValue(&b).To(40)
				So(b, ShouldEqual, 40)
				So(a, ShouldEqual, 30)
			})
			So(b, ShouldEqual, 1)
		})
		So(b, ShouldEqual, 1)
		So(a, ShouldEqual, 10)

		PatchConvey("test mock5", func() {
			MockValue(&a).To(30)
			So(a, ShouldEqual, 30)
		})

		So(a, ShouldEqual, 10)
	})
}

type testStruct struct {
	a string
	b int
}

func TestVarStruct(t *testing.T) {
	ttt := &testStruct{
		a: "1",
		b: 2,
	}
	PatchConvey("test mock2", t, func() {
		PatchConvey("test mock3", func() {
			MockValue(&ttt).To(&testStruct{
				a: "2",
				b: 3,
			})
			So(ttt.a, ShouldEqual, "2")
			PatchConvey("test mock3 a", func() {
				MockValue(&ttt).To(&testStruct{
					a: "3",
					b: 3,
				})
				So(ttt.a, ShouldEqual, "3")
			})
			PatchConvey("test mock3 b", func() {
				MockValue(&ttt).To(nil)
				So(ttt, ShouldBeNil)
			})
		})
	})
}

func (t *testStruct) String() string {
	return t.a
}

func TestVarStruct2(t *testing.T) {
	Convey("test mock nil", t, func() {
		var ttt fmt.Stringer
		PatchConvey("test mock3", func() {
			MockValue(&ttt).To(&testStruct{
				a: "2",
				b: 3,
			})
			So(ttt.(*testStruct).a, ShouldEqual, "2")
		})
		So(ttt, ShouldBeNil)
	})
}

func TestDuplicateMockValueDoesNotMutateTarget(t *testing.T) {
	value := 1
	PatchRun(func() {
		first := MockValue(&value).To(2)

		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			MockValue(&value).To(3)
		}()
		if recovered == nil {
			t.Fatal("duplicate MockValue did not panic")
		}
		if value != 2 {
			t.Fatalf("value after rejected duplicate = %d, want 2", value)
		}
		first.UnPatch()
		if value != 1 {
			t.Fatalf("value after unpatch = %d, want 1", value)
		}
	})
	if value != 1 {
		t.Fatalf("value after PatchRun = %d, want 1", value)
	}
}

func TestNestedMockValueLayersRequireLIFO(t *testing.T) {
	value := 1
	PatchRun(func() {
		outer := MockValue(&value).To(2)
		if value != 2 {
			t.Fatalf("outer value = %d, want 2", value)
		}
		PatchRun(func() {
			MockValue(&value).To(3)
			if value != 3 {
				t.Fatalf("inner value = %d, want 3", value)
			}

			var recovered interface{}
			func() {
				defer func() { recovered = recover() }()
				outer.UnPatch()
			}()
			if recovered == nil {
				t.Fatal("out-of-order UnPatch did not panic")
			}
			if value != 3 {
				t.Fatalf("value after rejected UnPatch = %d, want 3", value)
			}
		})
		if value != 2 {
			t.Fatalf("value after inner cleanup = %d, want 2", value)
		}
	})
	if value != 1 {
		t.Fatalf("value after outer cleanup = %d, want 1", value)
	}
}

func TestRootMockValueCanBeLayeredByContext(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	value := 1
	root := MockValue(&value).To(2)
	if value != 2 {
		t.Fatalf("root value = %d, want 2", value)
	}

	PatchRun(func() {
		MockValue(&value).To(3)
		if value != 3 {
			t.Fatalf("nested value = %d, want 3", value)
		}
	})
	if value != 2 {
		t.Fatalf("value after nested cleanup = %d, want 2", value)
	}
	root.UnPatch()
	if value != 1 {
		t.Fatalf("value after root unpatch = %d, want 1", value)
	}
}

func TestPrecreatedMockValueCapturesInstalledLayer(t *testing.T) {
	value := 1
	inner := MockValue(&value)

	PatchRun(func() {
		outer := MockValue(&value).To(2)
		if value != 2 {
			t.Fatalf("outer value = %d, want 2", value)
		}

		PatchRun(func() {
			inner.To(3)
			if value != 3 {
				t.Fatalf("inner value = %d, want 3", value)
			}
		})
		if value != 2 {
			t.Fatalf("value after inner cleanup = %d, want 2", value)
		}
		outer.UnPatch()
		if value != 1 {
			t.Fatalf("value after outer unpatch = %d, want 1", value)
		}
	})
	if value != 1 {
		t.Fatalf("value after PatchRun = %d, want 1", value)
	}
}

func TestMockValueCanBeReused(t *testing.T) {
	value := 1
	mocker := MockValue(&value)

	PatchRun(func() {
		mocker.To(2)
		if value != 2 {
			t.Fatalf("first patched value = %d, want 2", value)
		}
		mocker.UnPatch()
		if value != 1 {
			t.Fatalf("value after first unpatch = %d, want 1", value)
		}

		value = 5
		mocker.To(6)
		if value != 6 {
			t.Fatalf("second patched value = %d, want 6", value)
		}
		mocker.UnPatch()
		if value != 5 {
			t.Fatalf("value after second unpatch = %d, want 5", value)
		}
	})
	if value != 5 {
		t.Fatalf("value after PatchRun = %d, want 5", value)
	}
}

func TestConcurrentScopesRejectSameTarget(t *testing.T) {
	value := 1

	outerReady := make(chan struct{})
	releaseOuter := make(chan struct{})
	outerDone := make(chan interface{}, 1)
	go runBlockedPatchScope(&value, 2, outerReady, releaseOuter, outerDone)
	<-outerReady

	rejected := make(chan interface{}, 1)
	go func() {
		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			PatchRun(func() {
				MockValue(&value).To(3)
			})
		}()
		rejected <- recovered
	}()
	patchPanic := <-rejected
	afterRejected := value

	close(releaseOuter)
	outerPanic := <-outerDone

	if patchPanic == nil {
		t.Fatal("same-target patch from a concurrent scope owner did not panic")
	}
	if afterRejected != 2 {
		t.Fatalf("value after rejected concurrent patch = %d, want 2", afterRejected)
	}
	if outerPanic != nil {
		t.Fatalf("outer scope cleanup panicked: %v", outerPanic)
	}
	if value != 1 {
		t.Fatalf("value after outer scope cleanup = %d, want 1", value)
	}
}

func TestConcurrentRootMockValueHasSingleWinner(t *testing.T) {
	UnPatchAll()
	defer UnPatchAll()

	const workers = 8
	value := 0
	mockers := make([]*MockerVar, workers)
	for i := range mockers {
		mockers[i] = MockValue(&value)
	}

	start := make(chan struct{})
	results := make(chan int, workers)
	for i, mocker := range mockers {
		go func(value int, mocker *MockerVar) {
			<-start
			defer func() {
				if recover() != nil {
					results <- 0
				}
			}()
			mocker.To(value)
			results <- value
		}(i+1, mocker)
	}

	close(start)
	winner := 0
	successCount := 0
	for range workers {
		if result := <-results; result != 0 {
			successCount++
			winner = result
		}
	}
	if successCount != 1 {
		t.Fatalf("successful MockValue calls = %d, want 1", successCount)
	}
	if value != winner {
		t.Fatalf("patched value = %d, want winner %d", value, winner)
	}
	UnPatchAll()
	if value != 0 {
		t.Fatalf("value after cleanup = %d, want 0", value)
	}
}
