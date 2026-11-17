package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanickingHandlerReleasesTheLock(t *testing.T) {
	d := &Daemon{}
	h := d.api(func(*http.Request) (int, any, error) { panic("boom") })
	func() {
		defer func() { _ = recover() }() // net/http recovers it the same way
		h(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	if !d.mu.TryLock() {
		t.Fatal("the daemon lock is still held after a handler panicked")
	}
}
