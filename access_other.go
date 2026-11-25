//go:build !linux || !amd64

package main

import (
	"errors"
	"os"
)

func terminationSignal() os.Signal { return os.Interrupt }
func accessDiagnostics() map[string]any {
	return map[string]any{"uinput_access": false, "usb_devices": []any{}, "executable": "PodracerBridge-x86_64.AppImage"}
}
func installAccess(string) error {
	return errors.New("device access setup is only available on Linux x86-64")
}
