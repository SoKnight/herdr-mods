// herdr-time is the Time Tracker's collector and its CLI.
package main

import "os"

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		attachConsole()
	}
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}
