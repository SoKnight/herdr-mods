//go:build !windows

package main

import "errors"

func registerRun(string) error { return errors.New("Windows only") }
func launch(string) error      { return errors.New("Windows only") }
