//go:build go1.18
// +build go1.18

package iface

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	. "github.com/bytedance/mockey"
	"github.com/bytedance/mockey/internal/tool"
)

// New creates a new instance of the interface T with the given methods.
// It works as follows:
// 1. Dynamically generates implementation code for interface T
// 2. Compiles it into a Go plugin
// 3. Loads the plugin and retrieves an instance of the interface
// 4. Mocks method behaviors based on the provided funcs map
// Note Go Plugin is used to generate a new implementation of T, so it shares the same limitation as Go Plugin.
func New[T any](funcs map[string]any) T {
	// analyze the interface
	data := analyze[T]()

	// write impl files
	tmpDir, err := os.MkdirTemp("", "mockey_impl")
	if err != nil {
		panic(fmt.Errorf("new interface failed: error make tmp dir: %w", err))
	}
	if !tool.IsDebug() { // keep tmpDir for debug
		defer os.RemoveAll(tmpDir)
	}

	tool.DebugPrintf("[iface.New] interface: %s, tmpDir: %s\n", data.NameWithPkg, tmpDir)

	goFilePath := filepath.Join(tmpDir, "impl.go")
	goFile, err := os.OpenFile(goFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(fmt.Errorf("new interface failed: error create impl.go: %w", err))
	}
	defer goFile.Close()

	if err := GetTemplate().Execute(goFile, data); err != nil {
		panic(fmt.Errorf("new interface failed: error execute template: %w", err))
	}

	// build plugin and get the instance
	soFilePath := filepath.Join(tmpDir, "impl.so")

	compileArgs := append([]string{"build"}, extractCompileParams(goFilePath, soFilePath)...)
	compileCmd := exec.Command("go", compileArgs...)
	tool.DebugPrintf("[iface.New] compileCmd: %s\n", compileCmd.String())

	out, err := compileCmd.CombinedOutput()
	tool.DebugPrintf("[iface.New] err: %v, out: %s\n", err, string(out))
	tool.Assert(err == nil, "new interface failed: error compile plugin")

	p, err := plugin.Open(soFilePath)
	tool.Assert(err == nil, "new interface failed: error open plugin: %v", err)

	sym, err := p.Lookup(data.InstanceName)
	tool.Assert(err == nil, "new interface failed: error lookup symbol: %v", err)

	inst, ok := sym.(T)
	tool.Assert(ok, "new interface failed: error convert %T", sym)

	tool.DebugPrintf("[iface.New] instance new success: %s\n", data.InstanceName)

	// mock methods
	for k, v := range funcs {
		Mock(GetMethod(inst, k)).To(v).Build()
	}
	return inst
}

var genericReg = regexp.MustCompile(`[\[\]]`)

func analyze[T any]() *TemplateData {
	var i T
	typ := reflect.TypeOf(&i).Elem()

	res := &TemplateData{
		SourcePath:   CurrentPath(),
		NameWithPkg:  typ.String(),
		InstanceName: typ.Name() + "Impl",
	}
	res.InstanceName = genericReg.ReplaceAllString(res.InstanceName, "_") // if T is a generic interface

	for k := 0; k < typ.NumMethod(); k++ {
		m := typ.Method(k)
		md := &Method{Name: m.Name}

		ft := m.Type

		for j := 0; j < ft.NumIn(); j++ {
			inType := ft.In(j)
			md.InputArgs = append(md.InputArgs, &Arg{NameWithPkg: inType.String()})
			res.Imports = append(res.Imports, extractPkgPath(inType)...)
		}

		for j := 0; j < ft.NumOut(); j++ {
			outType := ft.Out(j)
			md.OutputArgs = append(md.OutputArgs, &Arg{NameWithPkg: outType.String()})
			res.Imports = append(res.Imports, extractPkgPath(outType)...)
		}
		res.Methods = append(res.Methods, md)
	}

	importMaps := map[string]struct{}{}
	for _, imp := range res.Imports {
		if imp == "" {
			continue
		}
		importMaps[imp] = struct{}{}
	}
	var imports []string
	for k := range importMaps {
		imports = append(imports, k)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i] < imports[j] })
	res.Imports = imports

	return res
}

func extractPkgPath(typ reflect.Type) (res []string) {
	switch typ.Kind() {
	case reflect.Ptr:
		res = append(res, extractPkgPath(typ.Elem())...)
	case reflect.Slice:
		res = append(res, extractPkgPath(typ.Elem())...)
	case reflect.Map:
		res = append(res, extractPkgPath(typ.Key())...)
		res = append(res, extractPkgPath(typ.Elem())...)
	default:
		if typ.PkgPath() == "" {
			return
		}
		res = append(res, typ.PkgPath())
	}
	return
}

func extractCompileParams(goPath, soPath string) (res []string) {
	res = []string{"-buildmode", "plugin"}
	defer func() { res = append(res, "-o", soPath, goPath) }()

	// extract from env `MOCKEY_PLUGIN_FLAG` with JSON array format
	// e.g. MOCKEY_PLUGIN_FLAG='["-gcflags","all=-l -N"]'
	const (
		PluginModeFlag = "MOCKEY_PLUGIN_FLAG"
	)
	if flagStr := os.Getenv(PluginModeFlag); flagStr != "" {
		tool.DebugPrintf("[iface.extractCompileParams] plugin mode flag: %s\n", flagStr)
		err := json.Unmarshal([]byte(flagStr), &res) == nil
		tool.Assert(err, "unmarshal plugin mode flag failed: %v", flagStr)
		return
	}

	// extract from build info
	if info, ok := debug.ReadBuildInfo(); ok && len(info.Settings) > 0 {
		tool.DebugPrintf("[iface.extractCompileParams] build info flag: %v\n", info.Settings)
		ignoreFlags := []string{"-buildmode", "-cover"}
		ignoreParamFlags := []string{"-race"}
		for _, setting := range info.Settings {
			if !strings.HasPrefix(setting.Key, "-") ||
				tool.ContainsString(ignoreFlags, setting.Key) {
				continue
			}
			if tool.ContainsString(ignoreParamFlags, setting.Key) {
				res = append(res, setting.Key)
				continue
			}
			res = append(res, setting.Key, setting.Value)
		}
		return
	}

	// fallback, only support "-race", "-gcflags"
	tool.DebugPrintf("[iface.extractCompileParams] fallback build flags\n")
	if tool.IsGCFlagsSet() {
		res = append(res, "-gcflags", "all=-l -N")
	}
	if tool.RaceEnabled() {
		res = append(res, "-race")
	}
	return
}
