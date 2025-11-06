package iface

import (
	"path/filepath"
	"runtime"
)

func RootPath() string {
	return filepath.Dir(CurrentPath())
}

func CurrentPath() string {
	_, path, _, _ := runtime.Caller(1)
	return filepath.Dir(path)
}

func CurrentDir() string {
	_, path, _, _ := runtime.Caller(1)
	return filepath.Base(filepath.Dir(path))
}
