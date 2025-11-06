//go:build !windows && go1.18
// +build !windows,go1.18

package iface

import (
	"bytes"
	"go/ast"
	"go/token"
	"io"
	"io/fs"
	"net"
	"testing"
	"time"

	"github.com/bytedance/mockey/exp/iface/type4test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewInterface(t *testing.T) {
	Convey("TestNewInterface", t, func() {
		Convey("one method", func() {
			reader := New[io.Reader](map[string]any{
				"Read": func(p []byte) (n int, err error) { return 0, io.EOF },
			})
			res, err := io.ReadAll(reader)
			So(err, ShouldBeNil)
			So(res, ShouldBeEmpty)
		})
		Convey("three methods, mock two", func() {
			file := New[fs.File](map[string]any{
				"Read": func(p []byte) (n int, err error) { return -1, nil },
				"Stat": func() (fs.FileInfo, error) { return nil, nil },
			})
			res1, err := file.Read([]byte{1, 2, 3})
			So(err, ShouldEqual, nil)
			So(res1, ShouldEqual, -1)

			res2, err := file.Stat()
			So(err, ShouldEqual, nil)
			So(res2, ShouldBeNil)

			So(func() { file.Close() }, ShouldPanicWith, "mockey generated!")
		})

		Convey("args in other package 1", func() {
			conn := New[net.Conn](map[string]any{
				"SetDeadline": func(t time.Time) error { return nil },
			})
			err := conn.SetDeadline(time.Time{})
			So(err, ShouldEqual, nil)
		})

		Convey("args in other package 2", func() {
			node := New[ast.Node](map[string]any{
				"Pos": func() token.Pos { return token.Pos(1) },
				"End": func() token.Pos { return token.Pos(2) },
			})
			So(node.Pos(), ShouldEqual, token.Pos(1))
			So(node.End(), ShouldEqual, token.Pos(2))
		})

		Convey("inner interface", func() {
			readerWriter := New[io.ReadWriter](map[string]any{
				"Read":  func(p []byte) (n int, err error) { return -1, io.EOF },
				"Write": func(p []byte) (n int, err error) { return -1, io.EOF },
			})
			n, err := readerWriter.Read(nil)
			So(n, ShouldEqual, -1)
			So(err, ShouldEqual, io.EOF)
			n, err = readerWriter.Write(nil)
			So(n, ShouldEqual, -1)
			So(err, ShouldEqual, io.EOF)
		})

		Convey("contains self", func() {
			visitor := New[ast.Visitor](map[string]any{
				"Visit": func(node ast.Node) (w ast.Visitor) { return nil },
			})
			v := visitor.Visit(nil)
			So(v, ShouldBeNil)
		})

		Convey("struct args", func() {
			structI := New[type4test.TestStructI](map[string]any{
				"Foo": func() bytes.Buffer {
					return bytes.Buffer{}
				},
				"FooPtr": func() *bytes.Buffer {
					return &bytes.Buffer{}
				},
			})
			res := structI.Foo()
			So(res, ShouldResemble, bytes.Buffer{})
			resPtr := structI.FooPtr()
			So(resPtr, ShouldResemble, &bytes.Buffer{})
		})

		Convey("slice args", func() {
			sliceI := New[type4test.TestSliceI](map[string]any{
				"Foo": func() []bytes.Buffer {
					return []bytes.Buffer{}
				},
				"FooPtr": func() *[]*bytes.Buffer {
					return &[]*bytes.Buffer{}
				},
			})
			res := sliceI.Foo()
			So(res, ShouldResemble, []bytes.Buffer{})
			resPtr := sliceI.FooPtr()
			So(resPtr, ShouldResemble, &[]*bytes.Buffer{})
		})

		Convey("map args", func() {
			mapI := New[type4test.TestMapI](map[string]any{
				"Foo": func() map[*io.SectionReader][]bytes.Buffer {
					return map[*io.SectionReader][]bytes.Buffer{}
				},
				"FooPtr": func() *map[*io.SectionReader][]bytes.Buffer {
					return &map[*io.SectionReader][]bytes.Buffer{}
				},
			})
			res := mapI.Foo()
			So(res, ShouldResemble, map[*io.SectionReader][]bytes.Buffer{})
			resPtr := mapI.FooPtr()
			So(resPtr, ShouldResemble, &map[*io.SectionReader][]bytes.Buffer{})
		})

		Convey("generic type", func() {
			myGenericI := New[type4test.TestGenericI[int]](map[string]any{
				"Foo": func() int { return 0 },
			})
			res := myGenericI.Foo()
			So(res, ShouldEqual, 0)
		})

		Convey("not match", func() {
			So(func() { New[io.Reader](map[string]any{"Read": 0}) }, ShouldPanic)
			So(func() { New[io.Reader](map[string]any{"Read": func() {}}) }, ShouldPanic)
		})

		Convey("not mocked method", func() {
			So(func() { _, _ = New[io.Reader](map[string]any{}).Read(nil) }, ShouldPanicWith, "mockey generated!")
		})
	})
}
