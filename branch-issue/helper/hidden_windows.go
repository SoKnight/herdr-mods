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
