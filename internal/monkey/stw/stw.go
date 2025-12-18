//go:build !mockey_disable_stw
// +build !mockey_disable_stw

package stw

func StopTheWorld() (resume func()) {
	return doStopTheWorld()
}
