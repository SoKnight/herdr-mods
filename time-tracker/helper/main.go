// herdr-time is the Time Tracker's collector and its CLI.
package main

import "os"

func main() { os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr)) }
