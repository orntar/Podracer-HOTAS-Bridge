# Podracer HOTAS Bridge 0.1.0

A portable Linux x86-64 companion for the LINSLABS two-handle rig running the supplied **joystick firmware** (`joystickMode = true`). Includes a local browser dashboard for calibration, steering/throttle response, saved button mappings, and profiles.

This is an independent application. It does not include the purchased LINSLABS firmware or models, and it does not copy JaffaQuake's source code. Its differential mixer is documented below; it is not an exact implementation of the original Windows bridge.

## Start on linux

1. Copy `PodracerBridge-0.1.0-x86_64.AppImage` to a folder in your home directory.
2. In the file's Properties → Permissions, allow execution. Alternatively:
   ```sh
   chmod +x PodracerBridge-0.1.0-x86_64.AppImage
   ./PodracerBridge-0.1.0-x86_64.AppImage
   ```
3. Your default browser opens the local dashboard. The app has no remote service, account, telemetry, or runtime downloads. It uses an operating-system browser rather than bundling one.
4. Plug in the rig, refresh the controller list, select it, and click **Connect**. It is never selected automatically.
5. In **Calibration**, select each handle's axis. Move that handle fully back and capture idle; move fully forward and capture full. Repeat for the other handle. Reversed direction is supported. Output remains off during setup.
6. Apply settings and save the `Default` profile. That name loads automatically on the next launch. Other profiles are available in the Profiles tab.
7. Click **Start output**. Return both handles to idle and release every button/toggle to arm the output.
8. Test **Podracer Bridge Xbox 360** in Steam's controller settings, then launch the game. Use its default controller scheme initially. Confirm left/right steering, throttle, brake, and button actions before racing.

Keep the app running during play. Closing the browser tab does **not** stop the bridge. Stop output before changing games. **Quit app** releases inputs and exits; Ctrl+C in the launching terminal does the same. Do not run the dashboard as root.

The app can run before Steam is started, which can help Steam discover the virtual gamepad. If Steam shows both the Arduino and the virtual gamepad, select the virtual gamepad. The bridge exclusively grabs the physical event interface while output is enabled, but it does not hide every possible USB/hidraw interface from every program.

If the AppImage cannot mount, try:
```sh
./PodracerBridge-0.1.0-x86_64.AppImage --appimage-extract-and-run
```

## One-time device access, only if required

Bazzite can run AppImages without package layering. A controller bridge additionally needs permission to read the selected input device and create virtual devices through `/dev/uinput`.

Open **Device access & help**. Choose the Arduino's actual USB entry and copy the generated setup command. The helper is built into the AppImage and is invoked explicitly with sudo. It:

- Adds `/etc/udev/rules.d/70-podracer-bridge-VENDOR-PRODUCT.rules`, granting the active local seat access to this controller's joystick event interface and `/dev/uinput`.
- Adds `/etc/modules-load.d/podracer-bridge.conf` to load the existing `uinput` kernel module at boot.
- Loads the module and reloads the rules now.

It installs no packages, does not disable SELinux, does not make devices world-writable, and does not add you to the broad `input` group. Virtual-device creation is still a real permission granted to the active local user; AppImage packaging cannot bypass it. The generated command uses extraction mode because FUSE mounting under sudo can otherwise be troublesome.

Unplug/replug the rig afterward. If permissions remain denied, log out/back in or reboot. The app needs a local desktop session with an active seat; this setup does not grant input access to arbitrary SSH sessions. If SELinux or another existing policy still denies access, capture the error for diagnosis rather than disabling the policy.

To remove the setup, delete only the rule created for your rig and the module-load file named above, then reboot. Delete `~/.config/podracer-bridge` separately if you also want to remove profiles. If `XDG_CONFIG_HOME` is set, profiles are stored beneath that directory instead.

## What is configurable

- Separate idle/full endpoint capture for each handle, including reversed axes.
- Independent handle deadzones and response exponents.
- Steering center deadzone, sensitivity, response exponent, and inversion.
- Throttle response exponent and maximum output.
- Thrust preservation through turns.
- Brake override of throttle.
- Held gamepad buttons, trigger buttons, D-pad directions, and the supplied keyboard actions.
- Profile save/load, JSON import/export, and visible raw inputs/output preview.

Tuning controls update their curve preview while dragging. **Apply settings** stops output and updates the engine. Restart output to try the new settings. This deliberately avoids changing steering behavior or held-key mappings mid-race. Saving also applies the settings and stops output.

## Default mappings

| Firmware button | Action |
|---|---|
| 1 | Gamepad LT / brake |
| 2 | Space / maneuver |
| 3 | Left Shift / boost |
| 4 | Left Ctrl / boost mode |
| 5–8 | Keyboard 1–4 / ability selection |
| 9 and 12 | E / ability activation |
| 10 | R / recovery |
| 11 | Insert / camera |
| 13 | Tab / view build |
| 14 | Escape / pause |
| 15–20 | Unassigned |

Both handles feed gamepad left-stick X (steering) and right trigger (throttle). Button 1 drives the left trigger. Keys are logical Linux key codes: the desktop keyboard layout can affect the characters they produce.

The ability-selection sequence comes from the Arduino: it cycles physical outputs 5–8, and the app maps those to keys 1–4. The app does not run a second cycle. Multiple inputs mapped to the same action are combined so releasing one toggle does not release E while the other toggle is still on.

The default profile uses mixed keyboard and gamepad input, matching the intent of the supplied firmware mappings. Actual game behavior under Proton must be tested. If mixed input is rejected, assign gamepad actions and configure the game's bindings accordingly. The app cannot guarantee that every keyboard-only game action has a gamepad equivalent.

## Mixer behavior

After normalization and per-handle deadzone/curve processing, let `L` and `R` range from 0 to 1.

```
base steering = L - R
base throttle = (1 - preserve) * (L + R) / 2 + preserve * max(L, R)
```

The steering deadzone, exponent and sensitivity are then applied, followed by optional inversion. The throttle exponent and output limit are applied separately. A mapped brake can override throttle. No reverse or automatic analog braking is inferred from pulling the handles back: fully back is idle, and the existing trigger provides braking. Preserve thrust defaults to 50%.

The advertised axis range may differ from the 0–32767 range inside the Arduino code because Linux interprets the HID descriptor. Capture actual values, not guessed numbers. The firmware's `forwardOnly = true` uses only half of its reported axis range; the app's calibration accounts for this.

## Recovery and limitations

- The bridge stops output on unplug, dropped input events, write errors, profile changes, or a long processing gap such as suspend. It never automatically reconnects or rearms.
- Every start requires idle handles and released controls. A toggle ON at firmware boot may be suppressed by the firmware until switched OFF once.
- A process killed forcibly cannot run its cleanup code; the OS closes its virtual devices, but game reaction to device removal is game-dependent. Stop normally when possible.
- The bridge does not track game focus. Keyboard actions go to the focused application. The dashboard tab is not a focus lock.
- No rumble, joystick force feedback, centered-stick/reverse mode, on-foot mode, arbitrary macros, or non-Linux hardware backend is included in 0.1.0.
- Calibration is tied to USB vendor/product/name/unique ID when available. Identical boards without unique USB IDs cannot be distinguished by that identity alone; choose the correct physical device explicitly.
- Two-handle mixing and digital brake support are independent implementations, not a full port of every JaffaQuake mode.
- Desktop-mode setup is the initial target. Keeping the process alive across Desktop/Gaming Mode session changes has not been verified; configure and test within the session where you will play. The engine may be launched with `--no-browser` after a profile is saved, but output still requires explicit activation from its dashboard.

## Command-line options

```
--demo                 Preview with simulated handles, no device output
--no-browser           Print the local dashboard URL without opening it
--port 41873           Choose a local port (default: an available port)
--profile NAME         Load a saved profile (default: Default)
--config-dir PATH      Override the profiles directory
--self-test            Check embedded assets and mixer without a display
--install-access V:P   Explicit privileged device-access setup, then exit
```

The server listens on `127.0.0.1` only and requires a random per-launch token for API access. The token is passed in the URL fragment, stored in the browser tab's session storage, and removed from the visible URL. No external fonts/scripts are loaded. A browser is needed to control this release.

## Build and validation

The application uses Go's standard library only. Go 1.24 or newer is required; Linux output targets `amd64` with CGO disabled. The browser assets are embedded in the executable.

```
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o podracer-bridge .
```

`packaging/build-linux.sh` assembles a type-2 AppImage with `mksquashfs` and a separately verified AppImage runtime (`APPIMAGE_RUNTIME`). `packaging/build-windows.ps1` cross-compiles and packages using portable tools beneath `.tools`. Tool provenance and hashes are recorded separately in the distribution.

Tests cover mixer behavior, reversed calibration, deadzones, shared actions, brake override, ability outputs, profiles, invalid configuration, output arming, disconnect/error/suspend cleanup, and local API protections. See `VALIDATION.md` for this build's actual checks and remaining hardware tests. A successful cross-build is not proof of device, Steam, Proton, or Bazzite compatibility.

## References

- LINSLABS project: https://www.linslabs.net/podracing-sim
- Windows bridge: https://github.com/JaffaQuake/Star-Wars-Galactic-Racer-HOSAS-HOTAS-Binder
- Linux uinput API: https://docs.kernel.org/input/uinput.html
- AppImage packaging: https://docs.appimage.org/packaging-guide/manual.html
- Bazzite AppImages: https://docs.bazzite.gg/Installing_and_Managing_Software/AppImage/
