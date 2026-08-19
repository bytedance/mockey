//go:build amd64 && !windows
// +build amd64,!windows

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

package monkey

import (
	"reflect"
	"testing"

	"github.com/bytedance/mockey/internal/monkey/common"
	"github.com/bytedance/mockey/internal/monkey/inst"
)

var orphanGlobal = 41

// orphanEntryTarget is shaped to produce the failing case rather than merely a
// short-branch one, and both halves of that matter:
//
//   - the RIP-relative read of orphanGlobal makes the legacy twelve-byte prefix
//     unrelocatable, so PatchValue falls through to the short branch;
//   - the entry begins CMP(4) + Jcc(2), so the cut rounds up to 6 while the E9
//     is only 5 wide. That one leftover byte is the orphan.
//
// A target whose cut happens to land exactly on 5 takes the short branch too,
// but leaves nothing behind and so cannot detect this bug at all.
//
//go:noinline
func orphanEntryTarget(a int, p *int) int {
	if a > 3 {
		return orphanGlobal
	}
	return *p + a
}

// illegalOrphanTarget is orphanEntryTarget's sharper twin: same short-branch
// shape, but its orphan byte does not decode.
//
// The distinction matters because it is the whole difference between a
// regression test that fires and one that does not. orphanEntryTarget's cut
// leaves 0x08 behind, and 0x08 happens to be a legal opcode (OR r/m8, r8), so
// an unpadded entry there is still a decodable instruction stream and a later
// Patch accepts it. The bug is present but invisible.
//
// Here the arithmetic chain in the taken branch pushes the Jcc displacement out
// to 0x1e, which is not a legal opcode. An unpadded entry becomes exactly what
// the wrapper failure was: an entry mockey can no longer read back, so the next
// Patch on the same function dies in Disassemble with "unrecognized
// instruction".
//
// The displacement is load-bearing and it is a property of the generated code,
// not of the source. TestIllegalOrphanFixtureStillBites asserts it rather than
// trusting it, so that a compiler change that moves the byte is reported as
// such instead of silently retiring the test.
//
//go:noinline
func illegalOrphanTarget(a int, p *int) int {
	if a > 3 {
		s := orphanGlobal
		s = s*3 + orphanGlobal2*1
		s = s*3 + orphanGlobal2*2
		return s
	}
	return *p + a + orphanGlobal2
}

var orphanGlobal2 = 7

//go:noinline
func orphanEntryLegacyTarget(a, b, c int) int {
	s := a*3 + b*5 + c*7
	for i := 0; i < 3; i++ {
		s += i * a
	}
	return s
}

// decodable reports whether a later Patch would accept this entry.
func decodable(code []byte, required int, checkLen bool) bool {
	_, ok := inst.TryDisassemble(code, required, checkLen)
	return ok
}

// entryOf reads back what is actually in the target's first bytes right now.
func entryOf(f interface{}, n int) []byte {
	return common.BytesOf(reflect.ValueOf(f).Pointer(), n)
}

// TestPatchedEntryStaysDecodable is the regression test for a cross-test mock
// leak seen on a real repository, which surfaced as a bare
// "unrecognized instruction" panic naming neither address nor cause.
//
// A patched entry must remain a valid instruction stream, because mockey reads
// its own patched code back: every later Patch of the same function
// disassembles the live entry to find a cutting point. The cutting point is
// rounded up to an instruction boundary and is therefore often wider than the
// branch written over it -- a five-byte E9 over a six-byte CMP/JBE prologue is
// the common case -- so any byte of the cut left unwritten is the tail of an
// instruction whose head is gone. The CPU never reaches it, so the patch works;
// the disassembler does, and chokes.
//
// This only needs one mock to still be live when the next is built, which is
// what a Mock(...) at the top level of a test function always leaves behind.
func TestPatchedEntryStaysDecodable(t *testing.T) {
	var proxy func(int, *int) int
	branchWidth := len(inst.BranchInto(0))
	// Capture how the pristine entry answers, before patching anything.
	cleanMock := decodable(entryOf(orphanEntryTarget, 64), branchWidth, true)
	cleanUnsafe := decodable(entryOf(orphanEntryTarget, 64), branchWidth, false)

	p := PatchFunc(orphanEntryTarget, func(int, *int) int { return -1 }, &proxy, false)
	defer p.Unpatch()
	if !p.shortBranch {
		t.Skip("target was not patched via the short branch; nothing to pin here")
	}

	cuttingIdx := len(p.original)
	live := entryOf(orphanEntryTarget, 64)

	// Every byte of the cut must have been written: nothing of the original
	// instruction stream may survive inside it.
	if got := live[:inst.ShortBranchSize()]; got[0] != 0xe9 {
		t.Fatalf("entry does not start with E9: % x", got)
	}
	for i := inst.ShortBranchSize(); i < cuttingIdx; i++ {
		if live[i] == p.original[i] {
			t.Fatalf("byte %d of the cut still holds the original 0x%02x: an orphan "+
				"instruction tail, entry = % x", i, live[i], live[:cuttingIdx])
		}
	}

	// The property that actually matters: patching must not make the entry any
	// less decodable than it already was.
	//
	// Note this is deliberately a comparison, not an absolute assertion. A
	// target that took the short branch is frequently one the legacy path had
	// already refused ("function is too short to patch"), and that refusal is a
	// legitimate answer about the target's own shape, not damage we did. The
	// bug being pinned here is the other kind of failure: a well-formed entry
	// turning into an undecodable one because we wrote over it.
	if got := decodable(live, branchWidth, true); got != cleanMock {
		t.Fatalf("patching changed how a later Mock decodes the entry: clean=%v live=%v, entry = % x",
			cleanMock, got, live[:16])
	}
	if got := decodable(live, branchWidth, false); got != cleanUnsafe {
		t.Fatalf("patching changed how a later MockUnsafe decodes the entry: clean=%v live=%v, entry = % x",
			cleanUnsafe, got, live[:16])
	}
}

// TestRepatchWhileShortBranchIsLive is the end-to-end form: patch, leak the
// patch (never unpatch it, as a top-level Mock does), then patch the same
// function again. This is the exact sequence that panicked.
func TestRepatchWhileShortBranchIsLive(t *testing.T) {
	var proxy func(int, *int) int
	first := PatchFunc(orphanEntryTarget, func(int, *int) int { return -1 }, &proxy, false)
	defer first.Unpatch()
	if !first.shortBranch {
		t.Skip("target was not patched via the short branch; nothing to pin here")
	}
	if got := orphanEntryTarget(0, nil); got != -1 {
		t.Fatalf("first patch is not effective: got %d, want -1", got)
	}

	// deliberately NOT unpatched, then re-patched -- both the safe and the
	// unsafe way, since the report's crash came in through MockUnsafe.
	var safeProxy func(int, *int) int
	second := PatchFunc(orphanEntryTarget, func(int, *int) int { return -2 }, &safeProxy, false)
	if got := orphanEntryTarget(0, nil); got != -2 {
		t.Fatalf("re-patch over a live patch is not effective: got %d, want -2", got)
	}
	second.Unpatch()

	var unsafeProxy func(int, *int) int
	third := PatchFunc(orphanEntryTarget, func(int, *int) int { return -3 }, &unsafeProxy, true)
	if got := orphanEntryTarget(0, nil); got != -3 {
		t.Fatalf("unsafe re-patch over a live patch is not effective: got %d, want -3", got)
	}
	third.Unpatch()
}

// TestIllegalOrphanFixtureStillBites guards the guard.
//
// TestRepatchOverIllegalOrphanEntry can only fail for the right reason while
// illegalOrphanTarget's orphan byte stays undecodable. That byte is chosen by
// the compiler, so a toolchain change can quietly turn the test below into one
// that passes whether or not the fix is present -- which is precisely the
// failure this file exists to prevent, and precisely what happened to the
// original fixture.
//
// So assert the premise separately. If this fails, the fixture needs a new
// shape; the padding itself may well be fine.
func TestIllegalOrphanFixtureStillBites(t *testing.T) {
	short := inst.ShortBranchSize()
	entry := entryOf(illegalOrphanTarget, 64)

	cuttingIdx, ok := inst.TryDisassemble(entry, short, true)
	if !ok {
		t.Fatalf("fixture no longer takes the short branch: entry = % x", entry[:16])
	}
	if cuttingIdx <= short {
		t.Fatalf("fixture leaves no orphan: cut=%d short=%d, entry = % x", cuttingIdx, short, entry[:16])
	}

	// Rebuild the entry the way an unpadded PatchValue would have left it: the
	// five-byte E9 written in, the rest of the cut left untouched.
	unpadded := make([]byte, len(entry))
	copy(unpadded, entry)
	unpadded[0] = 0xe9
	for i := 1; i < short; i++ {
		unpadded[i] = 0x11
	}
	if decodable(unpadded, len(inst.BranchInto(0)), false) {
		t.Fatalf("orphan byte 0x%02x still decodes: this fixture cannot detect the bug "+
			"it was written for, entry = % x", entry[short], entry[:16])
	}
}

// TestRepatchOverIllegalOrphanEntry is the end-to-end reproduction: it goes
// through PatchValue, not through a hand-built byte array, and it fails when
// the padding is removed.
//
// The sequence is the one the wrapper hit. A mock is left live -- the ordinary
// shape of a Mock(...) at the top of a test function -- and the next mock on
// the same function has to read the patched entry back to find its own cutting
// point. Without padding that read hits the orphan and Disassemble refuses.
func TestRepatchOverIllegalOrphanEntry(t *testing.T) {
	var proxy func(int, *int) int
	first := PatchFunc(illegalOrphanTarget, func(int, *int) int { return -1 }, &proxy, false)
	defer first.Unpatch()
	if !first.shortBranch {
		t.Skip("target was not patched via the short branch; nothing to pin here")
	}

	zero := 0
	if got := illegalOrphanTarget(0, &zero); got != -1 {
		t.Fatalf("first patch is not effective: got %d, want -1", got)
	}

	// The live patch is deliberately left in place. Re-patch through
	// MockUnsafe, which is the path the wrapper crash came in through.
	var unsafeProxy func(int, *int) int
	second := PatchFunc(illegalOrphanTarget, func(int, *int) int { return -2 }, &unsafeProxy, true)
	if got := illegalOrphanTarget(0, &zero); got != -2 {
		t.Fatalf("re-patch over a live patch is not effective: got %d, want -2", got)
	}
	second.Unpatch()
}

func TestLegacyPatchedEntryStaysDecodable(t *testing.T) {
	var proxy func(int, int, int) int
	p := PatchFunc(orphanEntryLegacyTarget, func(int, int, int) int { return -1 }, &proxy, false)
	defer p.Unpatch()
	if p.shortBranch {
		t.Skip("target took the short branch; this test is about the legacy one")
	}

	live := entryOf(orphanEntryLegacyTarget, 64)
	for i := len(inst.BranchInto(0)); i < len(p.original); i++ {
		if live[i] == p.original[i] {
			t.Fatalf("byte %d of the legacy cut still holds the original 0x%02x: "+
				"an orphan instruction tail, entry = % x", i, live[i], live[:len(p.original)])
		}
	}
	if !decodable(live, len(inst.BranchInto(0)), false) {
		t.Fatalf("live legacy entry is not decodable: % x", live[:16])
	}
}

// TestOrphanTailIsTheWrapperSymptom pins the byte sequence that produced the
// original report, so a future change that reintroduces the orphan tail is
// recognisable as this bug rather than as a fresh mystery.
//
// These are the real first bytes of a stack-checked Go prologue whose body
// reads a global:
//
//	49 3b 66 10   CMP RSP, [R14+0x10]
//	76 37         JBE +0x37            <- cut lands at 6, branch is 5 wide
//	55 48 89 e5   PUSH RBP; MOV RBP, RSP
//	48 c7 05 ...  MOV [RIP+disp], imm  <- PC-relative: legacy path refused,
//	                                      so the short branch is chosen
//
// Leaving the JBE's displacement byte 0x37 behind is not a harmless leftover:
// 0x37 is AAA, which does not exist in 64-bit mode.
func TestOrphanTailIsTheWrapperSymptom(t *testing.T) {
	clean := []byte{
		0x49, 0x3b, 0x66, 0x10,
		0x76, 0x37,
		0x55,
		0x48, 0x89, 0xe5,
		0x48, 0xc7, 0x05, 0x23, 0x67, 0x18, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x48, 0x89, 0xe5, 0x5d, 0xc3,
	}
	branchWidth := len(inst.BranchInto(0))

	cut, ok := inst.TryDisassemble(clean, inst.ShortBranchSize(), true)
	if !ok {
		t.Fatal("fixture no longer takes the short branch")
	}
	if cut <= inst.ShortBranchSize() {
		t.Fatalf("fixture no longer has an orphan tail: cut=%d", cut)
	}

	unpadded := append([]byte(nil), clean...)
	copy(unpadded, []byte{0xe9, 0x11, 0x22, 0x33, 0x44})
	if _, ok := inst.TryDisassemble(unpadded, branchWidth, false); ok {
		t.Fatal("fixture no longer demonstrates the failure; it must stay a " +
			"case where an unpadded entry is undecodable, or it pins nothing")
	}

	padded := inst.PadEntry([]byte{0xe9, 0x11, 0x22, 0x33, 0x44}, cut)
	fixed := append([]byte(nil), clean...)
	copy(fixed, padded)
	if _, ok := inst.TryDisassemble(fixed, branchWidth, false); !ok {
		t.Fatalf("padded entry is still undecodable: % x", fixed[:12])
	}
}
