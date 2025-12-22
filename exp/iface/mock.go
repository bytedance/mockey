//go:build go1.20
// +build go1.20

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

package iface

import (
	"github.com/bytedance/mockey"
	"github.com/bytedance/mockey/internal/tool"
)

type Mocker struct {
	builder *MockBuilder
	mockers []*mockey.Mocker
}

type MockBuilder struct {
	builders []*mockey.MockBuilder
}

func Mock(target interface{}) *MockBuilder {
	tool.AssertFunc(target)

	// TODO: validate, must have a interface receiver
	targets := findImplementTargets(target)
	builder := &MockBuilder{}
	for _, t := range targets {
		builder.builders = append(builder.builders, mockey.Mock(t))
	}
	return builder
}

func (builder *MockBuilder) When(when interface{}) *MockBuilder {
	panic("to be implemented") // FIXME: target arg mustn't be an interface
	return builder
}

func (builder *MockBuilder) To(hook interface{}) *MockBuilder {
	panic("to be implemented") // FIXME: target arg mustn't be an interface
	for _, b := range builder.builders {
		b.To(hook)
	}
	return builder
}

func (builder *MockBuilder) Return(results ...interface{}) *MockBuilder {
	for _, b := range builder.builders {
		b.Return(results...)
	}
	return builder
}

func (builder *MockBuilder) Build() *Mocker {
	mocker := Mocker{builder: builder}
	for _, b := range builder.builders {
		mocker.mockers = append(mocker.mockers, b.Build())
	}
	return &mocker
}

func (mocker *Mocker) Patch() *Mocker {
	for _, m := range mocker.mockers {
		m.Patch()
	}
	return mocker
}

func (mocker *Mocker) UnPatch() *Mocker {
	for _, m := range mocker.mockers {
		m.UnPatch()
	}
	return mocker
}

func (mocker *Mocker) Release() *MockBuilder {
	panic("to be implemented")
	return mocker.builder
}

func (mocker *Mocker) When(when interface{}) *Mocker {
	return mocker.rePatch(func() {
		panic("to be implemented")
	})
}

func (mocker *Mocker) To(to interface{}) *Mocker {
	return mocker.rePatch(func() {
		panic("to be implemented")
	})
}

func (mocker *Mocker) Return(results ...interface{}) *Mocker {
	return mocker.rePatch(func() {
		panic("to be implemented")
	})
}

func (mocker *Mocker) rePatch(do func()) *Mocker {
	panic("to be implemented")
	return mocker
}

func (mocker *Mocker) Times() int {
	panic("to be implemented")
	return 0
}

func (mocker *Mocker) MockTimes() int {
	panic("to be implemented")
	return 0
}
