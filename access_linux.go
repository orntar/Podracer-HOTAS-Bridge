//go:build linux && amd64

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

func terminationSignal() os.Signal { return syscall.SIGTERM }
func accessDiagnostics() map[string]any {
	devices := []map[string]string{}
	paths, _ := filepath.Glob("/sys/bus/usb/devices/*/idVendor")
	for _, p := range paths {
		base := filepath.Dir(p)
		v, _ := os.ReadFile(p)
		product, _ := os.ReadFile(filepath.Join(base, "idProduct"))
		name, _ := os.ReadFile(filepath.Join(base, "product"))
		maker, _ := os.ReadFile(filepath.Join(base, "manufacturer"))
		if len(name) > 0 {
			devices = append(devices, map[string]string{"id": strings.TrimSpace(string(v)) + ":" + strings.TrimSpace(string(product)), "name": strings.TrimSpace(string(maker)) + " " + strings.TrimSpace(string(name))})
		}
	}
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err == nil {
		f.Close()
	}
	executable := os.Getenv("APPIMAGE")
	if executable == "" {
		executable, _ = os.Executable()
	}
	return map[string]any{"uinput_access": err == nil, "usb_devices": devices, "executable": executable}
}
func installAccess(id string) error {
	if !regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{4}$`).MatchString(id) {
		return errors.New("use the controller's four-digit USB vendor:product ID, such as abcd:1234")
	}
	if os.Geteuid() != 0 {
		return errors.New("this one-time setup must be run with sudo; the dashboard should run as your normal user")
	}
	ids := strings.Split(strings.ToLower(id), ":")
	// A fixed destination and strictly validated IDs prevent arbitrary file writes.
	rule := fmt.Sprintf("# Podracer Bridge: only the selected USB controller's joystick event interface.\nSUBSYSTEM==\"input\", KERNEL==\"event*\", ATTRS{idVendor}==\"%s\", ATTRS{idProduct}==\"%s\", ENV{ID_INPUT_JOYSTICK}==\"1\", TAG+=\"uaccess\"\n# Virtual controller creation for the active local seat.\nSUBSYSTEM==\"misc\", KERNEL==\"uinput\", OPTIONS+=\"static_node=uinput\", TAG+=\"uaccess\"\n", ids[0], ids[1])
	if err := os.MkdirAll("/etc/udev/rules.d", 0755); err != nil {
		return err
	}
	dest := "/etc/udev/rules.d/70-podracer-bridge-" + strings.Join(ids, "-") + ".rules"
	if err := os.WriteFile(dest, []byte(rule), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/modules-load.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("/etc/modules-load.d/podracer-bridge.conf", []byte("uinput\n"), 0644); err != nil {
		return err
	}
	for _, args := range [][]string{{"modprobe", "uinput"}, {"udevadm", "control", "--reload-rules"}, {"udevadm", "trigger", "--action=add", "--subsystem-match=misc", "--sysname-match=uinput"}} {
		b, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("rules were written, but %s failed: %s (%w); reboot before trying again", args[0], b, err)
		}
	}
	fmt.Println("Device access rules saved. Unplug/replug the controller, then launch the AppImage as your normal user. If access is still denied, log out and back in or reboot. No system packages were installed.")
	return nil
}
