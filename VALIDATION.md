# Validation — 0.1.0 first test build

Built on Windows for **Linux x86-64**, November 24, 2026. Target: Bazzite/Fedora with the supplied LINSLABS joystick firmware.

## Passed

- Go unit and HTTP tests: mixing at idle/full/partial travel; steering polarity; thrust preservation; reversed calibration; deadzones and curves; brake override; shared held actions; ability-button outputs; validation of invalid settings; profile round-trip/overwrite/path restrictions; corrupt-profile preservation.
- Engine lifecycle tests with simulated devices: idle/released arming, disconnect cleanup, settings-change cleanup, processing-gap/suspend stop, virtual-output error cleanup, snapshot isolation.
- Local API checks: authentication, Host/Origin protections, read-only GET requests, malformed JSON rejection, demo output prevention, embedded page serving.
- `go vet` on both Windows and Linux targets; Linux executable and Linux test binary cross-compilation with CGO disabled.
- JavaScript syntax validation.
- Hidden Chrome dashboard test with the real application running in demo mode: both handle calibrations, simulated input meters, steering and throttle settings, button remapping, profile save/load, JSON export/import, help panel, and narrow-screen layout. No browser JavaScript errors.
- Visual review of wide and narrow dashboard screenshots.
- Type-2 AppImage signature and appended SquashFS magic checked. SquashFS filesystem creation and metadata validation completed. Final archive contents and executable modes are listed in `package-manifest.txt` in the distribution.
- Official Go archive checksum matched the published checksum. AppImage runtime checksum matched the GitHub release asset digest. Tool hashes and runtime source commit are recorded in `BUILD_PROVENANCE.json`.

## Not yet verified

There is no Linux runtime or connected rig in this Windows workspace. The following require testing on the user's Bazzite computer:

- Running the AppImage runtime itself, including FUSE mount/extraction and default-browser launch.
- Native evdev/uinput calls, USB device identification and button numbering with the actual firmware.
- Active-seat device permissions, the optional setup helper, and SELinux interaction on the user's installation.
- Steam recognition and game behavior under Proton, particularly mixed keyboard and gamepad actions.
- Hotplug/suspend behavior with real hardware; Desktop/Gaming Mode session lifetime; latency and long sessions.

This is a **first hardware-test build**, not a claim of verified Bazzite/game compatibility. All game-facing output starts disabled. Calibration and explicit Start are required.

## Recommended first hardware test

1. Launch in Desktop Mode. Confirm the rig is listed; use Device access & help only if access is missing.
2. Check that moving each handle affects its own raw axis, and that physical buttons highlight the expected mapping rows.
3. Calibrate both endpoints, apply, and save Default.
4. Start output, idle both handles, and release all controls. Confirm the virtual gamepad in Steam's controller test.
5. Verify both forward = throttle, differential travel = steering, left trigger = brake, and every mapped action.
6. Check the two E toggles together: releasing either one must not release the action while the other remains on.
7. Stop output and confirm inputs release. Restart, unplug the rig, and confirm output disappears/stops. Reconnect manually.
8. Finally test the intended game and adjust its Steam Input setting/bindings as needed.
