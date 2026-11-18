package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

// setRunValue sets the HKCU Run value herdr-time to this program's "run", or deletes it.
func setRunValue(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue("herdr-time"); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("herdr-time", `"`+exe+`" run`)
}
