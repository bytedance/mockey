//go:build linux && riscv64
// +build linux,riscv64

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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
)

const (
	writeOwnTextChild      = "MOCKEY_WRITE_OWN_TEXT_CHILD"
	writeRestoreErrorChild = "MOCKEY_WRITE_RESTORE_ERROR_CHILD"
)

func TestWriteOwnTextPage(t *testing.T) {
	if os.Getenv(writeOwnTextChild) == "1" {
		pc := reflect.ValueOf(write).Pointer()
		original := append([]byte(nil), common.BytesOf(pc, 4)...)
		if err := Write(pc, original); err != nil {
			t.Fatalf("write the writer's text page: %v", err)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteOwnTextPage$")
	cmd.Env = append(os.Environ(), writeOwnTextChild+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("writer text-page child failed: %v\n%s", err, output)
	}
}

func TestWriteRestoreErrorKeepsWriterExecutable(t *testing.T) {
	if os.Getenv(writeRestoreErrorChild) == "1" {
		pc := reflect.ValueOf(write).Pointer()
		original := append([]byte(nil), common.BytesOf(pc, 4)...)
		result := write(
			pc,
			common.PtrOf(original),
			len(original),
			common.PageOf(pc),
			common.PageSize(),
			-1,
		)
		runtime.KeepAlive(original)
		if result >= 0 {
			t.Fatalf("invalid restore protection returned %d, want a negative errno", result)
		}
		if permissions := mappingPermissions(t, pc); !strings.HasPrefix(permissions, "r-x") {
			t.Fatalf("writer page was not restored to RX: %q", permissions)
		}

		result = write(
			pc,
			common.PtrOf(original),
			len(original),
			common.PageOf(pc),
			common.PageSize(),
			syscall.PROT_READ|syscall.PROT_EXEC,
		)
		runtime.KeepAlive(original)
		if result != 0 {
			t.Fatalf("writer was not executable after fallback restore: %d", result)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteRestoreErrorKeepsWriterExecutable$")
	cmd.Env = append(os.Environ(), writeRestoreErrorChild+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("writer restore-error child failed: %v\n%s", err, output)
	}
}

func TestWriteInitialMprotectFailureDoesNotModifyTarget(t *testing.T) {
	pageSize := common.PageSize()
	mapping, err := syscall.Mmap(
		-1,
		0,
		pageSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer func() {
		if err := syscall.Munmap(mapping); err != nil {
			t.Errorf("munmap: %v", err)
		}
	}()

	original := []byte("original")
	replacement := []byte("patched!")
	copy(mapping, original)

	result := write(
		common.PtrOf(mapping),
		common.PtrOf(replacement),
		len(replacement),
		1, // Deliberately unaligned: the first mprotect must fail before copying.
		pageSize,
		syscall.PROT_READ|syscall.PROT_EXEC,
	)
	runtime.KeepAlive(replacement)
	if result >= 0 {
		t.Fatalf("initial mprotect returned %d, want a negative errno", result)
	}
	if got := mapping[:len(original)]; !bytes.Equal(got, original) {
		t.Fatalf("target changed after initial mprotect failure: got %q, want %q", got, original)
	}
}

func TestWriteAcrossExecutablePages(t *testing.T) {
	pageSize := common.PageSize()
	mapping, err := syscall.Mmap(
		-1,
		0,
		2*pageSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer func() {
		if err := syscall.Munmap(mapping); err != nil {
			t.Errorf("munmap: %v", err)
		}
	}()

	const bytesBeforeBoundary = 8
	data := []byte{
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27,
		0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37,
	}
	offset := pageSize - bytesBeforeBoundary
	if err := syscall.Mprotect(mapping, syscall.PROT_READ|syscall.PROT_EXEC); err != nil {
		t.Fatalf("mprotect RX: %v", err)
	}

	WriteWithSTW(common.PtrOf(mapping)+uintptr(offset), data)

	if got := mapping[offset : offset+len(data)]; !bytes.Equal(got, data) {
		t.Fatalf("cross-page write mismatch: got %x, want %x", got, data)
	}
	for _, addr := range []uintptr{
		common.PtrOf(mapping),
		common.PtrOf(mapping) + uintptr(pageSize),
	} {
		if perms := mappingPermissions(t, addr); !strings.HasPrefix(perms, "r-x") {
			t.Fatalf("mapping at %#x was not restored to RX: %q", addr, perms)
		}
	}
	t.Logf("validated executable cross-page write with %d-byte base pages", pageSize)
}

func TestWriteInlineMemmoveOverlap(t *testing.T) {
	pageSize := common.PageSize()
	mapping, err := syscall.Mmap(
		-1,
		0,
		pageSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer func() {
		if err := syscall.Munmap(mapping); err != nil {
			t.Errorf("munmap: %v", err)
		}
	}()

	for _, test := range []struct {
		name        string
		destination int
		source      int
		want        string
	}{
		{name: "forward", destination: 0, source: 1, want: "bcdeff"},
		{name: "backward", destination: 1, source: 0, want: "aabcde"},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy(mapping, "abcdef")
			res := write(
				common.PtrOf(mapping)+uintptr(test.destination),
				common.PtrOf(mapping)+uintptr(test.source),
				5,
				common.PageOf(common.PtrOf(mapping)),
				pageSize,
				syscall.PROT_READ|syscall.PROT_WRITE,
			)
			if res != 0 {
				t.Fatalf("write returned %d", res)
			}
			if got := string(mapping[:6]); got != test.want {
				t.Fatalf("overlap result: got %q, want %q", got, test.want)
			}
		})
	}
}

func mappingPermissions(t *testing.T, addr uintptr) string {
	t.Helper()

	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatalf("read /proc/self/maps: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var start, end uintptr
		if _, err := fmt.Sscanf(fields[0], "%x-%x", &start, &end); err != nil {
			continue
		}
		if start <= addr && addr < end {
			return fields[1]
		}
	}
	t.Fatalf("address %#x not found in /proc/self/maps", addr)
	return ""
}
