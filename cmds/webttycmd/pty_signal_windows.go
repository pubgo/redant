//go:build windows

package webttycmd

import (
	"fmt"
	"os"
	"os/exec"
)

func prepareInteractiveShellCmd(cmd *exec.Cmd) {
	_ = cmd
}

func signalPTYForegroundProcessGroup(ptmx *os.File, signalName string) error {
	if ptmx == nil {
		return fmt.Errorf("pty file is nil")
	}
	return fmt.Errorf("signal forwarding is not supported on windows")
}

func signalProcessGroupByPID(pid int, signalName string) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	return fmt.Errorf("signal forwarding is not supported on windows")
}
