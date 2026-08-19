//go:build darwin && arm64

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

package common

import (
	"sync"
	"syscall"
)

// TrampolineSize is the size of one trampoline slot (the long-jump sequence is
// 24 bytes on arm64; round up for alignment).
const TrampolineSize = 32

type nearArena struct {
	base   uintptr
	page   []byte
	cursor int
}

var (
	arenaMu sync.Mutex
	arenas  []*nearArena
)

// AllocTrampoline returns a TrampolineSize-byte slot whose address is within
// `reach` of target, carving it out of a shared near page so that many patches
// share one page. Returns nil when no page can be placed in range.
//
// The page is switched to RX once, while it is still fresh and unreachable.
// After that it is never mprotect'ed again: slots on it are live code as soon
// as any entry stub branches to them, and clearing PROT_EXEC on a page another
// thread is executing crashes that thread. Callers must therefore fill their
// slot with mem.WriteWithSTW, the same stop-the-world path used to patch any
// other live code.
func AllocTrampoline(target uintptr, reach uintptr) []byte {
	arenaMu.Lock()
	defer arenaMu.Unlock()

	inReach := func(addr uintptr) bool {
		d := int64(addr) - int64(target)
		return d > -int64(reach) && d < int64(reach)
	}

	// reuse an existing arena that is still in reach and has room
	for _, a := range arenas {
		if a.cursor+TrampolineSize > len(a.page) {
			continue
		}
		slot := a.base + uintptr(a.cursor)
		if !inReach(slot) || !inReach(slot+TrampolineSize) {
			continue
		}
		a.cursor += TrampolineSize
		return BytesOf(slot, TrampolineSize)
	}

	// otherwise map a fresh near page
	page := allocatePageWithin(target, reach)
	if page == nil {
		return nil
	}
	if err := mProtectRX(page); err != nil {
		return nil
	}
	a := &nearArena{base: PtrOf(page), page: page, cursor: TrampolineSize}
	arenas = append(arenas, a)
	return BytesOf(a.base, TrampolineSize)
}

func mProtectRX(b []byte) error {
	return syscall.Mprotect(b, syscall.PROT_READ|syscall.PROT_EXEC)
}
