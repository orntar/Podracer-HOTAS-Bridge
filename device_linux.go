//go:build linux && amd64

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

func ioctl(fd int, request uintptr, p []byte) error {
	if len(p) == 0 {
		return ioctlValue(fd, request, 0)
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(&p[0])))
	if errno != 0 {
		return errno
	}
	return nil
}
func ioctlValue(fd int, request, value uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, value)
	if errno != 0 {
		return errno
	}
	return nil
}
func ior(n, size int) uintptr     { return uintptr(0x80000000 | size<<16 | 0x4500 | n) }
func hasBit(b []byte, n int) bool { return n >= 0 && n/8 < len(b) && b[n/8]&(1<<uint(n%8)) != 0 }
func textIoctl(fd, n int) string {
	b := make([]byte, 256)
	if ioctl(fd, ior(n, len(b)), b) != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\x00")
}

type linuxPhysical struct {
	fd          int
	info        DeviceInfo
	axes        map[int]float64
	buttons     map[int]bool
	buttonCodes map[int]int
	closed      bool
}

func (p *linuxPhysical) Info() DeviceInfo { return p.info }
func openPhysical(path string) (Physical, error) {
	if filepath.Dir(path) != "/dev/input" || !strings.HasPrefix(filepath.Base(path), "event") {
		return nil, errors.New("select a device from the controller list")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w; see Device access in the app", path, err)
	}
	fail := func(e error) (Physical, error) { syscall.Close(fd); return nil, e }
	name := textIoctl(fd, 6)
	if strings.HasPrefix(name, "Podracer Bridge") {
		return fail(errors.New("cannot use the bridge's own virtual controller"))
	}
	id := make([]byte, 8)
	if err = ioctl(fd, ior(2, 8), id); err != nil {
		return fail(err)
	}
	vendor, product := binary.LittleEndian.Uint16(id[2:]), binary.LittleEndian.Uint16(id[4:])
	p := &linuxPhysical{fd: fd, axes: map[int]float64{}, buttons: map[int]bool{}, buttonCodes: map[int]int{}, info: DeviceInfo{Path: path, Name: name, Vendor: vendor, Product: product}}
	p.info.ID = fmt.Sprintf("%04x:%04x:%s:%s", vendor, product, name, textIoctl(fd, 8))
	bits := make([]byte, 8)
	if err = ioctl(fd, ior(0x23, len(bits)), bits); err != nil {
		return fail(err)
	}
	for code := 0; code < 64; code++ {
		if hasBit(bits, code) {
			a := make([]byte, 24)
			if err = ioctl(fd, ior(0x40+code, 24), a); err != nil {
				return fail(err)
			}
			v := int32(binary.LittleEndian.Uint32(a))
			lo := int32(binary.LittleEndian.Uint32(a[4:]))
			hi := int32(binary.LittleEndian.Uint32(a[8:]))
			if hi > lo {
				axis := AxisInfo{Code: code, Name: axisName(code), Min: lo, Max: hi, Value: v}
				p.info.Axes = append(p.info.Axes, axis)
				p.axes[code] = float64(v)
			}
		}
	}
	if len(p.info.Axes) < 2 {
		return fail(errors.New("device does not have two analog axes"))
	}
	keys := make([]byte, 96)
	if err = ioctl(fd, ior(0x21, len(keys)), keys); err != nil {
		return fail(err)
	}
	if !hasBit(keys, 288) && !hasBit(keys, 304) && !hasBit(keys, 256) {
		return fail(errors.New("device has no joystick/gamepad button group"))
	}
	current := make([]byte, 96)
	if err = ioctl(fd, ior(0x18, len(current)), current); err != nil {
		return fail(err)
	}
	n := 0
	for code := 256; code < 768; code++ {
		if hasBit(keys, code) {
			n++
			p.buttonCodes[code] = n
			p.buttons[n] = hasBit(current, code)
		}
	}
	p.info.Buttons = n
	if n == 0 {
		return fail(errors.New("no joystick buttons; use the joystick firmware"))
	}
	return p, nil
}
func axisName(c int) string {
	names := map[int]string{0: "X", 1: "Y", 2: "Z", 3: "RX", 4: "RY", 5: "RZ", 6: "Throttle", 7: "Rudder", 8: "Wheel", 9: "Gas", 10: "Brake", 16: "Hat X", 17: "Hat Y"}
	if n, ok := names[c]; ok {
		return n
	}
	return fmt.Sprintf("Axis %d", c)
}
func listDevices() ([]DeviceInfo, []string) {
	paths, _ := filepath.Glob("/dev/input/event*")
	devices := []DeviceInfo{}
	warnings := []string{}
	for _, path := range paths {
		p, err := openPhysical(path)
		if err != nil {
			if errors.Is(err, syscall.EACCES) {
				warnings = append(warnings, path+": permission denied")
			}
			continue
		}
		devices = append(devices, p.Info())
		p.Close()
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	return devices, warnings
}
func (p *linuxPhysical) Read() (map[int]float64, map[int]bool, error) {
	b := make([]byte, 24*128)
	// Bound draining so a misbehaving device cannot starve Stop or the watchdog.
	for batch := 0; batch < 32; batch++ {
		n, err := syscall.Read(p.fd, b)
		if err == syscall.EAGAIN {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if n == 0 {
			return nil, nil, errors.New("device reached end of stream")
		}
		if n%24 != 0 {
			return nil, nil, errors.New("invalid input event size")
		}
		for pos := 0; pos < n; pos += 24 {
			ev := b[pos : pos+24]
			typ := binary.LittleEndian.Uint16(ev[16:])
			code := int(binary.LittleEndian.Uint16(ev[18:]))
			value := int32(binary.LittleEndian.Uint32(ev[20:]))
			if typ == 0 && code == 3 {
				return nil, nil, errors.New("input events dropped; reconnect to resynchronize")
			}
			if typ == 3 {
				if _, ok := p.axes[code]; ok {
					p.axes[code] = float64(value)
				}
			}
			if typ == 1 {
				if idx, ok := p.buttonCodes[code]; ok {
					p.buttons[idx] = value != 0
				}
			}
		}
	}
	return p.axes, p.buttons, nil
}
func (p *linuxPhysical) Grab(on bool) error {
	v := uintptr(0)
	if on {
		v = 1
	}
	return ioctlValue(p.fd, 0x40044590, v)
}
func (p *linuxPhysical) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	return syscall.Close(p.fd)
}

// Match xpad's legacy BTN_A/B/X/Y codes for the emulated Xbox 360 identity.
var padKeys = map[string]int{"pad:A": 304, "pad:B": 305, "pad:X": 307, "pad:Y": 308, "pad:LB": 310, "pad:RB": 311, "pad:BACK": 314, "pad:START": 315, "pad:GUIDE": 316, "pad:LS": 317, "pad:RS": 318}
var keyboardKeys = map[string]int{"key:1": 2, "key:2": 3, "key:3": 4, "key:4": 5, "key:SPACE": 57, "key:SHIFT": 42, "key:CTRL": 29, "key:E": 18, "key:R": 19, "key:S": 31, "key:TAB": 15, "key:ESC": 1, "key:INSERT": 110, "key:ENTER": 28, "key:W": 17, "key:A": 30, "key:D": 32, "key:Q": 16, "key:F": 33, "key:C": 46, "key:V": 47, "key:UP": 103, "key:DOWN": 108, "key:LEFT": 105, "key:RIGHT": 106}

type linuxVirtual struct {
	pad, key int
	previous map[string]bool
	closed   bool
}

func createUinput(name string, keys map[string]int, gamepad bool) (int, error) {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("cannot create virtual controller: %w; open Device access for setup instructions", err)
	}
	fail := func(e error) (int, error) { syscall.Close(fd); return -1, e }
	if err = ioctlValue(fd, 0x40045564, 1); err != nil {
		return fail(err)
	}
	for _, code := range keys {
		if err = ioctlValue(fd, 0x40045565, uintptr(code)); err != nil {
			return fail(err)
		}
	}
	if gamepad {
		if err = ioctlValue(fd, 0x40045564, 3); err != nil {
			return fail(err)
		}
		for _, a := range []struct {
			code     int
			min, max int32
		}{{0, -32768, 32767}, {1, -32768, 32767}, {2, 0, 255}, {3, -32768, 32767}, {4, -32768, 32767}, {5, 0, 255}, {16, -1, 1}, {17, -1, 1}} {
			if err = ioctlValue(fd, 0x40045567, uintptr(a.code)); err != nil {
				return fail(err)
			}
			b := make([]byte, 28)
			binary.LittleEndian.PutUint16(b, uint16(a.code))
			binary.LittleEndian.PutUint32(b[8:], uint32(a.min))
			binary.LittleEndian.PutUint32(b[12:], uint32(a.max))
			if err = ioctl(fd, 0x401c5504, b); err != nil {
				return fail(err)
			}
		}
	}
	setup := make([]byte, 92)
	binary.LittleEndian.PutUint16(setup, 3)
	if gamepad {
		binary.LittleEndian.PutUint16(setup[2:], 0x045e)
		binary.LittleEndian.PutUint16(setup[4:], 0x028e)
		binary.LittleEndian.PutUint16(setup[6:], 0x0114)
	} else {
		binary.LittleEndian.PutUint16(setup[2:], 0x1209)
		binary.LittleEndian.PutUint16(setup[4:], 0x0001)
	}
	copy(setup[8:88], name)
	if err = ioctl(fd, 0x405c5503, setup); err != nil {
		return fail(err)
	}
	if err = ioctlValue(fd, 0x5501, 0); err != nil {
		return fail(err)
	}
	return fd, nil
}
func openVirtual() (Virtual, error) {
	p, err := createUinput("Podracer Bridge Xbox 360", padKeys, true)
	if err != nil {
		return nil, err
	}
	k, err := createUinput("Podracer Bridge Keyboard", keyboardKeys, false)
	if err != nil {
		ioctlValue(p, 0x5502, 0)
		syscall.Close(p)
		return nil, err
	}
	return &linuxVirtual{pad: p, key: k, previous: map[string]bool{}}, nil
}
func appendEvent(b []byte, typ, code int, value int32) []byte {
	v := make([]byte, 24)
	binary.LittleEndian.PutUint16(v[16:], uint16(typ))
	binary.LittleEndian.PutUint16(v[18:], uint16(code))
	binary.LittleEndian.PutUint32(v[20:], uint32(value))
	return append(b, v...)
}
func writeEvents(fd int, b []byte) error {
	b = appendEvent(b, 0, 0, 0)
	for len(b) > 0 {
		n, err := syscall.Write(fd, b)
		if err != nil {
			return err
		}
		if n <= 0 {
			return errors.New("virtual device write failed")
		}
		b = b[n:]
	}
	return nil
}
func boolInt(v bool) int32 {
	if v {
		return 1
	}
	return 0
}
func (v *linuxVirtual) Send(o Output) error {
	if v.closed {
		return os.ErrClosed
	}
	p := []byte{}
	k := []byte{}
	steer := int32(clamp(o.Steering, -1, 1) * 32767)
	p = appendEvent(p, 3, 0, steer)
	for _, code := range []int{1, 3, 4} {
		p = appendEvent(p, 3, code, 0)
	}
	p = appendEvent(p, 3, 2, int32(clamp(o.Brake, 0, 1)*255))
	p = appendEvent(p, 3, 5, int32(clamp(o.Throttle, 0, 1)*255))
	p = appendEvent(p, 3, 16, boolInt(o.Held["pad:RIGHT"])-boolInt(o.Held["pad:LEFT"]))
	p = appendEvent(p, 3, 17, boolInt(o.Held["pad:DOWN"])-boolInt(o.Held["pad:UP"]))
	for a, code := range padKeys {
		if o.Held[a] != v.previous[a] {
			p = appendEvent(p, 1, code, boolInt(o.Held[a]))
		}
	}
	for a, code := range keyboardKeys {
		if o.Held[a] != v.previous[a] {
			k = appendEvent(k, 1, code, boolInt(o.Held[a]))
		}
	}
	if err := writeEvents(v.pad, p); err != nil {
		return err
	}
	if len(k) > 0 {
		if err := writeEvents(v.key, k); err != nil {
			return err
		}
	}
	v.previous = o.Held
	return nil
}
func (v *linuxVirtual) Close() error {
	if v.closed {
		return nil
	}
	v.Send(Output{Held: map[string]bool{}})
	v.closed = true
	ioctlValue(v.pad, 0x5502, 0)
	ioctlValue(v.key, 0x5502, 0)
	syscall.Close(v.pad)
	return syscall.Close(v.key)
}
