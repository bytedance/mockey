# Mockey

English | [中文](README_cn.md)

Mockey is a simple and easy-to-use golang mock library, which can quickly and conveniently mock functions and variables. At present, it is widely used in the unit test writing of ByteDance services (7k+ repos) and is actively maintained. In essence it rewrites function instructions at runtime similarly to [gomonkey](https://github.com/agiledragon/gomonkey).

Mockey makes it easy to replace functions, methods and variables with mocks reducing the need to specify all dependencies as interfaces.

## Features
- Mock functions and methods
    - Basic 
      - Common / variant parameter function 
      - Common / variant parameter method (value or pointer receiver)
      - Nested structure method
      - Fixed value or dynamic mocking (via custom defined hook functions)
      - Export method of private type (under different packages)
      - Conditional mocking (via custom defined hook functions)
    - Advanced
      - Execute the original function after mocking (decorator pattern)
      - Goroutine conditional filtering (inclusion, exclusion, targeting)
      - Incrementally change mock behavior - sequence support
      - Get the execution times of target function
      - Get the execution times of mock function
- Mock variable
  - Common variable
  - Function variable

## Requirements

> 1. Mockey requires **inlining and compilation optimization to be disabled** during compilation or it won't run. See the [FAQs](#FAQ) for details.
> 2. It is strongly recommended to use it together with the [Convey](https://github.com/smartystreets/goconvey) library (module dependency)

## Install
```
go get github.com/bytedance/mockey@latest
```

## Quick Guide
```go
import (
	"fmt"
	"testing"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// Simple function
func Foo(in string) string {
	return in
}

// Function method (value receiver)
type A struct{}
func (a A) Foo(in string) string { return in }

// Function method (pointer receiver)
type B struct{}
func (b *B) Foo(in string) string { return in }

// Value
var Bar = 0

func TestMockXXX(t *testing.T) {

	PatchConvey("Function mocking", t, func() {
		Mock(Foo).Return("c").Build()          // mock function
		So(Foo("anything"), ShouldEqual, "c")  // assert `Foo` is mocked
	})

   PatchConvey("Method mocking (value receiver)", t, func() {
		Mock(A.Foo).Return("c").Build()              // mock method
		So(new(A).Foo("anything"), ShouldEqual, "c") // assert `A.Foo` is mocked
	})

   PatchConvey("Method mocking (pointer receiver)", t, func() {
		Mock((*B).Foo).Return("c").Build()        // mock method
      b := &B{}
		So(b.Foo("anything"), ShouldEqual, "c")   // assert `*B.Foo` is mocked
	})

   PatchConvey("Variable mocking", t, func() {
		MockValue(&Bar).To(1)            // mock variable
		So(Bar, ShouldEqual, 1)          // assert `Bar` is mocked
	})
	
   // the mocks are released automatically outside `PatchConvey`
   fmt.Println(Foo("a"))        // a
   fmt.Println(new(A).Foo("b")) // b
   fmt.Println(Bar)             // 0
}
```

## Compatibility
### OS Support
- Mac OS(Darwin)
- Linux
- Windows
### Arch Support 
- AMD64
- ARM64 (mac/linux only)
### Version Support 
- Go 1.13+

## Advanced features

### Conditional mocking

```go
PatchConvey("conditional mocking", t , func() {
   mock := Mock((*B).Fun).
      When(func(in string) bool { return len(in) > 10 }) // Same parameters as the mocked function parameter. The args are used to determine whether to execute the mock or not
      Return("MOCKED!").
      Build()

   // OR
   mock := Mock((*B).Fun).
      When(func(b *B, in string) bool { return len(in) > 10 }) // optionally the class reference can be passed as the first parameter
      Return("MOCKED!").
      Build()

   // we initialise
   dep := &B{}

   // long string -> mocked
   So(dep.Fun("something long enough"), ShouldEqual, "MOCKED!")

   // short string -> no mock
   So(dep.Fun("Imtiny"), ShouldEqual, "Imtiny")

   // how many invocations?
   convey.So(mock.Times(), convey.ShouldEqual, 2)     // # of function invocations (mocked or not)
   convey.So(mock.MockTimes(), convey.ShouldEqual, 1) // # of mocked invocations only
})
```

### Sequence support

```go
PatchConvey("sequence support", t , func() {
   // # mockey v1.2.4+
   mock := Mock(Fun).Return(Sequence("Alice").Times(3).Then("Bob").Then("Tom").Times(2)).Build()

   r := Fun("something")
   convey.So(r, convey.ShouldEqual, "Alice")

   r = Fun("something")
   convey.So(r, convey.ShouldEqual, "Alice")

   r = Fun("something")
   convey.So(r, convey.ShouldEqual, "Alice")

   r = Fun("something")
   convey.So(r, convey.ShouldEqual, "Bob")

   r = Fun("something")
   convey.So(r, convey.ShouldEqual, "Tom")

   ...

   convey.So(mock.Times(), convey.ShouldEqual, 5)
   convey.So(mock.MockTimes(), convey.ShouldEqual, 5)
})
```

### Generics support

```go
func GenericFun[T any](t T) T {
	return t
}

type GenericClass[T any] struct {
}

func (g *GenericClass[T]) FunA(t T) T {
	return t
}

...

PatchConvey("Generics function support", t, func() {

   mock := MockGeneric(GenericFun[string]).Return("MOCKED!").Build()
   // or Mock(GenericFun[string], OptGeneric)

   r := GenericFun("anything")   // mocked
   num := GenericFun(2)          // not mocked - type mismatch

   convey.So(r, convey.ShouldEqual, "MOCKED!")
   convey.So(mock.Times(), convey.ShouldEqual, 1)

   convey.So(num, convey.ShouldEqual, 2)
})

PatchConvey("Generics class/method support", t, func() {

   mock := MockGeneric((*GenericClass[string]).FunA).Return("MOCKED!").Build()
   // or Mock((*GenericClass[string]).FunA, OptGeneric)

   genClass := &GenericClass[string]{}

   r := genClass.FunA("anything")

   convey.So(r, convey.ShouldEqual, "MOCKED!")
   convey.So(mock.Times(), convey.ShouldEqual, 1)
})

```

### Decorator mock
```go
PatchConvey("decorator mock", t , func() {
   origin := Fun

   mockFun := func(p string) string {
      origin(p)  // call the original func
      return "b" // return something else
   }

   mock := Mock(Fun).To(mockFun).Origin(&origin).Build()

   r := Fun("anything")
   convey.So(r, convey.ShouldEqual, "b")
   convey.So(mock.Times(), convey.ShouldEqual, 1)
})
```



### Goroutine inclusion/exclusion support
```go
PatchConvey("filter go routine", t , func() {
   mock := Mock(Fun).ExcludeCurrentGoRoutine().Return("MOCKED!").Build()
   r := Fun("anything")
   convey.So(r, convey.ShouldEqual, "anything")
   convey.So(mock.Times(), convey.ShouldEqual, 1)
   convey.So(mock.MockTimes(), convey.ShouldEqual, 0)

   mock.IncludeCurrentGoRoutine()
   r = Fun("anything")
   convey.So(r, convey.ShouldEqual, "MOCKED!")
   convey.So(mock.Times(), convey.ShouldEqual, 1)
   convey.So(mock.MockTimes(), convey.ShouldEqual, 1)

   mock.IncludeCurrentGoRoutine()
   go Fun("anything")
   time.Sleep(1 * time.Second)
   convey.So(mock.Times(), convey.ShouldEqual, 1)
   convey.So(mock.MockTimes(), convey.ShouldEqual, 0)
})
```

PS: It's also possible to specify the goroutine where the mocking takes effect using [mock.FilterGoRoutine](https://github.com/bytedance/mockey/blob/07cb384c4d3b0374d9a559572084ec33f549c96c/mock.go#L208)

## FAQ/troubleshooting

### Go 1.23 compile error `"link: github.com/bytedance/mockey/internal/monkey/common: invalid reference to runtime.sysAllocOS"`?
add build flag  `-ldflags=-checklinkname=0`

### How to disable inline and compile optimization?
1. Command line：`go test -gcflags="all=-l -N" -v ./...`
2. Goland：fill `-gcflags="all=-l -N"` in the **Run/Debug Configurations > Go tool arguments** dialog box

### The original function is still entered after mocking?
1. Inline or compilation optimization is not disabled: you can try to use the debug mode. If you can run through it, it means that it is the problem. Please go to the [relevant section](#how-to-disable-inline-and-compile-optimization) of FAQ
3. The `Build()` method was not called: forgot to call `Build()`, resulting in no actual effect
4. Target function does not match exactly:
```go
func TestXXX(t *testing.T) {
   Mock((*A).Foo).Return("c").Build()
   fmt.Println(A{}.Foo("a")) // enters the original function, because the mock target should be `A.Foo`

   a := A{}
   Mock(a.Foo).Return("c").Build()
   fmt.Println(a.Foo("a")) // enters the original function, because the mock target should be `A.Foo` or extracted from instance `a` using `GetMethod`
}
```
4. The target function is executed in other goroutines：
```go
func TestXXX(t *testing.T) {
   PatchConvey("TestXXX", t, func() {
      Mock(Foo).Return("c").Build()
      go Foo("a") // the timing of executing 'foo' is uncertain
   })
   // when the main goroutine comes here, the relevant mock has been released by 'PatchConvey'. If 'foo' is executed before this, the mock succeeds, otherwise it fails
   fmt.Println("over")
   time.Sleep(time.Second)
}
```

### Error "function is too short to patch"？
1. Inline or compilation optimization is not disabled: you can try to use the debug mode. If you can run through it, it means that it is the problem. Please go to the [relevant section](#how-to-disable-inline-and-compile-optimization) of FAQ
2. The function is really too short: it means that the target function is less than one line, resulting in the compiled machine code being too short. Generally, two or more lines will not cause this problem
3. Repeat mocking the same function: repeat mocking the same function in the `PatchConvey` of the smallest unit. If there is such a need, please get the `Mocker` instance and remock.
4. Other tools mock this function: for example, [monkey](https://github.com/bouk/monkey) or other tools have mocked this function

## License
Mockey is distributed under the [Apache License](https://github.com/bytedance/mockey/blob/main/LICENSE-APACHE), version 2.0. The licenses of third party dependencies of Mockey are explained [here](https://github.com/bytedance/mockey/blob/main/licenses).
