package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// attachConsole gives the CLI of a -H=windowsgui build the console it was started from, which such a
// program has no handles to. A stdout or stderr that is a pipe or a file (the mod reads a pipe) stays.
func attachConsole() {
	out, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	errOut, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	valid := func(h windows.Handle) bool { return h != 0 && h != windows.InvalidHandle }
	if valid(out) && valid(errOut) {
		return
	}
	if r, _, _ := kernel32.NewProc("AttachConsole").Call(uintptr(^uint32(0))); r == 0 { // ATTACH_PARENT_PROCESS
		return
	}
	con, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	if !valid(out) {
		os.Stdout = con
	}
	if !valid(errOut) {
		os.Stderr = con
	}
}
