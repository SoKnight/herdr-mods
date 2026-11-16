//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	pGetLastInputInfo  = user32.NewProc("GetLastInputInfo")
	pOpenInputDesktop  = user32.NewProc("OpenInputDesktop")
	pCloseDesktop      = user32.NewProc("CloseDesktop")
	pGetUserObjectInfo = user32.NewProc("GetUserObjectInformationW")
	pGetWindowText     = user32.NewProc("GetWindowTextW")
	pGetTickCount64    = kernel32.NewProc("GetTickCount64")
	pWTSQuerySession   = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")
	micKey             = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone\NonPackaged`
	desktopReadObjects = uintptr(0x1)
	uoiName            = uintptr(2)
)

func tickCount() int64 {
	r, _, _ := pGetTickCount64.Call()
	return int64(r)
}

// bootTime returns the Unix ms of the last boot.
func bootTime(now int64) int64 { return now - tickCount() }

// wtsInfo is WTSINFOW.
type wtsInfo struct {
	State, SessionID, InBytes, OutBytes, InFrames, OutFrames, InCompressed, OutCompressed uint32
	WinStation                                                                            [32]uint16
	Domain                                                                                [17]uint16
	User                                                                                  [21]uint16
	Connect, Disconnect, LastInput, Logon, Current                                        int64 // FILETIME
}

// logonTime returns the Unix ms of the current session's logon, or 0. Unlike the tick count it is
// reset by a Fast Startup "shutdown", which only hibernates the kernel.
func logonTime() int64 {
	const currentSession, wtsSessionInfo = 0xFFFFFFFF, 24
	var info *wtsInfo
	var n uint32
	if r, _, _ := pWTSQuerySession.Call(0, currentSession, wtsSessionInfo, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&n))); r == 0 || info == nil {
		return 0
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(info)))
	if uintptr(n) < unsafe.Sizeof(*info) {
		return 0
	}
	if info.Logon <= 0 {
		return 0
	}
	return (info.Logon - 116444736000000000) / 10000 // 100 ns since 1601 → ms since 1970
}

func sample(now int64) (Sample, error) {
	s := Sample{At: now, Locked: locked(), CallExes: micUsers()}
	var li struct{ Size, Time uint32 }
	li.Size = uint32(unsafe.Sizeof(li))
	if r, _, err := pGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return s, err
	}
	// dwTime is a 32-bit tick count, so subtract in uint32 to survive wraparound.
	s.LastInput = now - int64(uint32(tickCount())-li.Time)
	s.Exe, s.Title = foreground()
	return s, nil
}

// locked is true when the input desktop is not "Default" or can't be opened.
func locked() bool {
	h, _, _ := pOpenInputDesktop.Call(0, 0, desktopReadObjects)
	if h == 0 {
		return true
	}
	defer pCloseDesktop.Call(h)
	var buf [256]uint16
	var n uint32
	r, _, _ := pGetUserObjectInfo.Call(h, uoiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&n)))
	return r == 0 || windows.UTF16ToString(buf[:]) != "Default"
}

// foreground returns the exe file name and title of the foreground window; empty on failure.
func foreground() (exe, title string) {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return
	}
	var tbuf [512]uint16
	if n, _, _ := pGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&tbuf[0])), uintptr(len(tbuf))); n > 0 {
		title = windows.UTF16ToString(tbuf[:n])
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	var pbuf [windows.MAX_PATH * 2]uint16
	n := uint32(len(pbuf))
	if windows.QueryFullProcessImageName(h, 0, &pbuf[0], &n) == nil {
		exe = filepath.Base(windows.UTF16ToString(pbuf[:n]))
	}
	return
}

// micUsers lists the exes whose microphone session is still open.
func micUsers() []string {
	k, err := registry.OpenKey(registry.CURRENT_USER, micKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, name := range names {
		sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		stop, _, err := sk.GetIntegerValue("LastUsedTimeStop")
		sk.Close()
		if err == nil && stop == 0 {
			out = append(out, filepath.Base(strings.ReplaceAll(name, "#", `\`)))
		}
	}
	return out
}
