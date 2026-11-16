//go:build !windows

package main

import "errors"

var errUnsupported = errors.New("window observation is only supported on Windows")

func sample(now int64) (Sample, error) { return Sample{}, errUnsupported }

func bootTime(now int64) int64 { return 0 }

func logonTime() int64 { return 0 }
