//go:build windows && (amd64 || arm64) && !mockey_disable_ss && go1.23 && !go1.28
// +build windows
// +build amd64 arm64
// +build !mockey_disable_ss
// +build go1.23
// +build !go1.28

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

package sysmon

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWindowsUsleepDuration(t *testing.T) {
	const childMarker = "MOCKEY_TEST_WINDOWS_USLEEP"
	if os.Getenv(childMarker) == "1" {
		started := time.Now()
		for index := 0; index < 3; index++ {
			usleep(20000)
		}
		elapsed := time.Since(started)
		if elapsed < 30*time.Millisecond || elapsed > 2*time.Second {
			t.Fatalf("three 20ms runtime sleeps took %s", elapsed)
		}
		return
	}

	// A mismatched register can turn a short sleep into minutes while blocking
	// the runtime thread. Bound the entire child process, including race builds.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWindowsUsleepDuration$")
	command.Env = append(os.Environ(), childMarker+"=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("runtime.usleep did not finish within 10s: %s", output)
	}
	if err != nil {
		t.Fatalf("runtime.usleep subprocess failed: %v\n%s", err, output)
	}
}
