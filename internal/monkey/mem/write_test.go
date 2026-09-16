package mem

// Copyright 2023 2022 ByteDance Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"bytes"
	"errors"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/tool"
)

func TestWrite(t *testing.T) {
	data := make([]byte, common.PageSize()*3)
	target := uintptr(common.PageSize() / 2)

	expected := [][2]uintptr{
		{target, uintptr(common.PageSize())},
		{uintptr(common.PageSize()), uintptr(common.PageSize()) * 2},
		{uintptr(common.PageSize() * 2), uintptr(common.PageSize()) * 3},
		{uintptr(common.PageSize() * 3), target + uintptr(len(data))},
	}

	begin := target
	end := target + uintptr(len(data))
	index := 0
	for begin < end {
		if common.PageOf(begin) < common.PageOf(end) {
			nextPage := common.PageOf(begin) + uintptr(common.PageSize())
			buf := data[:nextPage-begin]
			data = data[nextPage-begin:]

			// err := Write(begin, buf)
			// tool.Assert(err == nil, err)
			tool.Assert(begin == expected[index][0], index, begin, expected[index][0])
			tool.Assert(begin+uintptr(len(buf)) == expected[index][1], index, begin+uintptr(len(buf)), expected[index][1])

			begin += uintptr(len(buf))
			index += 1
			continue
		}

		// err := Write(begin, data)
		// tool.Assert(err == nil, err)
		tool.Assert(begin == expected[index][0], index, begin, expected[index][0])
		tool.Assert(begin+uintptr(len(data)) == expected[index][1], index, begin+uintptr(len(data)), expected[index][1])

		break
	}
}

func TestReplaceWithWriterRestoresAfterWriteError(t *testing.T) {
	target := make([]byte, len("original"))
	copy(target, "original")
	original := append([]byte(nil), target...)
	replacement := []byte("patched!")
	applyErr := errors.New("injected apply failure")
	calls := 0

	state, err := replaceWithWriter(
		common.PtrOf(target),
		replacement,
		original,
		func(address uintptr, data []byte) error {
			copy(common.BytesOf(address, len(data)), data)
			calls++
			if calls == 1 {
				return applyErr
			}
			return nil
		},
	)

	if state != ReplaceRestored {
		t.Fatalf("state = %v, want ReplaceRestored", state)
	}
	if !errors.Is(err, applyErr) {
		t.Fatalf("error = %v, want wrapped apply error", err)
	}
	if calls != 2 {
		t.Fatalf("writer calls = %d, want 2", calls)
	}
	if !bytes.Equal(target, original) {
		t.Fatalf("target = %q, want restored %q", target, original)
	}
}

func TestReplaceWithWriterRollsBackAcrossPages(t *testing.T) {
	pageSize := uintptr(common.PageSize())
	storage := make([]byte, 3*common.PageSize())
	base := common.PtrOf(storage)
	boundary := common.PageOf(base+pageSize) + pageSize
	targetAddress := boundary - 4
	targetOffset := int(targetAddress - base)
	target := storage[targetOffset : targetOffset+8]
	copy(target, "original")
	original := append([]byte(nil), target...)
	replacement := []byte("patched!")
	applyErr := errors.New("injected second-page failure")
	calls := 0

	state, err := replaceWithWriter(
		targetAddress,
		replacement,
		original,
		func(address uintptr, data []byte) error {
			copy(common.BytesOf(address, len(data)), data)
			calls++
			if calls == 2 {
				return applyErr
			}
			return nil
		},
	)

	if state != ReplaceRestored {
		t.Fatalf("state = %v, want ReplaceRestored", state)
	}
	if !errors.Is(err, applyErr) {
		t.Fatalf("error = %v, want wrapped apply error", err)
	}
	if calls != 4 {
		t.Fatalf("writer calls = %d, want 4", calls)
	}
	if !bytes.Equal(target, original) {
		t.Fatalf("target = %q, want restored %q", target, original)
	}
}

func TestReplaceWithWriterReportsUncertainRollback(t *testing.T) {
	target := make([]byte, len("original"))
	copy(target, "original")
	original := append([]byte(nil), target...)
	replacement := []byte("patched!")
	applyErr := errors.New("injected apply failure")
	rollbackErr := errors.New("injected rollback failure")
	calls := 0

	state, err := replaceWithWriter(
		common.PtrOf(target),
		replacement,
		original,
		func(address uintptr, data []byte) error {
			copy(common.BytesOf(address, len(data)), data)
			calls++
			if calls == 1 {
				return applyErr
			}
			return rollbackErr
		},
	)

	if state != ReplaceUncertain {
		t.Fatalf("state = %v, want ReplaceUncertain", state)
	}
	if !errors.Is(err, applyErr) {
		t.Fatalf("error = %v, want wrapped apply error", err)
	}
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte(rollbackErr.Error())) {
		t.Fatalf("error = %v, want rollback failure", err)
	}
	if calls != 2 {
		t.Fatalf("writer calls = %d, want 2", calls)
	}
}

func TestReplaceWithWriterRejectsMismatchedLengths(t *testing.T) {
	target := make([]byte, len("original"))
	copy(target, "original")
	original := append([]byte(nil), target...)
	calls := 0

	state, err := replaceWithWriter(
		common.PtrOf(target),
		[]byte("short"),
		original,
		func(uintptr, []byte) error {
			calls++
			return nil
		},
	)

	if state != ReplaceRestored {
		t.Fatalf("state = %v, want ReplaceRestored", state)
	}
	if err == nil {
		t.Fatal("mismatched lengths returned no error")
	}
	if calls != 0 {
		t.Fatalf("writer calls = %d, want 0", calls)
	}
	if !bytes.Equal(target, original) {
		t.Fatalf("target = %q, want unchanged %q", target, original)
	}
}

func TestReplaceWithWriterPropagatesPanicAfterMutation(t *testing.T) {
	target := make([]byte, len("original"))
	copy(target, "original")
	original := append([]byte(nil), target...)
	replacement := []byte("patched!")
	panicValue := &struct{ message string }{message: "injected writer panic"}
	calls := 0
	var recovered interface{}

	func() {
		defer func() {
			recovered = recover()
		}()
		_, _ = replaceWithWriter(
			common.PtrOf(target),
			replacement,
			original,
			func(address uintptr, data []byte) error {
				copy(common.BytesOf(address, len(data)), data)
				calls++
				panic(panicValue)
			},
		)
	}()

	if recovered != panicValue {
		t.Fatalf("panic = %#v, want original value %#v", recovered, panicValue)
	}
	if calls != 1 {
		t.Fatalf("writer calls = %d, want 1 without rollback", calls)
	}
	if bytes.Equal(target, original) {
		t.Fatalf("target was not modified before panic: %q", target)
	}
}
