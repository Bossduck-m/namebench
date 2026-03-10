//go:build !windows

package main

import "os/exec"

func startHiddenCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
