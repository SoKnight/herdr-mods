package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestARequestCanComeFromAFileBesideTheHelper(t *testing.T) {
	if got, err := request(`{"action":"x"}`); err != nil || string(got) != `{"action":"x"}` {
		t.Fatalf("inline: %q %v", got, err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(filepath.Dir(self), "request-test.json")
	if err := os.WriteFile(file, []byte(`{"action":"long"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := request("@" + file); err != nil || string(got) != `{"action":"long"}` {
		t.Fatalf("file: %q %v", got, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the request file is left behind")
	}
	elsewhere := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(elsewhere, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := request("@" + elsewhere); err == nil {
		t.Fatal("read a file that isn't beside the helper")
	}
	if _, err := os.Stat(elsewhere); err != nil {
		t.Fatal("removed a file that isn't beside the helper")
	}
}
