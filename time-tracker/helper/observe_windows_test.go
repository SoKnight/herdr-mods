//go:build windows

package main

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestSampleDoesNotWaitForStuckAudio(t *testing.T) {
	swap(t, &audible, func() ([]string, error) { return []string{"a.exe"}, nil })
	s, _ := sample(nowMs())
	assertEq(t, slices.Equal(s.SoundExes, []string{"a.exe"}), true)

	release := make(chan struct{})
	var calls atomic.Int32
	swap(t, &audible, func() ([]string, error) {
		calls.Add(1)
		<-release // a stuck audio service
		return []string{"late.exe"}, nil
	})
	t.Cleanup(func() {
		close(release)
		for audioBusy.Load() { // the late answer lands before the next test samples
			time.Sleep(time.Millisecond)
		}
	})
	began := time.Now()
	s, _ = sample(nowMs())
	assertEq(t, time.Since(began) < 1500*time.Millisecond, true)
	assertEq(t, s.SoundExes == nil, true)
	s, _ = sample(nowMs()) // the stuck call is still in flight: no second one, no wait
	assertEq(t, time.Since(began) < 1500*time.Millisecond, true)
	assertEq(t, s.SoundExes == nil, true)
	assertEq(t, calls.Load(), int32(1))
}
