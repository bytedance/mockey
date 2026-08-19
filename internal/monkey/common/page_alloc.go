//go:build !windows
// +build !windows

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
	"fmt"
	"runtime"
	"syscall"
)

func allocate(n int) ([]byte, error) {
	return mmap(0, n, 0)
}

func allocateNear(target uintptr, n int) ([]byte, error) {
	if runtime.GOOS == "linux" {
		if page, supported := allocateNearLinux(target, n); page != nil || supported {
			if page != nil {
				return page, nil
			}
			return nil, fmt.Errorf("no free page within rel32 range of 0x%x", target)
		}
	}
	return allocateNearWithHints(target, n)
}

func allocateNearLinux(target uintptr, n int) ([]byte, bool) {
	const (
		mapFixedNoReplace = 0x100000
		searchStep        = uintptr(64 << 10)
		maxDistance       = uintptr(1<<31 - 1)
	)
	supported := true
	for offset := searchStep; offset+uintptr(n) < maxDistance; offset += searchStep {
		for _, above := range []bool{true, false} {
			hint, ok := nearHint(target, offset, above)
			if !ok {
				continue
			}
			page, err := mmap(hint, n, mapFixedNoReplace)
			if err != nil {
				if err == syscall.EINVAL {
					supported = false
					return nil, supported
				}
				continue
			}
			addr := PtrOf(page)
			if addr != hint {
				// Kernels older than MAP_FIXED_NOREPLACE may ignore the flag.
				_ = free(page)
				return nil, false
			}
			if Rel32Reachable(target+5, addr+uintptr(n-1)) {
				return page, supported
			}
			_ = free(page)
		}
	}
	return nil, supported
}

func allocateNearWithHints(target uintptr, n int) ([]byte, error) {
	const (
		megabyte = uintptr(1 << 20)
		gigabyte = uintptr(1 << 30)
	)
	offsets := [...]uintptr{
		megabyte, 16 * megabyte, 64 * megabyte, 256 * megabyte,
		512 * megabyte, gigabyte, gigabyte + 512*megabyte,
	}
	for _, offset := range offsets {
		for _, above := range []bool{true, false} {
			hint, ok := nearHint(target, offset, above)
			if !ok {
				continue
			}
			page, err := mmap(hint, n, 0)
			if err != nil {
				continue
			}
			addr := PtrOf(page)
			if Rel32Reachable(target+5, addr) && Rel32Reachable(target+5, addr+uintptr(n-1)) {
				return page, nil
			}
			_ = free(page)
		}
	}
	return nil, fmt.Errorf("no free page within rel32 range of 0x%x", target)
}

func nearHint(target, offset uintptr, above bool) (uintptr, bool) {
	var hint uintptr
	if above {
		hint = target + offset
		if hint < target {
			return 0, false
		}
	} else {
		if offset > target {
			return 0, false
		}
		hint = target - offset
	}
	return PageOf(hint), true
}

func free(b []byte) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, PtrOf(b), uintptr(len(b)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func mmap(hint uintptr, n int, extraFlags uintptr) ([]byte, error) {
	addr, _, errno := syscall.Syscall6(
		syscall.SYS_MMAP,
		hint,
		uintptr(n),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE|extraFlags,
		^uintptr(0),
		0,
	)
	if errno != 0 {
		return nil, errno
	}
	return BytesOf(addr, n), nil
}
