//go:build !windows

package ui

import "os/exec"

func combinedOutputHidden(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
