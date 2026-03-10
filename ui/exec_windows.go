//go:build windows

package ui

import (
	"os/exec"
	"syscall"
)

func combinedOutputHidden(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.CombinedOutput()
}
