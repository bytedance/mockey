//go:build riscv64
// +build riscv64

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

import "testing"

func TestHasRequiredGCFlags(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "required order", value: "all=-N -l", want: true},
		{name: "reverse order", value: "all=-l -N", want: true},
		{name: "split assignments", value: "all=-N all=-l", want: true},
		{name: "missing no optimize", value: "all=-l", want: false},
		{name: "missing no inline", value: "all=-N", want: false},
		{name: "wrong package pattern", value: "github.com/acme/project=-N -l", want: false},
		{name: "mixed package patterns", value: "all=-N github.com/acme/project=-l", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasRequiredGCFlags(test.value); got != test.want {
				t.Fatalf("hasRequiredGCFlags(%q): got %v, want %v", test.value, got, test.want)
			}
		})
	}
}
