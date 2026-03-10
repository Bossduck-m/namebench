//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func startHiddenCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
