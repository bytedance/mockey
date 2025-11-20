//go:build go1.20
// +build go1.20

package fn

import (
	"reflect"
	"testing"

	"github.com/bytedance/mockey/internal/fn/type4test"
)

func TestAdapterImpl_extendedTargetType0(t *testing.T) {
	type fields struct {
		target interface{}
	}
	tests := []struct {
		name   string
		fields fields
		want   reflect.Type
	}{
		{"type4test.Foo[string]", fields{type4test.Foo[string]}, reflect.TypeOf(func(GenericInfo, string) {})},
		{"type4test.NoArgs[string]}", fields{type4test.NoArgs[string]}, reflect.TypeOf(func(GenericInfo) {})},
		{"(*type4test.A[string]).Foo", fields{(*type4test.A[string]).Foo}, reflect.TypeOf(func(*type4test.A[string], GenericInfo, int) {})},
		{"(*type4test.A[string]).NoArgs", fields{(*type4test.A[string]).NoArgs}, reflect.TypeOf(func(*type4test.A[string], GenericInfo) {})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := (&AdapterImpl{
				target: tt.fields.target,
			}).init()
			if got := a.extendedTargetType0(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extendedTargetType0() = %v, want %v", got, tt.want)
			}
		})
	}
}
