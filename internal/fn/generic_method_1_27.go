//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

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

package fn

import (
	"net/url"
	"reflect"
	"runtime"
	"strings"

	"github.com/bytedance/mockey/internal/monkey/inst"
	"github.com/bytedance/mockey/internal/tool"
)

// initGenericMethod handles method expressions with their own type parameters.
// Require an explicit generic or method option: their runtime names also occur
// on ordinary closures, which must retain their existing behavior.
func (a *AnalyzerImpl) initGenericMethod() {
	if a.genericIn == nil && a.methodIn == nil {
		return
	}
	if a.genericIn != nil && !*a.genericIn || a.methodIn != nil && !*a.methodIn {
		return
	}
	if a.nameAnalyzer == nil {
		a.nameAnalyzer = NewNameAnalyzerByValue(a.targetValue)
	}
	if !a.nameAnalyzer.IsAnonymousFormat() {
		return
	}
	if a.targetType.NumIn() > 0 && genericMethodReceiverMatches(a.targetType.In(0), a.nameAnalyzer) {
		// A declared method named func1 can also have an anonymous-looking name.
		return
	}

	// Reuse the existing generic call analysis and leave dictionary recovery to
	// its execution probe, without reading a closure's captured words.
	entry, _ := inst.GetGenericAddr(a.targetValue.Pointer(), 10000)
	callee := NewNameAnalyzer(runtime.FuncForPC(entry).Name(), false)
	if !callee.IsGeneric() || !callee.HasMiddleName() {
		return
	}
	receiverMatches := a.targetType.NumIn() > 0 && genericMethodReceiverMatches(a.targetType.In(0), callee)
	if callee.IsAnonymousFormat() && !receiverMatches {
		// Keep explicitly generic functions named func1, and nested closures,
		// on the existing function path unless their receiver matches.
		return
	}
	tool.Assert(receiverMatches,
		"generic method requires the declaring receiver's method expression; bound values and promoted receivers are unsupported")
	a.generic, a.method = true, true
}

func genericMethodReceiverMatches(receiver reflect.Type, callee *NameAnalyzer) bool {
	pointer := receiver.Kind() == reflect.Pointer
	if pointer {
		receiver = receiver.Elem()
	}
	name := receiver.Name()
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	if pointer {
		name = "(*" + name + ")"
	}
	// Runtime names escape dots in the final component of the package path.
	pkgPath, err := url.PathUnescape(callee.PkgName())
	return err == nil && receiver.PkgPath() == pkgPath && name == callee.MiddleName()
}
