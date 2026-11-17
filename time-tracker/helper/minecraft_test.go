package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerFromLog(t *testing.T) {
	log := "[12:00:01] [Render thread/INFO]: Connecting to mc.tricky.net, 25565\n[12:30:00] [Render thread/INFO]: Connecting to localhost, 25566\n"
	assertEq(t, serverFromLog([]byte(log)), "localhost:25566")
	assertEq(t, serverFromLog([]byte(log+"[13:00:00] [Server thread/INFO]: Starting integrated minecraft server version 26.3\n")), "")
	assertEq(t, serverFromLog(nil), "")
}

func TestMinecraftServerReadsTheNewestLog(t *testing.T) {
	dir := t.TempDir()
	old, cur, missing := filepath.Join(dir, "a.log"), filepath.Join(dir, "b.log"), filepath.Join(dir, "none.log")
	must(t, os.WriteFile(old, []byte("Connecting to old.net, 25565\n"), 0o644))
	must(t, os.WriteFile(cur, []byte("Connecting to new.net, 25565\n"), 0o644))
	past := time.Now().Add(-time.Hour)
	must(t, os.Chtimes(old, past, past))
	assertEq(t, minecraftServer([]string{missing, old, cur}), "new.net:25565")
	assertEq(t, minecraftServer([]string{missing}), "")
	t.Setenv("MCDIR", dir)
	assertEq(t, minecraftServer([]string{`%MCDIR%\b.log`}), "new.net:25565")
}

func TestMinecraftWindow(t *testing.T) {
	assertEq(t, isMinecraftOnline("javaw.exe", "Minecraft 26.3 - Multiplayer (3rd-party Server)"), true)
	assertEq(t, isMinecraftOnline("javaw.exe", "Minecraft 26.3 - Singleplayer"), false)
	assertEq(t, isMinecraftOnline("chrome.exe", "Minecraft - Multiplayer guide"), false)
}
