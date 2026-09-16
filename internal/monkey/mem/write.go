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

package mem

import (
	"fmt"
	"runtime"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/stw"
	"github.com/bytedance/mockey/internal/monkey/sysmon"
	"github.com/bytedance/mockey/internal/tool"
)

// ReplaceState describes the state of the target after ReplaceWithSTW returns.
type ReplaceState uint8

const (
	// ReplaceApplied means the replacement was installed successfully.
	ReplaceApplied ReplaceState = iota
	// ReplaceRestored means installing the replacement failed, but the original
	// bytes were restored before the world was resumed.
	ReplaceRestored
	// ReplaceUncertain means both the installation and its rollback failed. The
	// caller must assume that the target can still reference the replacement.
	ReplaceUncertain
)

type writeFunc func(target uintptr, data []byte) error

type replaceError struct {
	apply    error
	rollback error
}

func (e *replaceError) Error() string {
	if e.rollback == nil {
		return fmt.Sprintf("replace target: %v (original bytes restored)", e.apply)
	}
	return fmt.Sprintf("replace target: %v; rollback target: %v", e.apply, e.rollback)
}

func (e *replaceError) Unwrap() error {
	return e.apply
}

// WriteWithSTW copies data bytes to the target address and replaces the original bytes, during which it will stop the
// world (only the current goroutine's P is running).
func WriteWithSTW(target uintptr, data []byte) {
	resumeFn := suspendRuntime()
	defer resumeFn()

	err := writeRange(target, data, Write)
	tool.Assert(err == nil, err)
}

// ReplaceWithSTW installs replacement at target. If installation fails, it
// restores original before resuming the world. A ReplaceUncertain result means
// that the caller must keep every object referenced by replacement alive.
// A writer panic is propagated after the runtime resumes; the caller must then
// treat the target as uncertain because the writer can already have modified it.
func ReplaceWithSTW(target uintptr, replacement, original []byte) (ReplaceState, error) {
	resumeFn := suspendRuntime()
	defer resumeFn()

	return replaceWithWriter(target, replacement, original, Write)
}

func replaceWithWriter(target uintptr, replacement, original []byte, writer writeFunc) (ReplaceState, error) {
	if len(replacement) != len(original) {
		return ReplaceRestored, fmt.Errorf(
			"replacement and original lengths differ: %d != %d",
			len(replacement), len(original),
		)
	}
	applyErr := writeRange(target, replacement, writer)
	if applyErr == nil {
		return ReplaceApplied, nil
	}

	rollbackErr := writeRange(target, original, writer)
	if rollbackErr == nil {
		return ReplaceRestored, &replaceError{apply: applyErr}
	}
	return ReplaceUncertain, &replaceError{apply: applyErr, rollback: rollbackErr}
}

func writeRange(target uintptr, data []byte, writer writeFunc) error {
	begin := target
	end := target + uintptr(len(data))
	for begin < end {
		if common.PageOf(begin) < common.PageOf(end) {
			nextPage := common.PageOf(begin) + uintptr(common.PageSize())
			buf := data[:nextPage-begin]
			data = data[nextPage-begin:]
			if err := writer(begin, buf); err != nil {
				return fmt.Errorf("write %#x: %w", begin, err)
			}
			begin += uintptr(len(buf))
			continue
		}
		if err := writer(begin, data); err != nil {
			return fmt.Errorf("write %#x: %w", begin, err)
		}
		return nil
	}
	return nil
}

func suspendRuntime() (resume func()) {
	runtime.LockOSThread()
	stwResume := stw.StopTheWorld()
	// Suspend the system monitor thread to avoid SIGBUS errors during memory writes
	// See https://github.com/bytedance/mockey/issues/68 for more details.
	sysmonResume := sysmon.SuspendSysmon()

	resume = func() {
		sysmonResume()
		stwResume()
		runtime.UnlockOSThread()
	}
	return
}
