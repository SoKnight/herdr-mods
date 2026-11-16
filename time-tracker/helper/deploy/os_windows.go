package main

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func registerRun(value string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runVal, value)
}

// launch starts the collector detached from this console, so closing it doesn't stop the collector.
func launch(exe string) error {
	cmd := exec.Command(exe, "run")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000 | 0x00000008} // CREATE_NO_WINDOW | DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
