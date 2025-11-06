//go:build !windows && go1.18
// +build !windows,go1.18

package type4test

import (
	"bytes"
	"io"
)

type TestMapI interface {
	Foo() map[*io.SectionReader][]bytes.Buffer
	FooPtr() *map[*io.SectionReader][]bytes.Buffer
}

type TestSliceI interface {
	Foo() []bytes.Buffer
	FooPtr() *[]*bytes.Buffer
}

type TestStructI interface {
	Foo() bytes.Buffer
	FooPtr() *bytes.Buffer
}

type TestGenericI[T any] interface {
	Foo() T
}
