//go:build go1.25
// +build go1.25

package tool

// gGoroutineIDOffset Go1.25 removed the `gobuf.ret` field before goid
const gGoroutineIDOffset = 152
