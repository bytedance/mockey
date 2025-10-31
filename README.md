# Mockey
English | [中文](README_cn.md)

[![Release](https://img.shields.io/github/v/release/bytedance/mockey)](https://github.com/bytedance/mockey/releases)
[![License](https://img.shields.io/github/license/bytedance/mockey)](https://github.com/bytedance/mockey/blob/main/LICENSE-APACHE)
[![Go Report Card](https://goreportcard.com/badge/github.com/bytedance/mockey)](https://goreportcard.com/report/github.com/bytedance/mockey)
[![OpenIssue](https://img.shields.io/github/issues/bytedance/mockey)](https://github.com/bytedance/mockey/issues)
[![ClosedIssue](https://img.shields.io/github/issues-closed/bytedance/mockey)](https://github.com/bytedance/mockey/issues?q=is%3Aissue+is%3Aclosed)

Mockey is a simple and easy-to-use golang mock library, which can quickly and conveniently mock functions and variables. At present, it is widely used in the unit test writing of ByteDance services (7k+ repos) and is actively maintained. In essence it rewrites function instructions at runtime similarly to [gomonkey](https://github.com/agiledragon/gomonkey).

Mockey makes it easy to replace functions, methods and variables with mocks reducing the need to specify all dependencies as interfaces.
> 1. Mockey requires **inlining and compilation optimization to be disabled** during compilation, or it won't work. See the [FAQs](#how-to-disable-inline-and-compile-optimization) for details.
> 2. It is strongly recommended to use it together with the [Convey](https://github.com/smartystreets/goconvey) library (module dependency) in unit tests.

## Install
```
go get github.com/bytedance/mockey@latest
```

## Quick Guide
```go
package main

import (
	"fmt"
	"math/rand"

	. "github.com/bytedance/mockey"
)

func main() {
	Mock(rand.Int).Return(1).Build() // mock `rand.Int` to return 1
	
	fmt.Printf("rand.Int() always return: %v\n", rand.Int())
}
```

## Features
- Mock functions and methods
    - Basic
        - Simple / generic / variadic function or method (value or pointer receiver)
        - Supporting hook function
        - Supporting `PatchConvey` (automatically release mocks after each test case)
        - Providing `GetMethod` to handle special cases (e.g., exported method of unexported types, unexported method, and methods in nested structs)
    - Advanced
        - Conditional mocking
        - Sequence returning
        - Decorator pattern (execute the original function after mocking)
        - Goroutine filtering (inclusion, exclusion, targeting)
        - Acquire `Mocker` for advanced usage (e.g., getting the execution times of target/mock function)
- Mock variable
    - Common variable
    - Function variable

## Compatibility
### OS Support
- Mac OS(Darwin)
- Linux
- Windows

### Arch Support 
- AMD64
- ARM64

### Version Support 
- Go 1.13+

## Basic Features
### Simple function/method
Use `Mock` to mock function/method, and use `Return` to specify the return value:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return in
}

type A struct{}

func (a A) Foo(in string) string { return in }

type B struct{}

func (b *B) Foo(in string) string { return in }

func main() {
	// mock function
	Mock(Foo).Return("MOCKED!").Build()
	fmt.Println(Foo("anything")) // MOCKED!

	// mock method (value receiver)
	Mock(A.Foo).Return("MOCKED!").Build()
	fmt.Println(A{}.Foo("anything")) // MOCKED!

	// mock method (pointer receiver)
	Mock((*B).Foo).Return("MOCKED!").Build()
	fmt.Println(new(B).Foo("anything")) // MOCKED!
    
    // Tips: if the target has no return value, you still need to call the empty `Return()`.
}
```

### Generic function/method
Use `MockGeneric` to mock generic function/method:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func FooGeneric[T any](t T) T {
	return t
}

type GenericClass[T any] struct {
}

func (g *GenericClass[T]) Foo(t T) T {
	return t
}

func main() {
	// mock generic function 
	MockGeneric(FooGeneric[string]).Return("MOCKED!").Build() // `Mock(FooGeneric[string], OptGeneric)` also works
	fmt.Println(FooGeneric("anything"))                       // MOCKED!
	fmt.Println(FooGeneric(1))                                // 1 | Not working because of type mismatch!

	// mock generic method 
	MockGeneric((*GenericClass[string]).Foo).Return("MOCKED!").Build()
	fmt.Println(new(GenericClass[string]).Foo("anything")) // MOCKED!
}
```

### Variadic function/method
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func FooVariadic(in ...string) string {
	return in[0]
}

type A struct{}

func (a A) FooVariadic(in ...string) string { return in[0] }

func main() {
	// mock variadic function
	Mock(FooVariadic).Return("MOCKED!").Build()
	fmt.Println(FooVariadic("anything")) // MOCKED!

	// mock variadic method
	Mock(A.FooVariadic).Return("MOCKED!").Build()
	fmt.Println(A{}.FooVariadic("anything")) // MOCKED!
}
```

### Supporting hook function
Use `To` to specify the hook function:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return in
}

type A struct {
	prefix string
}

func (a A) Foo(in string) string { return a.prefix + ":" + in }

func main() {
	// NOTE: hook function must have the same function signature as the original function!
	Mock(Foo).To(func(in string) string { return "MOCKED!" }).Build()
	fmt.Println(Foo("anything")) // MOCKED!

	// NOTE: for method mocking, the receiver can be added to the signature of the hook function on your need.
	Mock(A.Foo).To(func(a A, in string) string { return a.prefix + ":inner:" + "MOCKED!" }).Build()
	fmt.Println(A{prefix: "prefix"}.Foo("anything")) // prefix:inner:MOCKED!
}
```

### Supporting `PatchConvey`
Use `PatchConvey` to organize test cases, just like `Convey` in `smartystreets/goconvey`. `PatchConvey` will automatically release the mocks after each test case:
```go
package main_test

import (
	"testing"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func Foo(in string) string {
	return "ori:" + in
}

func TestXXX(t *testing.T) {
    // mock 
	PatchConvey("mock 1", t, func() {
		Mock(Foo).Return("MOCKED-1!").Build() // mock 
		res := Foo("anything")                // invoke
		So(res, ShouldEqual, "MOCKED-1!")     // assert
	})

	// mock released
	PatchConvey("mock released", t, func() {
		res := Foo("anything")               // invoke
		So(res, ShouldEqual, "ori:anything") // assert
	})

	// mock again
	PatchConvey("mock 2", t, func() {
		Mock(Foo).Return("MOCKED-2!").Build() // mock 
		res := Foo("anything")                // invoke
		So(res, ShouldEqual, "MOCKED-2!")     // assert
	})
}
```

### Providing `GetMethod` to handle special cases
Use `GetMethod` to get the specific method in special cases.

Mock method through an instance:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

type A struct{}

func (a A) Foo(in string) string { return in }

func main() {
	a := new(A)

	// Mock(a.Foo) won't work, because `a` is an instance of `A`, not the type `A`
	Mock(GetMethod(a, "Foo")).Return("MOCKED!").Build()
	fmt.Println(a.Foo("anything")) // MOCKED!
}
```

Mock exported method of unexported types:
```go
package main

import (
	"crypto/sha256"
	"fmt"

	. "github.com/bytedance/mockey"
)

func main() {
	// `sha256.New()` returns an unexported `*digest`, whose `Sum` method is the one we want to mock
	Mock(GetMethod(sha256.New(), "Sum")).Return([]byte{0}).Build()

	fmt.Println(sha256.New().Sum([]byte("anything"))) // [0]
}
```

Mock unexported method:
```go
package main

import (
	"bytes"
	"fmt"

	. "github.com/bytedance/mockey"
)

func main() {
	// `*bytes.Buffer` has an unexported `empty` method, which is the one we want to mock
	Mock(GetMethod(new(bytes.Buffer), "empty")).Return(true).Build()

	buf := bytes.NewBuffer([]byte{1, 2, 3, 4})
	b, err := buf.ReadByte()
	fmt.Println(b, err)      // 0 EOF | `ReadByte` calls `empty` method inside to check if the buffer is empty and return io.EOF
}
```

Mock methods in nested structs:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

type Wrapper struct {
	inner // nested struct
}

type inner struct{}

func (i inner) Foo(in string) string {
	return in
}

func main() {
	// Mock(Wrapper.Foo) won't work, because the target should be `inner.Foo`
	Mock(GetMethod(Wrapper{}, "Foo")).Return("MOCKED!").Build() // or Mock(inner.Foo).Return("MOCKED!").Build()

	fmt.Println(Wrapper{}.Foo("anything")) // MOCKED! 
    
	// Tips: `redis.Client` can be mocked in the same way as above
}
```

## Advanced features
### Conditional mocking
Use `When` to define multiple conditions:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return "ori:" + in
}

func main() {
	// NOTE: condition function must have the same input signature as the original function!
	Mock(Foo).
		When(func(in string) bool { return len(in) == 0 }).Return("EMPTY").
		When(func(in string) bool { return len(in) <= 2 }).Return("SHORT").
		When(func(in string) bool { return len(in) <= 5 }).Return("MEDIUM").
		Build()

	fmt.Println(Foo(""))            // EMPTY
	fmt.Println(Foo("h"))           // SHORT
	fmt.Println(Foo("hello"))       // MEDIUM
	fmt.Println(Foo("hello world")) // ori:hello world
}
```

### Sequence returning
Use `Sequence` to mock multiple return values:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return in
}

func main() {
	Mock(Foo).Return(Sequence("Alice").Then("Bob").Times(2).Then("Tom")).Build()
	fmt.Println(Foo("anything")) // Alice
	fmt.Println(Foo("anything")) // Bob
	fmt.Println(Foo("anything")) // Bob
	fmt.Println(Foo("anything")) // Tom
}
```

### Decorator pattern
Use `Origin` to keep the original logic of the target after mock:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return "ori:" + in
}

func main() {
	// `origin` will carry the logic of the `Foo` function
	origin := Foo

	// `decorator` will do some AOP around `origin`
	decorator := func(in string) string {
		fmt.Println("arg is", in)
		out := origin(in)
		fmt.Println("res is", out)
		return out
	}

	Mock(Foo).Origin(&origin).To(decorator).Build()

	fmt.Println(Foo("anything"))
	/*
		arg is anything
		res is ori:anything
		ori:anything
	*/
}
```

### Goroutine filtering
Use `ExcludeCurrentGoRoutine` / `IncludeCurrentGoRoutine` / `FilterGoRoutine` to determine in which goroutine the mock takes effect:
```go
package main

import (
	"fmt"
	"time"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return in
}

func main() {
	// use `ExcludeCurrentGoRoutine` to exclude current goroutine
	Mock(Foo).ExcludeCurrentGoRoutine().Return("MOCKED!").Build()
	fmt.Println(Foo("anything")) // anything | mock won't work in current goroutine

	go func() {
		fmt.Println(Foo("anything")) // MOCKED! | mock works in other goroutines
	}()

	time.Sleep(time.Second) // wait for goroutine to finish
}
```

### Acquire `Mocker`
Acquire `Mocker` to use advanced features:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo(in string) string {
	return in
}

func main() {
	mocker := Mock(Foo).When(func(in string) bool { return len(in) > 5 }).Return("MOCKED!").Build()
	fmt.Println(Foo("any"))      // any
	fmt.Println(Foo("anything")) // MOCKED!

	// use `MockTimes` and `Times` to track the number of times mock worked and `Foo` is called
	fmt.Println(mocker.MockTimes()) // 1
	fmt.Println(mocker.Times())     // 2

	// remock `Foo` to return "MOCKED2!"
	mocker.Return("MOCKED2!")
	fmt.Println(Foo("anything")) // MOCKED2!

	// release `Foo` mock
	mocker.Release()
	fmt.Println(Foo("anything")) // anything | mock won't work, because mock released
}
```

## FAQ/troubleshooting
### How to disable inline and compile optimization?
1. Command line：`go test -gcflags="all=-l -N" -v ./...` in tests or `go build -gcflags="all=-l -N"` in main packages.
2. Goland：use Debug or fill `-gcflags="all=-l -N"` in the **Run/Debug Configurations > Go tool arguments** dialog box.
3. VSCode: use Debug or add `"go.buildFlags": ["-gcflags=\'all=-N -l\'"]` in `settings.json`.

### Mock doesn't work?
1. Inline or compilation optimizations are not disabled. Please check if this log has been printed and refer to [relevant section](#how-to-disable-inline-and-compile-optimization) of FAQ.
    ```
    Mockey check failed, please add -gcflags="all=-N -l".
    ```
2. Check if `Build()` was not called, or that neither `Return(xxx)` nor `To(xxx)` was called, resulting in no actual effect. If the target function has no return value, you still need to call the empty `Return()`.
3. Mock targets do not match exactly, as below:
    ```go
    package main_test
    
    import (
    	"fmt"
    	"testing"
    
    	. "github.com/bytedance/mockey"
    )
    
    type A struct{}
    
    func (a A) Foo(in string) string { return in }
    
    func TestXXX(t *testing.T) {
    	Mock((*A).Foo).Return("MOCKED!").Build()
    	fmt.Println(A{}.Foo("anything")) // won't work, because the mock target should be `A.Foo`
    
    	a := A{}
    	Mock(a.Foo).Return("MOCKED!").Build()
    	fmt.Println(a.Foo("anything")) // won't work, because the mock target should be `A.Foo`
    }
    ```
   Please refer to [Generic function/method](#generic-functionmethod) if the target is generic. Otherwise, try to check if it hits specific cases in [GetMethod](#providing-getmethod-to-handle-special-cases).
4. The target function is executed in other goroutines when mock released:
    ```go
    package main_test
    
    import (
    	"fmt"
    	"testing"
    	"time"
    
    	. "github.com/bytedance/mockey"
    )
    
    func Foo(in string) string {
    	return in
    }
    
    func TestXXX(t *testing.T) {
    	PatchConvey("TestXXX", t, func() {
    		Mock(Foo).Return("MOCKED!").Build()
    		go func() { fmt.Println(Foo("anything")) }() // the timing of executing 'Foo' is uncertain
    	})
    	// when the main goroutine comes here, the relevant mock has been released by 'PatchConvey'. If 'Foo' is executed before this, the mock succeeds, otherwise it fails
    	fmt.Println("over")
    	time.Sleep(time.Second)
    }
    ```

### Error "function is too short to patch"？
1. Inline or compilation optimizations are not disabled. Please check if this log has been printed and refer to [relevant section](#how-to-disable-inline-and-compile-optimization) of FAQ.
    ```
    Mockey check failed, please add -gcflags="all=-N -l".
    ```
2. The function is really too short resulting in the compiled machine code is not long enough. Generally, two or more lines will not cause this problem. If there is such a need, you may use `MockUnsafe` to mock it causing unknown issues.
3. The function has been mocked by other tools (such as [monkey](https://github.com/bouk/monkey) etc.)

### Error "re-mock <xxx>, previous mock at: xxx"
The function has been mocked repeatedly in the smallest unit, as below:
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo() string { return "" }

func main() {
	Mock(Foo).Return("MOCKED!").Build() // mock for the first time
	Mock(Foo).Return("MOCKED2!").Build() // mock for the second time, will panic!
	fmt.Println(Foo())
}
```
Please refer to [PatchConvey](#supporting-patchconvey) to organize your test cases. If there is such a need, please refer to [Acquire Mocker](#acquire-mocker) to remock.

### Don't mock system functions
It is best not to directly mock system functions, as it may cause probabilistic crash issues. For example, mock `time.Now` will cause:
```
fatal error: semacquire not on the G stack
```

## License
Mockey is distributed under the [Apache License](https://github.com/bytedance/mockey/blob/main/LICENSE-APACHE), version 2.0. The licenses of third party dependencies of Mockey are explained [here](https://github.com/bytedance/mockey/blob/main/licenses).
