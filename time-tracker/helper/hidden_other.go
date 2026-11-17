//go:build !windows

package main

import "os/exec"

func hidden(cmd *exec.Cmd) *exec.Cmd { return cmd }

func launchCollector(exe string) error {
	cmd := exec.Command(exe, "run")
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
