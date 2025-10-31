# Mockey 

[English](README.md) | 中文

Mockey 是一个简单易用的 Golang 打桩库，可以快速方便地进行函数和变量的 mock。目前， mockey 已广泛应用于字节跳动服务的单元测试编写中（7k+ 仓库）并积极维护。它的底层原理是在运行时重写函数指令，这一点和 [gomonkey](https://github.com/agiledragon/gomonkey) 类似。

Mockey 使得 mock 函数、方法和变量变得容易，不需要将所有依赖指定为接口类型。
> 1. Mockey要求**在编译期间禁用内联和编译优化**，否则无法工作。详见[常见问题](#如何禁用内联和编译优化)。
> 2. 强烈建议在单元测试中与 [Convey](https://github.com/smartystreets/goconvey) 库一起使用。

## 安装
```
go get github.com/bytedance/mockey@latest
```

## 快速上手
```go
package main

import (
	"fmt"
	"math/rand"

	. "github.com/bytedance/mockey"
)

func main() {
	Mock(rand.Int).Return(1).Build() // mock `rand.Int` 返回 1
	
	fmt.Printf("rand.Int() 总是返回: %v\n", rand.Int())
}
```

## 特性
- 函数和方法
  - 基础功能
    - 简单/泛型/可变参数函数或方法（值或指针接收器）
    - 支持钩子函数
    - 支持`PatchConvey`（每个测试用例后自动释放 mock）
    - 提供`GetMethod`处理特殊情况（如未导出类型的导出方法、未导出方法和嵌套结构体中的方法）
  - 高级功能
    - 条件 mock
    - 序列返回
    - 装饰器模式（在 mock 的同时执行原始函数）
    - Goroutine过滤（包含、排除、目标定位）
    - 获取`Mocker`用于高级用法（如获取目标/mock 函数的执行次数）
- mock 变量
  - 普通变量
  - 函数变量

## 兼容性
### 操作系统支持
- Mac OS(Darwin)
- Linux
- Windows

### 架构支持
- AMD64
- ARM64

### 版本支持
- Go 1.13+

## 基础特性
### 简单函数/方法
使用 `Mock` 来 mock 函数/方法，使用 `Return` 来指定返回值：
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
	// mock 函数
	Mock(Foo).Return("MOCKED!").Build()
	fmt.Println(Foo("anything")) // MOCKED!

	// mock 方法（值接收器）
	Mock(A.Foo).Return("MOCKED!").Build()
	fmt.Println(A{}.Foo("anything")) // MOCKED!

	// mock 方法（指针接收器）
	Mock((*B).Foo).Return("MOCKED!").Build()
	fmt.Println(new(B).Foo("anything")) // MOCKED!
    
    // 提示：如果目标函数没有返回值，您仍然需要调用空的 `Return()`。
}
```

### 泛型函数/方法
使用 `MockGeneric` 来 mock 泛型函数/方法：
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
	// mock 泛型函数
	MockGeneric(FooGeneric[string]).Return("MOCKED!").Build() // `Mock(FooGeneric[string], OptGeneric)` 也可以工作
	fmt.Println(FooGeneric("anything"))                       // MOCKED!
	fmt.Println(FooGeneric(1))                                // 1 | 不工作，因为类型不匹配！

	// mock 泛型方法
	MockGeneric((*GenericClass[string]).Foo).Return("MOCKED!").Build()
	fmt.Println(new(GenericClass[string]).Foo("anything")) // MOCKED!
}
```

### 可变参数函数/方法
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
	// mock 可变参数函数
	Mock(FooVariadic).Return("MOCKED!").Build()
	fmt.Println(FooVariadic("anything")) // MOCKED!

	// mock 可变参数方法
	Mock(A.FooVariadic).Return("MOCKED!").Build()
	fmt.Println(A{}.FooVariadic("anything")) // MOCKED!
}
```

### 支持钩子函数
使用 `To` 来指定钩子函数：
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
	// 注意：钩子函数必须与原始函数具有相同的函数签名！
	Mock(Foo).To(func(in string) string { return "MOCKED!" }).Build()
	fmt.Println(Foo("anything")) // MOCKED!

	// 注意：对于方法 mock，可以根据需要将接收器添加到钩子函数的签名中。
	Mock(A.Foo).To(func(a A, in string) string { return a.prefix + ":inner:" + "MOCKED!" }).Build()
	fmt.Println(A{prefix: "prefix"}.Foo("anything")) // prefix:inner:MOCKED!
}
```

### 支持 `PatchConvey`
使用 `PatchConvey` 组织测试用例，就像 `smartystreets/goconvey` 中的 `Convey` 一样。`PatchConvey` 会在每个测试用例后自动释放 mock：
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
		res := Foo("anything")                // 调用
		So(res, ShouldEqual, "MOCKED-1!")     // 断言
	})

	// mock 已释放
	PatchConvey("mock released", t, func() {
		res := Foo("anything")               // 调用
		So(res, ShouldEqual, "ori:anything") // 断言
	})

	// 再次 mock
	PatchConvey("mock 2", t, func() {
		Mock(Foo).Return("MOCKED-2!").Build() // mock
		res := Foo("anything")                // 调用
		So(res, ShouldEqual, "MOCKED-2!")     // 断言
	})
}
```

### 提供 `GetMethod` 处理特殊情况
使用 `GetMethod` 在特殊情况下获取特定方法。

通过实例 mock 方法：
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

	// Mock(a.Foo) 不起作用，因为 `a` 是 `A` 的实例，不是类型 `A`
	Mock(GetMethod(a, "Foo")).Return("MOCKED!").Build()
	fmt.Println(a.Foo("anything")) // MOCKED!
}
```

mock 未导出类型的导出方法：
```go
package main

import (
	"crypto/sha256"
	"fmt"

	. "github.com/bytedance/mockey"
)

func main() {
	// `sha256.New()` 返回未导出的 `*digest`，我们想要 mock 它的 `Sum` 方法
	Mock(GetMethod(sha256.New(), "Sum")).Return([]byte{0}).Build()

	fmt.Println(sha256.New().Sum([]byte("anything"))) // [0]
}
```

mock 未导出方法：
```go
package main

import (
	"bytes"
	"fmt"

	. "github.com/bytedance/mockey"
)

func main() {
	// `*bytes.Buffer` 有一个未导出的 `empty` 方法，这是我们想要 mock 的
	Mock(GetMethod(new(bytes.Buffer), "empty")).Return(true).Build()

	buf := bytes.NewBuffer([]byte{1, 2, 3, 4})
	b, err := buf.ReadByte()
	fmt.Println(b, err)      // 0 EOF | `ReadByte` 在内部调用 `empty` 方法检查缓冲区是否为空并返回 io.EOF
}
```

mock 嵌套结构体中的方法：
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

type Wrapper struct {
	inner // 嵌套结构体
}

type inner struct{}

func (i inner) Foo(in string) string {
	return in
}

func main() {
	// Mock(Wrapper.Foo) 不起作用，因为目标应该是 `inner.Foo`
	Mock(GetMethod(Wrapper{}, "Foo")).Return("MOCKED!").Build() // 或者 Mock(inner.Foo).Return("MOCKED!").Build()

	fmt.Println(Wrapper{}.Foo("anything")) // MOCKED! 
    
	// 提示：`redis.Client` 可以用上述相同的方式 mock
}
```

## 高级特性
### 条件 mock
使用 `When` 定义多个条件：
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
	// 注意：条件函数必须与原始函数具有相同的输入签名！
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

### 序列返回
使用 `Sequence` mock 多个返回值：
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

### 装饰器模式
使用 `Origin` 在 mock 的同时保留目标的原始逻辑：
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
	// `origin` 将携带 `Foo` 函数的逻辑
	origin := Foo

	// `decorator` 将在 `origin` 周围做一些 AOP
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

### Goroutine过滤
使用 `ExcludeCurrentGoRoutine` / `IncludeCurrentGoRoutine` / `FilterGoRoutine` 来确定 mock 在哪个 goroutine 中生效：
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
	// 使用 `ExcludeCurrentGoRoutine` 排除当前 goroutine
	Mock(Foo).ExcludeCurrentGoRoutine().Return("MOCKED!").Build()
	fmt.Println(Foo("anything")) // anything | mock 在当前 goroutine 中不起作用

	go func() {
		fmt.Println(Foo("anything")) // MOCKED! | mock 在其他 goroutine 中起作用
	}()

	time.Sleep(time.Second) // 等待 goroutine 完成
}
```

### 获取 `Mocker`
获取 `Mocker` 以使用高级特性：
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

	// 使用 `MockTimes` 和 `Times` 跟踪 mock 工作次数和 `Foo` 被调用次数
	fmt.Println(mocker.MockTimes()) // 1
	fmt.Println(mocker.Times())     // 2

	// 重新 mock `Foo` 返回 "MOCKED2!"
	mocker.Return("MOCKED2!")
	fmt.Println(Foo("anything")) // MOCKED2!

	// 释放 `Foo` mock
	mocker.Release()
	fmt.Println(Foo("anything")) // anything | mock 不起作用，因为 mock 已释放
}
```

## 常见问题/故障排除
### 如何禁用内联和编译优化？
1. 命令行：测试中使用 `go test -gcflags="all=-l -N" -v ./...` 或主包中使用 `go build -gcflags="all=-l -N"`。
2. Goland：使用 Debug 或在 **Run/Debug Configurations > Go tool arguments** 对话框中填写 `-gcflags="all=-l -N"`。
3. VSCode: 使用 Debug 或在 `settings.json` 中添加 `"go.buildFlags": ["-gcflags=\'all=-N -l\'"]`。

### mock 不起作用？
1. 未禁用内联或编译优化。请检查是否已打印此日志并参考FAQ的[相关部分](#如何禁用内联和编译优化)。
    ```
    Mockey check failed, please add -gcflags="all=-N -l".
    ```
2. 检查是否未调用 `Build()` 方法，或者既没有调用 `Return(xxx)` 也没有调用 `To(xxx)`，这将导致没有实际效果。如果目标函数没有返回值，您仍然需要调用空的 `Return()`。
3. mock 目标不完全匹配，如下所示：
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
    	fmt.Println(A{}.Foo("anything")) // 不起作用，因为 mock 目标应该是 `A.Foo`
    
    	a := A{}
    	Mock(a.Foo).Return("MOCKED!").Build()
    	fmt.Println(a.Foo("anything")) // 不起作用，因为 mock 目标应该是 `A.Foo`
    }
    ```
   如果目标是泛型，请参考[泛型函数/方法](#泛型函数方法)。否则，请尝试检查是否遇到[GetMethod](#提供-getmethod-处理特殊情况)中的特定情况。
4. 目标函数在 mock 释放时在其他goroutine中执行：
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
    		go func() { fmt.Println(Foo("anything")) }() // 执行 'Foo' 的时机不确定
    	})
    	// 当主goroutine来到这里时，相关 mock 已被 'PatchConvey' 释放。如果 'Foo' 在这之前执行， mock 成功，否则失败
    	fmt.Println("over")
    	time.Sleep(time.Second)
    }
    ```

### 错误 "function is too short to patch"？
1. 未禁用内联或编译优化。请检查是否已打印此日志并参考FAQ的[相关部分](#如何禁用内联和编译优化)。
    ```
    Mockey check failed, please add -gcflags="all=-N -l".
    ```
2. 函数确实太短导致编译后的机器码不够长。通常两行或更多行不会导致此问题。如果有这样的需求，可以使用 `MockUnsafe` 来 mock 它，但可能会导致未知问题。
3. 函数已被其他工具（如[monkey](https://github.com/bouk/monkey)等） mock。

### 错误 "re-mock <xxx>, previous mock at: xxx"
函数在最小单元中被重复 mock，如下所示：
```go
package main

import (
	"fmt"

	. "github.com/bytedance/mockey"
)

func Foo() string { return "" }

func main() {
	Mock(Foo).Return("MOCKED!").Build() // 第一次 mock
	Mock(Foo).Return("MOCKED2!").Build() // 第二次 mock，会 panic!
	fmt.Println(Foo())
}
```
请参考[PatchConvey](#支持-patchconvey)来组织您的测试用例。如果有这样的需求，请参考[获取Mocker](#获取-mocker)来重新 mock。

### 不要 mock 系统函数
最好不要直接 mock 系统函数，因为可能会导致概率性崩溃问题。例如，mock  `time.Now` 会导致：
```
fatal error: semacquire not on the G stack
```

## 许可证
Mockey 根据 [Apache License](https://github.com/bytedance/mockey/blob/main/LICENSE-APACHE) 2.0 版本分发。Mockey 的第三方依赖项的许可证在此处[说明](https://github.com/bytedance/mockey/blob/main/licenses)。