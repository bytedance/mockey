//go:build go1.18
// +build go1.18

package iface

import (
	"embed"
	"text/template"
)

var (
	//go:embed template.tmpl
	f embed.FS
	t *template.Template
)

func init() {
	t = template.Must(template.ParseFS(f, "template.tmpl"))
}

func GetTemplate() *template.Template {
	return t
}

type TemplateData struct {
	SourcePath   string
	NameWithPkg  string
	Imports      []string
	Methods      []*Method
	InstanceName string
}

type Method struct {
	Name       string
	InputArgs  []*Arg
	OutputArgs []*Arg
}
type Arg struct {
	NameWithPkg string
}
