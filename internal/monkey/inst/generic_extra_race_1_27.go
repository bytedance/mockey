//go:build race && go1.27 && !go1.28
// +build race,go1.27,!go1.28

/*
 * Copyright 2022 ByteDance Inc.
 * Modified in 2026 to support Go 1.27.
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
	"github.com/bytedance/mockey/internal/monkey/linkname"
	"github.com/bytedance/mockey/internal/tool"
)

func init() {
	// These runtime symbols cannot be referenced with go:linkname in Go 1.27.
	// Resolve their entry PCs from the function table, which linkname initializes
	// before this package, to recognize race instrumentation in generic wrappers.
	// ABIInternal and ABI0 entries can share a name, so register all matching PCs
	// instead of using FuncPCForName, which keeps only one entry for each name.
	for _, function := range linkname.FuncList() {
		switch name := function.Name(); name {
		case "runtime.racefuncenter", "runtime.racefuncenterfp", "runtime.racefuncexit",
			"runtime.raceread", "runtime.racewrite", "runtime.racereadrange", "runtime.racewriterange",
			"runtime.racereadrangepc1", "runtime.racewriterangepc1", "runtime.racecallbackthunk":
			pc := function.Entry()
			name = name[len("runtime."):]
			proxyCallRace[pc] = name
			tool.DebugPrintf("race func: %x(%v)\n", pc, name)
		}
	}
}
