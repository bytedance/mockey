//go:build amd64 && !windows
// +build amd64,!windows

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

package monkey

import (
	"reflect"
	"syscall"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
)

var relayLiveGlobal = 41

// Shaped so the short-branch path takes it: an early RET, then a RIP-relative
// global read, which the legacy 12-byte path refuses to copy.
//
//go:noinline
func relayLiveTarget(p *int) int {
	if p == nil {
		return relayLiveGlobal
	}
	return *p
}

//go:noinline
func relayLiveLongTarget(a, b, c int) int {
	s := a*3 + b*5 + c*7
	for i := 0; i < 3; i++ {
		s += i * a
	}
	return s
}

// pageIsMapped answers without touching the page: msync reports ENOMEM for an
// address that is not mapped.
func pageIsMapped(addr uintptr) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_MSYNC, addr&^0xfff, 0x1000, 4 /*MS_ASYNC*/)
	return errno != syscall.ENOMEM
}

// TestShortBranchRelayStaysMappedAfterUnpatch is the regression test for an
// unrecoverable crash seen on a real repository.
//
// With the short branch, the target's entry is a five-byte E9 into a relay that
// lives inside the trampoline page, so every call to the target passes through
// that page -- not just calls through origin, as on the legacy path. Unpatch
// used to unmap it, leaving any goroutine that had already taken the jump
// executing unmapped memory: "unexpected fault address <page>+0x20", a fatal
// error that recover cannot catch.
//
// Restoring the entry bytes does not close the window, because the jump has
// already happened, so the page must outlive the patch.
func TestShortBranchRelayStaysMappedAfterUnpatch(t *testing.T) {
	var proxy func(*int) int
	p := PatchFunc(relayLiveTarget, func(*int) int { return -1 }, &proxy, false)
	if !p.shortBranch {
		t.Skip("target was not patched via the short branch; nothing to pin here")
	}
	hook := p.hook
	relay := common.PtrOf(p.code) + 0x20
	if !pageIsMapped(relay) {
		t.Fatal("relay page is not mapped while patched")
	}

	p.Unpatch()

	if !pageIsMapped(relay) {
		t.Fatal("relay page was unmapped by Unpatch; an in-flight caller would " +
			"fault on it unrecoverably")
	}
	retiredRelays.Lock()
	retired := retiredRelays.pages[len(retiredRelays.pages)-1]
	retiredRelays.Unlock()
	if reflect.ValueOf(retired.hook).Pointer() != reflect.ValueOf(hook).Pointer() {
		t.Fatal("retired relay lost the hook function value embedded as a raw address")
	}
	if got := relayLiveTarget(nil); got != relayLiveGlobal {
		t.Fatalf("target not restored: got %d, want %d", got, relayLiveGlobal)
	}
}

// The legacy path keeps releasing its page: nothing but origin ever reaches it,
// so there is no window to protect and no reason to leak.
func TestLegacyPatchStillReleasesItsPage(t *testing.T) {
	var proxy func(int, int, int) int
	p := PatchFunc(relayLiveLongTarget, func(int, int, int) int { return -1 }, &proxy, false)
	if p.shortBranch {
		t.Skip("target took the short branch; this test is about the legacy one")
	}
	page := common.PtrOf(p.code)
	p.Unpatch()
	if pageIsMapped(page) {
		t.Fatal("legacy trampoline page should still be released by Unpatch")
	}
}
