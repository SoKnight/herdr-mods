//go:build !windows

package main

func audibleExes() ([]string, error) { return nil, errUnsupported }
