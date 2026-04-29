// +build windows

package joystick

import (
	"fmt"
	"golang.org/x/sys/windows"
	"math"
	"unsafe"
)

// XInput structs and constants

const (
	_XINPUT_FLAG_GAMEPAD = 0x00000001
)

type xinputGamepad struct {
	wButtons      uint16
	bLeftTrigger  uint8
	bRightTrigger uint8
	sThumbLX      int16
	sThumbLY      int16
	sThumbRX      int16
	sThumbRY      int16
}

type xinputState struct {
	dwPacketNumber uint32
	gamepad        xinputGamepad
}

type xinputVibration struct {
	wLeftMotorSpeed  uint16
	wRightMotorSpeed uint16
}

type xinputCapabilities struct {
	xtype     uint8
	subType   uint8
	flags     uint16
	gamepad   xinputGamepad
	vibration xinputVibration
}

var (
	xinputDLL          *windows.DLL
	procXInputGetState *windows.Proc
	procXInputGetCaps  *windows.Proc
)

func init() {
	for _, dllName := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		dll, err := windows.LoadDLL(dllName)
		if err == nil {
			xinputDLL = dll
			break
		}
	}
	if xinputDLL != nil {
		procXInputGetState, _ = xinputDLL.FindProc("XInputGetState")
		procXInputGetCaps, _ = xinputDLL.FindProc("XInputGetCapabilities")
	}
}

type xinputImpl struct {
	id    int
	name  string
	state State
}

func openXInput(id int) (Joystick, error) {
	if procXInputGetState == nil || procXInputGetCaps == nil {
		return nil, fmt.Errorf("XInput not available")
	}

	var caps xinputCapabilities
	ret, _, _ := procXInputGetCaps.Call(uintptr(id), _XINPUT_FLAG_GAMEPAD, uintptr(unsafe.Pointer(&caps)))
	if ret != 0 {
		return nil, fmt.Errorf("XInput controller %d not found", id)
	}

	js := &xinputImpl{
		id:   id,
		name: "XInput Controller",
	}
	js.state.AxisData = make([]int, 6)
	return js, nil
}

func (js *xinputImpl) AxisCount() int {
	return 6
}

func (js *xinputImpl) ButtonCount() int {
	return 16
}

func (js *xinputImpl) Name() string {
	return js.name
}

func (js *xinputImpl) Read() (State, error) {
	var xinState xinputState
	ret, _, _ := procXInputGetState.Call(uintptr(js.id), uintptr(unsafe.Pointer(&xinState)))
	if ret != 0 {
		return js.state, fmt.Errorf("Failed to read XInput controller %d", js.id)
	}

	g := xinState.gamepad
	js.state.Buttons = uint32(g.wButtons)
	js.state.AxisData[0] = int(g.sThumbLX)
	js.state.AxisData[1] = int(g.sThumbLY)
	js.state.AxisData[2] = int(g.sThumbRX)
	js.state.AxisData[3] = int(g.sThumbRY)
	js.state.AxisData[4] = int(mapValue(int64(g.bLeftTrigger), 0, 255, -32767, 32768))
	js.state.AxisData[5] = int(mapValue(int64(g.bRightTrigger), 0, 255, -32767, 32768))
	return js.state, nil
}

func (js *xinputImpl) Close() {
	// no impl under windows
}

var PrintFunc func(x, y int, s string)

const (
	_MAXPNAMELEN            = 32
	_MAX_JOYSTICKOEMVXDNAME = 260
	_MAX_AXIS               = 6

	_JOY_RETURNX        = 1
	_JOY_RETURNY        = 2
	_JOY_RETURNZ        = 4
	_JOY_RETURNR        = 8
	_JOY_RETURNU        = 16
	_JOY_RETURNV        = 32
	_JOY_RETURNPOV      = 64
	_JOY_RETURNBUTTONS  = 128
	_JOY_RETURNRAWDATA  = 256
	_JOY_RETURNPOVCTS   = 512
	_JOY_RETURNCENTERED = 1024
	_JOY_USEDEADZONE    = 2048
	_JOY_RETURNALL      = (_JOY_RETURNX | _JOY_RETURNY | _JOY_RETURNZ | _JOY_RETURNR | _JOY_RETURNU | _JOY_RETURNV | _JOY_RETURNPOV | _JOY_RETURNBUTTONS)

	_JOYCAPS_HASZ    = 0x1
	_JOYCAPS_HASR    = 0x2
	_JOYCAPS_HASU    = 0x4
	_JOYCAPS_HASV    = 0x8
	_JOYCAPS_HASPOV  = 0x10
	_JOYCAPS_POV4DIR = 0x20
	_JOYCAPS_POVCTS  = 0x40
)

type JOYCAPS struct {
	wMid        uint16
	wPid        uint16
	szPname     [_MAXPNAMELEN]uint16
	wXmin       uint32
	wXmax       uint32
	wYmin       uint32
	wYmax       uint32
	wZmin       uint32
	wZmax       uint32
	wNumButtons uint32
	wPeriodMin  uint32
	wPeriodMax  uint32
	wRmin       uint32
	wRmax       uint32
	wUmin       uint32
	wUmax       uint32
	wVmin       uint32
	wVmax       uint32
	wCaps       uint32
	wMaxAxes    uint32
	wNumAxes    uint32
	wMaxButtons uint32
	szRegKey    [_MAXPNAMELEN]uint16
	szOEMVxD    [_MAX_JOYSTICKOEMVXDNAME]uint16
}

type JOYINFOEX struct {
	dwSize         uint32
	dwFlags        uint32
	dwAxis         [_MAX_AXIS]uint32
	dwButtons      uint32
	dwButtonNumber uint32
	dwPOV          uint32
	dwReserved1    uint32
	dwReserved2    uint32
}

var (
	winmmdll      = windows.MustLoadDLL("Winmm.dll")
	joyGetPosEx   = winmmdll.MustFindProc("joyGetPosEx")
	joyGetDevCaps = winmmdll.MustFindProc("joyGetDevCapsW")
)

type axisLimit struct {
	min, max uint32
}

type joystickImpl struct {
	id           int
	axisCount    int
	povAxisCount int
	buttonCount  int
	name         string
	state        State
	axisLimits   []axisLimit
}

func mapValue(val, srcMin, srcMax, dstMin, dstMax int64) int64 {
	return (val-srcMin)*(dstMax-dstMin)/(srcMax-srcMin) + dstMin
}

// Open opens the Joystick for reading, with the supplied id
//
// Under linux the id is used to construct the joystick device name:
//   for example: id 0 will open device: "/dev/input/js0"
//
// Under Windows the id is the actual numeric id of the joystick
//
// If successful, a Joystick interface is returned which can be used to
// read the state of the joystick, else an error is returned
func Open(id int) (Joystick, error) {
	// Prefer XInput over WinMM: XInputGetState works regardless of window focus,
	// while joyGetPosEx (WinMM) may stop delivering input when the app loses focus
	// due to HID exclusive-access behaviour in some drivers.
	js, err := openXInput(id)
	if err == nil {
		return js, nil
	}

	// Fall back to WinMM for controllers that are not XInput-compatible (e.g. flight sticks)
	wmJs := &joystickImpl{}
	wmJs.id = id
	err = wmJs.getJoyCaps()
	if err == nil {
		return wmJs, nil
	}

	return nil, fmt.Errorf("no joystick found with id %d", id)
}

func (js *joystickImpl) getJoyCaps() error {
	var caps JOYCAPS
	ret, _, _ := joyGetDevCaps.Call(uintptr(js.id), uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))

	if ret != 0 {
		return fmt.Errorf("Failed to read Joystick %d", js.id)
	} else {
		js.axisCount = int(caps.wNumAxes)
		js.buttonCount = int(caps.wNumButtons)
		js.name = windows.UTF16ToString(caps.szPname[:])

		if caps.wCaps&_JOYCAPS_HASPOV != 0 {
			js.povAxisCount = 2
		}

		js.state.AxisData = make([]int, js.axisCount+js.povAxisCount, js.axisCount+js.povAxisCount)

		js.axisLimits = []axisLimit{
			{caps.wXmin, caps.wXmax},
			{caps.wYmin, caps.wYmax},
			{caps.wZmin, caps.wZmax},
			{caps.wRmin, caps.wRmax},
			{caps.wUmin, caps.wUmax},
			{caps.wVmin, caps.wVmax},
		}

		return nil
	}
}

func axisFromPov(povVal float64) int {
	switch {
	case povVal < -0.5:
		return -32767
	case povVal > 0.5:
		return 32768
	default:
		return 0
	}
}

func (js *joystickImpl) getJoyPosEx() error {
	var info JOYINFOEX
	info.dwSize = uint32(unsafe.Sizeof(info))
	info.dwFlags = _JOY_RETURNALL
	ret, _, _ := joyGetPosEx.Call(uintptr(js.id), uintptr(unsafe.Pointer(&info)))

	if ret != 0 {
		return fmt.Errorf("Failed to read Joystick %d", js.id)
	} else {
		js.state.Buttons = info.dwButtons

		for i := 0; i < js.axisCount; i++ {
			js.state.AxisData[i] = int(mapValue(int64(info.dwAxis[i]),
				int64(js.axisLimits[i].min), int64(js.axisLimits[i].max), -32767, 32768))
		}

		if js.povAxisCount > 0 {
			angleDeg := float64(info.dwPOV) / 100.0
			if angleDeg > 359.0 {
				js.state.AxisData[js.axisCount] = 0
				js.state.AxisData[js.axisCount+1] = 0
				return nil
			}

			angleRad := angleDeg * math.Pi / 180.0
			sin, cos := math.Sincos(angleRad)

			js.state.AxisData[js.axisCount] = axisFromPov(sin)
			js.state.AxisData[js.axisCount+1] = axisFromPov(-cos)
		}
		return nil
	}
}

func (js *joystickImpl) AxisCount() int {
	return js.axisCount + js.povAxisCount
}

func (js *joystickImpl) ButtonCount() int {
	return js.buttonCount
}

func (js *joystickImpl) Name() string {
	return js.name
}

func (js *joystickImpl) Read() (State, error) {
	err := js.getJoyPosEx()
	return js.state, err
}

func (js *joystickImpl) Close() {
	// no impl under windows
}
