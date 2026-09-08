# Go 1.27 compatibility

Go 1.27 is supported by the core `mockey` package on Linux and macOS AMD64/ARM64, and Windows AMD64. Tests must disable inlining and optimization:

```sh
go test -gcflags="all=-N -l" ./...
go test -race -gcflags="all=-N -l" ./...
```

The experimental `exp/iface` package remains excluded on Go 1.26 and later. Its receiver metadata adaptation is separate from core Go 1.27 support.

## Generic methods

Go 1.27 adds methods with their own type parameters. Applications declaring these methods must enable the Go 1.27 language version in their `go.mod`. Mockey retains its existing module directive and older-toolchain compatibility.

```go
type Service struct{}

func (s *Service) Echo[T any](value T) T { return value }

func TestEcho(t *testing.T) {
    service := &Service{}
    mock := mockey.Mock((*Service).Echo[int]).Return(42).Build()
    defer mock.UnPatch()
    if got := service.Echo[int](1); got != 42 {
        t.Fatalf("got %d, want 42", got)
    }
}
```

Supported forms include declared value/pointer method expressions, generic receiver types, and bound pointer method values such as `service.Echo[int]`. Hooks, variadic parameters, `Origin`, and distinct instantiations sharing a compiler shape are covered by regression tests. A bound pointer-method patch affects the selected instantiation across receivers; its hook uses the receiver-free signature.

Bound value generic method values and promoted generic method expressions fail before patching. Use the declaring receiver's method expression instead, such as `Service.Echo[int]` or `(*Inner).Echo[int]`. Ordinary user closures retain their existing behavior. Generic interface methods are not part of Go 1.27.

## Runtime adaptation

Version-specific runtime layouts and metadata adapters stop at Go 1.28, which requires a new compatibility audit. The implementation follows [runtime layouts](https://github.com/golang/go/blob/go1.27.0/src/runtime/runtime2.go), [function metadata](https://github.com/golang/go/blob/go1.27.0/src/runtime/symtab.go), and [compiler wrapper generation](https://github.com/golang/go/blob/go1.27.0/src/cmd/compile/internal/noder/reader.go).

Race helper lookup handles duplicate ABI entries. Installed patches retain their hook values for garbage collection. Windows runtime sleeps use the Go register calling convention.

Original-function trampolines relocate relative control flow and address generation. A stack-growth retry resumes the active original proxy without repeating conditions, decorators, or counters. Ordinary recursion and calls through older generic patches retain their mocking behavior. This requires tracking active original calls per goroutine; benchmarks cover serial and parallel `Origin` calls.

Distant AMD64 RIP-relative `LEA` instructions use local address literals without changing register width or flags. EIP-relative address generation, distant segment-prefixed `LEA`, and other unsupported distant data references fail explicitly before the target is patched.

## Validation

CI covers Go 1.27 normal/race execution on the five platform combinations above, older Go regression jobs, and runtime-suspension-disabled builds. Additional tests cover generic dictionaries, the standard library `math/rand/v2.Rand.N` method, hook lifetime, forced stack growth, nested patches, and instruction relocation.

Run call-path benchmarks with the same compiler flags:

```sh
go test -gcflags="all=-N -l" -run='^$' -bench=. -benchmem ./...
```
