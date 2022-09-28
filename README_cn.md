# Mockey 

[English](README.md) | 中文

Mockey 是一款简单易用的 Golang 打桩工具库，能够快速方便地进行函数、变量的 mock，目前在字节跳动各业务的单元测试编写中应用较为广泛，其底层是通过运行时改写函数指令实现的猴子补丁（Monkey Patch）

> 1. 编译时需要**禁用内联和编译优化**，否则可能会 mock 失败或者报错，详见下面的 [FAQ](#FAQ) 章节
> 2. 实际编写单测的过程中，建议结合 [Convey](https://github.com/smartystreets/goconvey) 库一起使用

## 安装
```
go get github.com/bytedance/mockey@latest
```

## 快速上手
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
		Mock(Foo).Return("c").Build()   // mock函数 
		Mock(A.Foo).Return("c").Build() // mock方法 
		MockValue(&Bar).To(1)           // mock变量 

		So(Foo("a"), ShouldEqual, "c")        // 断言`Foo`成功mock 
		So(new(A).Foo("b"), ShouldEqual, "c") // 断言`A.Foo`成功mock 
		So(Bar, ShouldEqual, 1)               // 断言`Bar`成功mock 
	})
	// `PatchConvey`外自动释放mock
	fmt.Println(Foo("a"))        // a
	fmt.Println(new(A).Foo("b")) // b
	fmt.Println(Bar)             // 0
}
```
## 功能概览
- mock 函数和方法
  - 基础功能
    - 普通/可变参数函数
    - 普通/可变参数方法
    - 嵌套结构体方法
    - 私有类型的导出方法（不同包下）
  - 高级功能
    - mock 后执行原函数
    - goroutine 条件过滤
    - 增量改变 mock 行为
    - 获取原函数执行次数
    - 获取 mock 函数执行次数
- mock 变量
  - 普通变量
  - 函数变量
## 兼容性
### 平台支持
- Mac OS(Darwin)
- Linux
- Windows
### 架构支持
- AMD64
- ARM64
### 版本支持
- Go 1.13+

## 开源许可

Mockey 基于[Apache License 2.0](https://github.com/bytedance/mockey/blob/main/LICENSE-APACHE) 许可证，其依赖的三方组件的开源许可见 [Licenses](https://github.com/bytedance/mockey/blob/main/licenses)

## FAQ

### 如何禁用内联和编译优化？
1. 命令行：`go test -gcflags="all=-l -N" -v ./...`
2. Goland：在 **运行/调试配置 > Go工具实参** 对话框中填写 `-gcflags="all=-l -N"` 

### Mock 后函数后仍走入了原函数？
1. 未禁用内联或者编译优化：可以尝试使用 debug 模式，如果能跑过则说明是该问题，请转到 FAQ [相关章节](#如何禁用内联和编译优化)
2. 未调用`Build()`方法：mock 函数时漏写了`.Build()`，导致没有任何实际效果产生
3. 目标函数不完全匹配：
```go
func TestXXX(t *testing.T) {
Mock((*A).Foo).Return("c").Build()
fmt.Println(A{}.Foo("a")) // 走入原函数，目标函数应该是A.Foo

a := A{}
Mock(a.Foo).Return("c").Build()
fmt.Println(a.Foo("a")) // 走入原函数，目标函数应该是A.Foo或者使用工具函数GetMethod从a中获取 
}
```
4. 目标函数在其他协程里执行：
```go
func TestXXX(t *testing.T) {
   PatchConvey("TestXXX", t, func() {
      Mock(Foo).Return("c").Build()
      go Foo("a") // 执行Foo的时机不定
   })
   // 主协程走到这里时相关mock已经被PatchConvey释放，如果Foo先于此执行则mock成功，否则失败
   fmt.Println("over")
   time.Sleep(time.Second)
}
```

### 报错 "function is too short to patch"？
1. 未禁用内联或者编译优化：可以尝试使用 debug 模式，如果能跑过则说明是该问题，请转到 FAQ [相关章节](#如何禁用内联和编译优化)
2. 函数确实太短：指目标函数小于一行，导致编译后机器码太短，一般两行及以上不会有这个问题
3. 重复 mock 同一个函数：在最小单位的`PatchConvey`中重复 mock 同一个函数，如果确实有这种需求可以尝试获取`Mocker`后重新 mock
4. 其他工具 mock 了该函数：例如已经使用 [monkey](https://github.com/bouk/monkey) 等工具 mock 了该函数
