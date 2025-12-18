//go:build go1.25 && !go1.26
// +build go1.25,!go1.26

package tool

// gGoroutineIDOffset Go1.25 removed the `gobuf.ret` field before goid
const gGoroutineIDOffset = 152
