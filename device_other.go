//go:build !linux || !amd64

package main

import "errors"

func listDevices() ([]DeviceInfo, []string) {
	return []DeviceInfo{}, []string{"Hardware output requires Linux x86-64. Use --demo to preview the dashboard."}
}
func openPhysical(string) (Physical, error) {
	return nil, errors.New("hardware input requires Linux x86-64")
}
func openVirtual() (Virtual, error) { return nil, errors.New("hardware output requires Linux x86-64") }
