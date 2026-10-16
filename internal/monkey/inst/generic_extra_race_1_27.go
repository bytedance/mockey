//go:build race && go1.27 && !go1.28
// +build race,go1.27,!go1.28

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

package inst

import (
	"github.com/bytedance/mockey/internal/monkey/linkname"
	"github.com/bytedance/mockey/internal/tool"
)

func init() {
	// Go 1.27 rejects direct go:linkname references to private race helpers.
	// Only their entry PCs are needed to recognize calls in generic wrappers.
	names := make(map[string]string)
	for _, name := range []string{
		"racefuncenter",
		"racefuncexit",
		"raceread",
		"racewrite",
		"racereadrange",
		"racewriterange",
		"racereadrangepc1",
		"racewriterangepc1",
		"racecallbackthunk",
	} {
		names["runtime."+name] = name
	}
	// ABI0 implementations and ABIInternal wrappers can share a runtime name.
	// Register every entry so calls to either ABI are recognized.
	for _, fn := range linkname.FuncList() {
		if name, ok := names[fn.Name()]; ok {
			proxyCallRace[fn.Entry()] = name
			tool.DebugPrintf("race func: %x(%v)\n", fn.Entry(), name)
		}
	}
}
