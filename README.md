# Mockey

English | [中文](README_cn.md)

Mockey is a simple and easy-to-use golang mock tool library, which can quickly and conveniently mock functions and variables. At present, it is widely used in the unit test writing of ByteDance services. The bottom layer is monkey patch realized by rewriting function instructions at runtime.

> 1. **inlining and compilation optimization need to be disabled** during compilation, otherwise mock may fail or an error may be reported. See the following [FAQ](#FAQ) chapter for details.
> 2. In the actual process of writing a unit test, it is recommended to use it together with the [Convey](https://github.com/smartystreets/goconvey) library.

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

func Foo(in string) string {
	return in
}

type A struct{}

func (a A) Foo(in string) string { return in }

var Bar = 0

func TestMockXXX(t *testing.T) {
	PatchConvey("TestMockXXX", t, func() {
		Mock(Foo).Return("c").Build()   // mock function
		Mock(A.Foo).Return("c").Build() // mock method
		MockValue(&Bar).To(1)           // mock variable

		So(Foo("a"), ShouldEqual, "c")        // assert `Foo` is mocked
		So(new(A).Foo("b"), ShouldEqual, "c") // assert `A.Foo` is mocked
		So(Bar, ShouldEqual, 1)               // assert `Bar` is mocked
	})
	// mock is released automatically outside `PatchConvey`
	fmt.Println(Foo("a"))        // a
	fmt.Println(new(A).Foo("b")) // b
	fmt.Println(Bar)             // 0
}
```
## Features
- Mock functions and methods
    - Basic 
      - Common / variant parameter function 
      - Common / variant parameter method
      - Nested structure method
      - Export method of private type (under different packages)
    - Advanced
      - Execute the original function after mocking 
      - Goroutine conditional filtering
      - Incrementally change mock behavior
      - Get the execution times of target function
      - Get the execution times of mock function
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

## License
Mockey is distributed under the [Apache License](https://github.com/bytedance/mockey/blob/main/LICENSE), version 2.0. The licenses of third party dependencies of Mockey are explained [here](https://github.com/bytedance/mockey/blob/main/licenses).

## FAQ

### How to disable inline and compile optimization?
1. Command line：`go test -gcflags="all=-l -N" -v ./...`
2. Goland：fill `-gcflags="all=-l -N"` in the **Run/Debug Configurations > Go tool arguments** dialog box

### The original function is still entered after mocking?
1. Inline or compilation optimization is not disabled: you can try to use the debug mode. If you can run through it, it means that it is the problem. Please go to the [relevant section](#How to disable inline and compile optimization?) of FAQ
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
1. Inline or compilation optimization is not disabled: you can try to use the debug mode. If you can run through it, it means that it is the problem. Please go to the [relevant section](#How to disable inline and compile optimization?) of FAQ
2. The function is really too short: it means that the target function is less than one line, resulting in the compiled machine code being too short. Generally, two or more lines will not cause this problem
3. Repeat mocking the same function: repeat mocking the same function in the `PatchConvey` of the smallest unit. If there is such a need, please get the `Mocker` instance and remock.
4. Other tools mock this function: for example, [monkey](https://github.com/bouk/monkey) or other tools have mocked this function
