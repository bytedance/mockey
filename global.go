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

import (
	"reflect"
	"sync"

	"github.com/bytedance/mockey/internal/tool"
	"github.com/smartystreets/goconvey/convey"
)

type mockScope struct {
	owner      int64
	byIdentity map[uintptr]mockerInstance
	order      []mockerInstance
}

var (
	// mockLifecycleMu serializes all process-wide target and layer changes.
	mockLifecycleMu sync.Mutex
	// mockRegistryMu protects scope and registration metadata only.
	mockRegistryMu        sync.Mutex
	rootMockScope         = &mockScope{byIdentity: make(map[uintptr]mockerInstance)}
	mockScopesByGoroutine = make(map[int64][]*mockScope)
	mockScopeByInstance   = make(map[mockerInstance]*mockScope)
	mockOwnerByInstance   = make(map[mockerInstance]int64)
	mockLayersByPatchKey  = make(map[uintptr][]mockerInstance)
)

func registrationOwner(scope *mockScope, goroutineID int64) int64 {
	if scope == rootMockScope {
		return goroutineID
	}
	return scope.owner
}

// registrationScopeLocked selects a local scope first, then the innermost scope
// of the sole active owner. It falls back to root when ownership is ambiguous.
func registrationScopeLocked(goroutineID int64) *mockScope {
	if scopes := mockScopesByGoroutine[goroutineID]; len(scopes) > 0 {
		return scopes[len(scopes)-1]
	}

	var soleScopes []*mockScope
	for _, scopes := range mockScopesByGoroutine {
		if len(scopes) == 0 {
			continue
		}
		if soleScopes != nil {
			return rootMockScope
		}
		soleScopes = scopes
	}
	if len(soleScopes) > 0 {
		return soleScopes[len(soleScopes)-1]
	}
	return rootMockScope
}

// localMockScopeLocked never inherits a context owned by another goroutine.
func localMockScopeLocked(goroutineID int64) *mockScope {
	if scopes := mockScopesByGoroutine[goroutineID]; len(scopes) > 0 {
		return scopes[len(scopes)-1]
	}
	return rootMockScope
}

func addToGlobal(mocker mockerInstance) {
	goroutineID := tool.GetGoroutineID()
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()

	scope := registrationScopeLocked(goroutineID)
	// Keep registration defensive even though callers validate before mutating a target.
	assertNotInGlobalLocked(mocker, scope, goroutineID)
	identityKey := mocker.identityKey()
	patchKey := mocker.layerKey()
	tool.DebugPrintf("[addToGlobal] identity 0x%x, patch target 0x%x added\n", identityKey, patchKey)
	scope.byIdentity[identityKey] = mocker
	scope.order = append(scope.order, mocker)
	mockScopeByInstance[mocker] = scope
	mockOwnerByInstance[mocker] = registrationOwner(scope, goroutineID)
	mockLayersByPatchKey[patchKey] = append(mockLayersByPatchKey[patchKey], mocker)
}

func assertNotInGlobal(mocker mockerInstance) {
	goroutineID := tool.GetGoroutineID()
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()
	assertNotInGlobalLocked(mocker, registrationScopeLocked(goroutineID), goroutineID)
}

func assertNotInGlobalLocked(mocker mockerInstance, scope *mockScope, goroutineID int64) {
	identityKey := mocker.identityKey()
	if last, ok := scope.byIdentity[identityKey]; ok {
		tool.Assert(false, "re-mock %v, previous mock at: %v", last.name(), last.caller())
	}

	patchKey := mocker.layerKey()
	layers := mockLayersByPatchKey[patchKey]
	if len(layers) == 0 {
		return
	}
	previous := layers[len(layers)-1]
	previousScope, registered := mockScopeByInstance[previous]
	tool.Assert(registered, "patch target 0x%x has inconsistent layer metadata", patchKey)
	previousOwner, ownerRegistered := mockOwnerByInstance[previous]
	tool.Assert(ownerRegistered, "patch target 0x%x has inconsistent owner metadata", patchKey)
	if previousScope == rootMockScope && scope != rootMockScope {
		return
	}
	if previousScope == rootMockScope && scope == rootMockScope && previousOwner == goroutineID {
		return
	}
	if previousScope != rootMockScope && scope != rootMockScope && previousScope.owner == scope.owner {
		return
	}
	tool.Assert(false, "cannot layer mock %v across concurrent mock scope owners, previous mock at: %v", mocker.name(), previous.caller())
}

func assertTopInGlobal(mocker mockerInstance) {
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()

	patchKey := mocker.layerKey()
	layers := mockLayersByPatchKey[patchKey]
	tool.Assert(len(layers) > 0 && layers[len(layers)-1] == mocker, "mocks for patch target 0x%x must be unpatched in reverse creation order", patchKey)
}

func removeFromGlobal(mocker mockerInstance) {
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()

	identityKey := mocker.identityKey()
	patchKey := mocker.layerKey()
	scope, registered := mockScopeByInstance[mocker]
	tool.Assert(registered, "mocker identity 0x%x is not registered", identityKey)
	current, ok := scope.byIdentity[identityKey]
	tool.Assert(ok && current == mocker, "mocker identity 0x%x has inconsistent scope metadata", identityKey)
	layers := mockLayersByPatchKey[patchKey]
	tool.Assert(len(layers) > 0 && layers[len(layers)-1] == mocker, "mocks for patch target 0x%x must be unpatched in reverse creation order", patchKey)
	orderIndex := -1
	for index := len(scope.order) - 1; index >= 0; index-- {
		if scope.order[index] == mocker {
			orderIndex = index
			break
		}
	}
	tool.Assert(orderIndex >= 0, "mocker identity 0x%x has inconsistent order metadata", identityKey)

	tool.DebugPrintf("[removeFromGlobal] identity 0x%x, patch target 0x%x removed\n", identityKey, patchKey)
	delete(scope.byIdentity, identityKey)
	scope.order = append(scope.order[:orderIndex], scope.order[orderIndex+1:]...)
	delete(mockScopeByInstance, mocker)
	delete(mockOwnerByInstance, mocker)
	if len(layers) == 1 {
		delete(mockLayersByPatchKey, patchKey)
	} else {
		mockLayersByPatchKey[patchKey] = layers[:len(layers)-1]
	}
}

func pushGlobalScope() *mockScope {
	goroutineID := tool.GetGoroutineID()
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()

	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()
	scope := &mockScope{owner: goroutineID, byIdentity: make(map[uintptr]mockerInstance)}
	mockScopesByGoroutine[goroutineID] = append(mockScopesByGoroutine[goroutineID], scope)
	return scope
}

func popGlobalScope(scope *mockScope) {
	goroutineID := tool.GetGoroutineID()
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()

	mockRegistryMu.Lock()
	scopes := mockScopesByGoroutine[goroutineID]
	ownsScope := scope != nil && scope.owner == goroutineID && len(scopes) > 0 && scopes[len(scopes)-1] == scope
	mockRegistryMu.Unlock()
	tool.Assert(ownsScope, "mock scope must be popped by its owning goroutine in reverse nesting order")

	unpatchAllScopeLocked(scope)
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()
	scopes = mockScopesByGoroutine[goroutineID]
	tool.Assert(len(scopes) > 0 && scopes[len(scopes)-1] == scope, "mock scope ownership changed during cleanup")
	if len(scopes) == 1 {
		delete(mockScopesByGoroutine, goroutineID)
	} else {
		mockScopesByGoroutine[goroutineID] = scopes[:len(scopes)-1]
	}
}

func unpatchAllScopeLocked(scope *mockScope) {
	for {
		mockRegistryMu.Lock()
		if len(scope.order) == 0 {
			mockRegistryMu.Unlock()
			return
		}
		mocker := scope.order[len(scope.order)-1]
		mockRegistryMu.Unlock()

		mocker.unPatchLocked()
	}
}

func unpatchAllRootScopeLocked() {
	for {
		mocker := nextRootCleanupCandidate()
		if mocker == nil {
			return
		}
		mocker.unPatchLocked()
	}
}

func nextRootCleanupCandidate() mockerInstance {
	mockRegistryMu.Lock()
	defer mockRegistryMu.Unlock()

	for index := len(rootMockScope.order) - 1; index >= 0; index-- {
		mocker := rootMockScope.order[index]
		layers := mockLayersByPatchKey[mocker.layerKey()]
		tool.Assert(len(layers) > 0, "root mock for patch target 0x%x has inconsistent layer metadata", mocker.layerKey())
		if layers[len(layers)-1] == mocker {
			return mocker
		}
	}
	return nil
}

// PatchConvey creates a test context that automatically manages mock lifecycles.
// It wraps around the `convey.Convey` function and adds automatic mock cleanup functionality.
// Context scopes are owned by the goroutine that entered them, so overlapping
// contexts in different goroutines clean up independently. A goroutine without a
// local context joins the innermost context of the sole active owner. With multiple
// active owners, its mocks belong to the shared root scope because ownership is ambiguous.
// Concurrent owners cannot layer mocks on the same target; the second mock is
// rejected before the target is rewritten.
//
// Benefits:
// - No need to manually manage mock cleanup with defer statements
// - Supports nested contexts, where each level only cleans up its own mocks
// - Ensures cleanup between properly nested test cases
//
// Usage examples:
//
// Basic usage:
//
//	PatchConvey("Test case description", t, func() {
//	    // Create mocks here
//	    Mock(someFunction).Return(42).Build()
//	    // Test code that uses the mocks
//	})
//	// All mocks are automatically cleaned up here
//
// Nested usage:
//
//	PatchConvey("Outer test", t, func() {
//	    Mock(outerFunc).Return(1).Build()
//	    PatchConvey("Inner test", func() {
//	        Mock(innerFunc).Return(2).Build()
//	        // Both outerFunc and innerFunc are mocked here
//	    })
//	    // Only innerFunc is cleaned up, outerFunc remains mocked
//	})
//	// All mocks are cleaned up
func PatchConvey(items ...interface{}) {
	for i, item := range items {
		if reflect.TypeOf(item).Kind() == reflect.Func {
			items[i] = reflect.MakeFunc(reflect.TypeOf(item), func(args []reflect.Value) []reflect.Value {
				scope := pushGlobalScope()
				defer popGlobalScope(scope)
				return tool.ReflectCall(reflect.ValueOf(item), args)
			}).Interface()
		}
	}

	convey.Convey(items...)
}

// PatchRun creates a test context that automatically manages mock lifecycles.
//
// Benefits:
// - No need to manually manage mock cleanup with defer statements
// - Supports nested contexts, where each level only cleans up its own mocks
// - More lightweight than PatchConvey when goconvey integration is not needed
//
// Context scopes are owned by the goroutine that entered them, so overlapping
// contexts in different goroutines clean up independently. A goroutine without a
// local context joins the innermost context of the sole active owner. With multiple
// active owners, its mocks belong to the shared root scope because ownership is ambiguous.
// Concurrent owners cannot layer mocks on the same target; the second mock is
// rejected before the target is rewritten.
//
// Usage example:
//
//	PatchRun(func() {
//	    // Create mocks here
//	    Mock(someFunction).Return(42).Build()
//	    // Test code that uses the mocks
//	    result := someFunction() // Returns 42
//	})
//	// All mocks are automatically cleaned up here
//	result := someFunction() // Returns original value
//
// Nested usage example:
//
//	PatchRun(func() {
//	    Mock(functionA).Return("outer").Build()
//
//	    // Inner PatchRun inherits outer mocks but can override them
//	    PatchRun(func() {
//	        Mock(functionB).Return("inner").Build()
//	        Mock(functionA).Return("overridden").Build()
//
//	        resultA := functionA() // Returns "overridden"
//	        resultB := functionB() // Returns "inner"
//	    })
//	    // Inner mocks are cleaned up, outer mocks remain
//
//	    resultA := functionA() // Returns "outer"
//	    resultB := functionB() // Returns original value
//	})
//	// All mocks are cleaned up
//	resultA := functionA() // Returns original value
func PatchRun(f func()) {
	scope := pushGlobalScope()
	defer popGlobalScope(scope)
	f()
}

// UnPatchAll unpatches all mocks in the caller's current `PatchConvey` or `PatchRun` context.
// A caller without a local context unpatches the shared root scope, even while contexts
// owned by other goroutines are active.
// Root mocks shadowed by active child layers are skipped while other safe root mocks
// are cleaned. After the child layer exits, call UnPatchAll again to clean the restored
// root layer. Cleanup of a local context remains strictly LIFO.
//
// For example:
//
//	Test1(t) {
//	    Mock(a).Build()
//		Mock(b).Build()
//
//	    // a and b will be unpatched
//		UnpatchAll()
//	}
//
//	Test2(t) {
//		Mock(a).Build()
//		PatchConvey(t,func(){
//		    Mock(b).Build()
//
//			// only b will be unpatched
//			UnpatchAll()
//		}
//	}
func UnPatchAll() {
	goroutineID := tool.GetGoroutineID()
	mockLifecycleMu.Lock()
	defer mockLifecycleMu.Unlock()
	mockRegistryMu.Lock()
	scope := localMockScopeLocked(goroutineID)
	mockRegistryMu.Unlock()
	if scope == rootMockScope {
		unpatchAllRootScopeLocked()
		return
	}
	unpatchAllScopeLocked(scope)
}
