//go:build go1.20
// +build go1.20

package iface

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type MyI interface {
	Foo1(string) string
	Foo2()
}

type MyIImpl1 struct {
	inner string
}

func (m *MyIImpl1) Foo1(s string) string {
	return strings.Repeat(s, 1) + m.inner
}

func (m *MyIImpl1) Foo2() {}

type MyIImpl2 struct {
	inner string
}

func (m MyIImpl2) Foo1(s string) string {
	return strings.Repeat(s, 1) + m.inner
}

func (m MyIImpl2) Foo2() {}

type NotImpl struct {
	inner string
}

func (n *NotImpl) Foo1(s string) string {
	return strings.Repeat(s, 1) + n.inner
}

func TestMockInterface(t *testing.T) {
	Convey("TestMockInterface", t, func() {
		mocker := Mock(MyI.Foo1).Return("MOCKED!").Build()

		m1 := &MyIImpl1{inner: "iii"}
		res1 := m1.Foo1("anything")
		So(res1, ShouldEqual, "MOCKED!")
		m2 := &MyIImpl2{}
		res2 := m2.Foo1("anything")
		So(res2, ShouldEqual, "MOCKED!")

		n := NotImpl{}
		res3 := n.Foo1("anything")
		So(res3, ShouldEqual, "MOCKED!") // FIXME: unexpected

		mocker.UnPatch()
		res11 := m1.Foo1("anything")
		So(res11, ShouldEqual, "anythingiii")
		res22 := m2.Foo1("anything")
		So(res22, ShouldEqual, "anything")
	})
}
