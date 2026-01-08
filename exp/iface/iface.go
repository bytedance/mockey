//go:build go1.20
// +build go1.20

package iface

import (
	"reflect"
	"runtime"
	"unsafe"

	"github.com/bytedance/mockey/internal/fn"
	fn2 "github.com/bytedance/mockey/internal/monkey/fn"
)

func findImplementTargets(in interface{}) (res []interface{}) {
	return findImplementTargetsByFunc(in)
}

func findImplementTargetsByFunc(in interface{}) (res []interface{}) {
	ifaceAnalyzer := fn.NewNameAnalyzerByValue(reflect.ValueOf(in))
	ifaceName := ifaceAnalyzer.FuncName()
	funcList := funcNameMap[ifaceName]
	for _, fun := range funcList {
		pc := fun.Entry()
		if pc == reflect.ValueOf(in).Pointer() {
			continue
		}
		// FIXME: cannot determine exactly, try use other information

		// in: func(self MyI, inArgs...) outArgs

		// res:
		// func(self *MyIImpl1, inArgs...) outArgs
		// func(self MyIImpl2, inArgs...) outArgs
		// ...

		// FIXME: cannot get the right receiver type
		res = append(res, fn2.MakeFunc(reflect.TypeOf(in), fun.Entry()).Interface())
	}
	return
}

var (
	funcNameMap = make(map[string][]*runtime.Func)
)

const (
	funcTabOffset = 128
	textOffset    = 176
)

func init() {
	md := getMainModuleData()
	textStart := *(*uintptr)(unsafe.Pointer(uintptr(md) + uintptr(textOffset)))
	funcTabStart := *(**functab)(unsafe.Pointer(uintptr(md) + uintptr(funcTabOffset)))
	funcTabSize := *(*int)(unsafe.Pointer(uintptr(md) + uintptr(funcTabOffset) + unsafe.Sizeof(uintptr(0))))
	header := reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(funcTabStart)),
		Len:  funcTabSize,
		Cap:  funcTabSize,
	}
	funcTabs := *(*[]functab)(unsafe.Pointer(&header))

	for _, tab := range funcTabs {
		pc := textStart + uintptr(tab.entryoff)
		fun := runtime.FuncForPC(pc)
		curFullName := fun.Name()
		if curFullName == "" {
			continue
		}
		curAnalyzer := fn.NewNameAnalyzer(curFullName)
		if !curAnalyzer.HasMiddleName() { // not a method
			continue
		}
		funcName := curAnalyzer.FuncName()
		funcNameMap[funcName] = append(funcNameMap[funcName], fun)
	}
}

type functab struct {
	entryoff uint32
	funcoff  uint32
}

func getMainModuleData() unsafe.Pointer {
	var f = getMainModuleData
	entry := **(**uintptr)(unsafe.Pointer(&f))
	_, pointer := findfunc(entry)
	return pointer
}

//go:linkname findfunc runtime.findfunc
func findfunc(_ uintptr) (unsafe.Pointer, unsafe.Pointer)
