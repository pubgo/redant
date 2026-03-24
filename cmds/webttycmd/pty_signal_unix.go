//go:build !windows

package webttycmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func prepareInteractiveShellCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func signalPTYForegroundProcessGroup(ptmx *os.File, signalName string) error {
	if ptmx == nil {
		return fmt.Errorf("pty file is nil")
	}

	pgid, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return fmt.Errorf("read pty foreground process group failed: %w", err)
	}
	if pgid <= 0 {
		return fmt.Errorf("invalid pty foreground process group: %d", pgid)
	}

	var sig unix.Signal
	switch signalName {
	case "INT":
		sig = unix.SIGINT
	case "TSTP":
		sig = unix.SIGTSTP
	default:
		return fmt.Errorf("unsupported signal name: %s", signalName)
	}

	if err := unix.Kill(-pgid, sig); err != nil {
		return fmt.Errorf("send signal to foreground process group failed: %w", err)
	}
	return nil
}

func signalProcessGroupByPID(pid int, signalName string) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}

	pgid, err := unix.Getpgid(pid)
	if err != nil {
		return fmt.Errorf("get process group by pid failed: %w", err)
	}
	if pgid <= 0 {
		return fmt.Errorf("invalid process group id: %d", pgid)
	}

	var sig unix.Signal
	switch signalName {
	case "INT":
		sig = unix.SIGINT
	case "TSTP":
		sig = unix.SIGTSTP
	default:
		return fmt.Errorf("unsupported signal name: %s", signalName)
	}

	if err := unix.Kill(-pgid, sig); err != nil {
		return fmt.Errorf("send signal to process group failed: %w", err)
	}
	return nil
}
