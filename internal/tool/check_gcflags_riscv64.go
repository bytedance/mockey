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

package tool

import (
	"runtime/debug"
	"strings"
)

func IsGCFlagsSet() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-gcflags" {
			return hasRequiredGCFlags(setting.Value)
		}
	}
	return false
}

func hasRequiredGCFlags(value string) bool {
	var (
		allPackages bool
		noOptimize  bool
		noInline    bool
	)
	for _, field := range strings.Fields(value) {
		if pattern, flags, ok := strings.Cut(field, "="); ok {
			allPackages = pattern == "all"
			if !allPackages {
				continue
			}
			field = flags
		} else if !allPackages {
			continue
		}

		switch field {
		case "-N":
			noOptimize = true
		case "-l":
			noInline = true
		}
	}
	return noOptimize && noInline
}
