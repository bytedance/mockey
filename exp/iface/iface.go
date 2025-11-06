//go:build !windows && go1.18
// +build !windows,go1.18

package iface

import (
	"github.com/bytedance/mockey/internal/iface"
)

// New creates a new implement of the interface T with the given methods.
// NOTE THIS IS AN EXPERIMENTAL FEATURE AND IS NOT GUARANTEED TO STAY THE SAME IN THE FUTURE.
//
// Usage Examples:
//
//	reader := New[io.Reader](map[string]any{
//		"Read": func(p []byte) (n int, err error) { return 0, io.EOF },
//	})
//
// res, err := io.ReadAll(reader)
// fmt.Println(res, err) // Output: [] <nil>
//
// Notes:
// 1. This function uses the Go plugin mechanism and inherits its limitations
// 2. Creating mock instances involves code generation and compilation, which is relatively slow compared to static mock implementations
func New[T any](funcs map[string]any) T {
	return iface.New[T](funcs)
}
