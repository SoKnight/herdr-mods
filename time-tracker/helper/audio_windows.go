//go:build windows

package main

import (
	"fmt"
	"runtime"
	"slices"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32               = windows.NewLazySystemDLL("ole32.dll")
	pCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	pCoUninitialize     = ole32.NewProc("CoUninitialize")
	pCoCreateInstance   = ole32.NewProc("CoCreateInstance")
	clsidMMDeviceEnum   = guid("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	iidMMDeviceEnum     = guid("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	iidSessionManager2  = guid("{77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F}")
	iidSessionControl2  = guid("{BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D}")
	iidMeterInformation = guid("{C02216F6-8C67-4B5B-9D00-D008E73E0064}")
)

// Vtable slots, in the order of mmdeviceapi.h, audiopolicy.h and endpointvolume.h (IUnknown's 0-2 first).
const (
	mQueryInterface          = 0
	mRelease                 = 2
	mGetDefaultAudioEndpoint = 3 + 1 // IMMDeviceEnumerator: EnumAudioEndpoints, GetDefaultAudioEndpoint
	mActivate                = 3     // IMMDevice
	mGetSessionEnumerator    = 3 + 2 // IAudioSessionManager's GetAudioSessionControl, GetSimpleAudioVolume; then IAudioSessionManager2's
	mGetCount                = 3     // IAudioSessionEnumerator
	mGetSession              = 4
	mGetState                = 3         // IAudioSessionControl
	mGetProcessID            = 3 + 9 + 2 // IAudioSessionControl's 9, then GetSessionIdentifier, GetSessionInstanceIdentifier
	mGetPeakValue            = 3         // IAudioMeterInformation

	clsctxAll               = 0x17
	coinitMultithreaded     = 0
	rpcEChangedMode         = 0x80010106
	eRender, eMultimedia    = 0, 1
	audioSessionStateActive = 1
	audiblePeak             = 0.001
)

func guid(s string) *windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return &g
}

// comObj is a COM interface pointer: its first word points at the vtable.
type comObj struct{ vtbl *[32]uintptr }

// call keeps the out-pointers its callers pass as uintptr(unsafe.Pointer(&x)) alive and unmoved, as LazyProc.Call does.
//
//go:uintptrescapes
func (o *comObj) call(method int, args ...uintptr) error {
	r, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	if int32(r) < 0 {
		return fmt.Errorf("HRESULT 0x%08X", uint32(r))
	}
	return nil
}

func (o *comObj) release() { o.call(mRelease) }

func (o *comObj) query(iid *windows.GUID) (*comObj, error) {
	var p *comObj
	err := o.call(mQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&p)))
	if err == nil && p == nil {
		err = fmt.Errorf("QueryInterface gave nil")
	}
	return p, err
}

// audibleExes lists the exes holding an active render session on the default device whose peak is
// above silence.
func audibleExes() ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch r, _, _ := pCoInitializeEx.Call(0, coinitMultithreaded); {
	case r == rpcEChangedMode: // this thread already has COM in another mode: use it as it is
	case int32(r) < 0:
		return nil, fmt.Errorf("CoInitializeEx: HRESULT 0x%08X", uint32(r))
	default: // S_OK or S_FALSE: each needs its CoUninitialize
		defer pCoUninitialize.Call()
	}

	var enum *comObj
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(clsidMMDeviceEnum)), 0, clsctxAll,
		uintptr(unsafe.Pointer(iidMMDeviceEnum)), uintptr(unsafe.Pointer(&enum))); int32(r) < 0 || enum == nil {
		return nil, fmt.Errorf("CoCreateInstance(MMDeviceEnumerator): HRESULT 0x%08X", uint32(r))
	}
	defer enum.release()

	var dev *comObj
	if err := enum.call(mGetDefaultAudioEndpoint, eRender, eMultimedia, uintptr(unsafe.Pointer(&dev))); err != nil || dev == nil {
		return nil, fmt.Errorf("GetDefaultAudioEndpoint: %v", err) // no output device
	}
	defer dev.release()

	var mgr *comObj
	if err := dev.call(mActivate, uintptr(unsafe.Pointer(iidSessionManager2)), clsctxAll, 0, uintptr(unsafe.Pointer(&mgr))); err != nil || mgr == nil {
		return nil, fmt.Errorf("Activate(IAudioSessionManager2): %v", err)
	}
	defer mgr.release()

	var sessions *comObj
	if err := mgr.call(mGetSessionEnumerator, uintptr(unsafe.Pointer(&sessions))); err != nil || sessions == nil {
		return nil, fmt.Errorf("GetSessionEnumerator: %v", err)
	}
	defer sessions.release()

	var n int32
	if err := sessions.call(mGetCount, uintptr(unsafe.Pointer(&n))); err != nil {
		return nil, fmt.Errorf("GetCount: %v", err)
	}
	var out []string
	for i := range n {
		if exe := audibleSession(sessions, i); exe != "" && !slices.Contains(out, exe) {
			out = append(out, exe)
		}
	}
	return out, nil
}

// audibleSession returns the exe of session i when it is active and not silent, else "".
func audibleSession(sessions *comObj, i int32) string {
	var ctl *comObj
	if err := sessions.call(mGetSession, uintptr(i), uintptr(unsafe.Pointer(&ctl))); err != nil || ctl == nil {
		return ""
	}
	defer ctl.release()
	var state int32
	if ctl.call(mGetState, uintptr(unsafe.Pointer(&state))) != nil || state != audioSessionStateActive {
		return ""
	}
	meter, err := ctl.query(iidMeterInformation)
	if err != nil {
		return ""
	}
	defer meter.release()
	var peak float32
	if meter.call(mGetPeakValue, uintptr(unsafe.Pointer(&peak))) != nil || peak <= audiblePeak {
		return ""
	}
	ctl2, err := ctl.query(iidSessionControl2)
	if err != nil {
		return ""
	}
	defer ctl2.release()
	var pid uint32
	if ctl2.call(mGetProcessID, uintptr(unsafe.Pointer(&pid))) != nil {
		return ""
	}
	return imageName(pid)
}
