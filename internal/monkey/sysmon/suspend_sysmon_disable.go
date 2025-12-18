//go:build mockey_disable_ss
// +build mockey_disable_ss

package sysmon

func SuspendSysmon() (resume func()) {
	return func() {}
}
