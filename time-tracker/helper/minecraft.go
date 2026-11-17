package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var connecting = regexp.MustCompile(`Connecting to (\S+), (\d+)`)

// isMinecraftOnline tells the game's window is on a multiplayer screen.
func isMinecraftOnline(exe, title string) bool {
	return (strings.EqualFold(exe, "javaw.exe") || strings.EqualFold(exe, "java.exe")) &&
		strings.HasPrefix(title, "Minecraft") && strings.Contains(title, "Multiplayer")
}

// serverFromLog gives host:port of the last server the log connected to, "" after a local world.
func serverFromLog(tail []byte) string {
	server := ""
	for _, line := range bytes.Split(tail, []byte("\n")) {
		if m := connecting.FindSubmatch(line); m != nil {
			server = fmt.Sprintf("%s:%s", m[1], m[2])
		} else if bytes.Contains(line, []byte("Starting integrated minecraft server")) {
			server = ""
		}
	}
	return server
}

// minecraftServer reads the server from the last 256 KiB of the most recently modified log; "" on any error.
func minecraftServer(paths []string) string {
	var best string
	var bestAt int64
	for _, p := range paths {
		p = expandWin(p)
		if fi, err := os.Stat(p); err == nil && (best == "" || fi.ModTime().UnixNano() > bestAt) {
			best, bestAt = p, fi.ModTime().UnixNano()
		}
	}
	f, err := os.Open(best)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	const tailSize = 256 << 10
	off := max(fi.Size()-tailSize, 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := io.ReadFull(io.NewSectionReader(f, off, int64(len(buf))), buf); err != nil {
		return ""
	}
	return serverFromLog(buf)
}

var envRef = regexp.MustCompile(`%([^%]+)%`)

// expandWin replaces %NAME% with the environment variable, on any OS.
func expandWin(p string) string {
	return envRef.ReplaceAllStringFunc(p, func(m string) string { return os.Getenv(m[1 : len(m)-1]) })
}
