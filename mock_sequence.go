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

package mockey

import "github.com/bytedance/mockey/internal/tool"

type SequenceOpt interface {
	// Private make sure it is mockey private interface
	Private
	// GetNext is used by mockey, don't use it if you don't know what it does
	GetNext() []interface{}
}

type sequenceOpt interface {
	SequenceOpt
	Times(int) sequenceOpt
	Then(...interface{}) sequenceOpt
}

type sequence struct {
	MockeyPrivate // make sure it does implements mockey SequenceOpt
	values        []*sequenceValue
	curV          int // current value
	curT          int // current value times
}

type sequenceValue struct {
	v []interface{}
	t int
}

func (s *sequence) GetNext() []interface{} {
	seqV := s.values[s.curV]
	s.curT++
	if s.curT >= seqV.t {
		s.curT = 0
		s.curV++
		if s.curV >= len(s.values) {
			s.curV = 0
		}
	}
	return seqV.v
}

func (s *sequence) Then(value ...interface{}) sequenceOpt {
	s.values = append(s.values, &sequenceValue{
		v: value,
		t: 1,
	})
	return s
}

func (s *sequence) Times(t int) sequenceOpt {
	tool.Assert(t > 0, "return times should more than 0")
	s.values[len(s.values)-1].t = t
	return s
}

func Sequence(value ...interface{}) sequenceOpt {
	seq := &sequence{}
	seq.Then(value...)
	return seq
}
