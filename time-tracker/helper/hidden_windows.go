package main

import (
	"os/exec"
	"syscall"
)

// hidden keeps a console program from flashing a window: Herdr starts the helper without one.
func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

// launchCollector starts the collector detached from this console, so closing it doesn't stop the collector.
func launchCollector(exe string) error {
	cmd := exec.Command(exe, "run")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000 | 0x00000008} // CREATE_NO_WINDOW | DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
