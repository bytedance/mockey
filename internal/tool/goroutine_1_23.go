//go:build go1.23 && !go1.25
// +build go1.23,!go1.25

package tool

// gGoroutineIDOffset Go1.23 introduced the `syscallbp` field before goid
const gGoroutineIDOffset = 160
