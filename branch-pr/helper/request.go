package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// request is the JSON the mod sent: the argument itself, or "@file" when it was too long for
// a Windows command line (32,767 characters). The file, beside the helper, is read once and
// removed; a file anywhere else is refused.
func request(arg string) ([]byte, error) {
	if !strings.HasPrefix(arg, "@") {
		return []byte(arg), nil
	}
	path := filepath.Clean(arg[1:])
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(filepath.Dir(path), filepath.Dir(self)) {
		return nil, errors.New("a request file must be beside the helper")
	}
	defer os.Remove(path)
	return os.ReadFile(path)
}
