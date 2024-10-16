//go:build go1.23 && mockey_stw
// +build go1.23,mockey_stw

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

package stw

import (
	_ "unsafe"
)

func newSTWCtx() ctx {
	return &stwCtx{}
}

type stwCtx struct {
	w worldStop
}

const stwForTestResetDebugLog = 16

func (ctx *stwCtx) StopTheWorld() {
	ctx.w = stopTheWorld(stwForTestResetDebugLog)
}

func (ctx *stwCtx) StartTheWorld() {
	startTheWorld(ctx.w)
}

// stwReason is an enumeration of reasons the world is stopping.
type stwReason uint8

// worldStop provides context from the stop-the-world required by the
// start-the-world.
type worldStop struct {
	reason           stwReason
	startedStopping  int64
	finishedStopping int64
	stoppingCPUTime  int64
}

//go:linkname stopTheWorld runtime.stopTheWorld
func stopTheWorld(reason stwReason) worldStop

//go:linkname startTheWorld runtime.startTheWorld
func startTheWorld(w worldStop)
