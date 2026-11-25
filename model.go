package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
)

const version = "0.1.0"

type AxisConfig struct {
	Code       int     `json:"code"`
	Idle       float64 `json:"idle"`
	Full       float64 `json:"full"`
	Calibrated bool    `json:"calibrated"`
	Deadzone   float64 `json:"deadzone"`
	Curve      float64 `json:"curve"`
}
type Config struct {
	Version           int            `json:"version"`
	DeviceID          string         `json:"device_id"`
	Left              AxisConfig     `json:"left"`
	Right             AxisConfig     `json:"right"`
	ThrottleCurve     float64        `json:"throttle_curve"`
	ThrottleLimit     float64        `json:"throttle_limit"`
	PreserveThrust    float64        `json:"preserve_thrust"`
	SteeringCurve     float64        `json:"steering_curve"`
	SteeringGain      float64        `json:"steering_gain"`
	SteeringDeadzone  float64        `json:"steering_deadzone"`
	InvertSteering    bool           `json:"invert_steering"`
	BrakeCutsThrottle bool           `json:"brake_cuts_throttle"`
	Mappings          map[int]string `json:"mappings"`
}
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var actions = []Action{
	{"none", "Unassigned"}, {"pad:LT", "Gamepad • brake / left trigger"}, {"pad:RT", "Gamepad • right trigger"},
	{"pad:A", "Gamepad • A"}, {"pad:B", "Gamepad • B"}, {"pad:X", "Gamepad • X"}, {"pad:Y", "Gamepad • Y"},
	{"pad:LB", "Gamepad • left bumper"}, {"pad:RB", "Gamepad • right bumper"},
	{"pad:BACK", "Gamepad • Back"}, {"pad:START", "Gamepad • Start"}, {"pad:GUIDE", "Gamepad • Guide"},
	{"pad:LS", "Gamepad • left stick click"}, {"pad:RS", "Gamepad • right stick click"},
	{"pad:UP", "Gamepad • D-pad up"}, {"pad:DOWN", "Gamepad • D-pad down"}, {"pad:LEFT", "Gamepad • D-pad left"}, {"pad:RIGHT", "Gamepad • D-pad right"},
	{"key:1", "Keyboard • 1 (ability 1)"}, {"key:2", "Keyboard • 2 (ability 2)"}, {"key:3", "Keyboard • 3 (ability 3)"}, {"key:4", "Keyboard • 4 (ability 4)"},
	{"key:SPACE", "Keyboard • Space (maneuver)"}, {"key:SHIFT", "Keyboard • Left Shift (boost)"}, {"key:CTRL", "Keyboard • Left Ctrl (boost mode)"},
	{"key:E", "Keyboard • E (activate ability)"}, {"key:R", "Keyboard • R (recovery)"}, {"key:S", "Keyboard • S (brake)"},
	{"key:TAB", "Keyboard • Tab (view build)"}, {"key:ESC", "Keyboard • Escape (pause)"}, {"key:INSERT", "Keyboard • Insert (camera)"},
	{"key:ENTER", "Keyboard • Enter"}, {"key:W", "Keyboard • W"}, {"key:A", "Keyboard • A"}, {"key:D", "Keyboard • D"}, {"key:Q", "Keyboard • Q"},
	{"key:F", "Keyboard • F"}, {"key:C", "Keyboard • C"}, {"key:V", "Keyboard • V"}, {"key:UP", "Keyboard • Up"}, {"key:DOWN", "Keyboard • Down"}, {"key:LEFT", "Keyboard • Left"}, {"key:RIGHT", "Keyboard • Right"},
}
var physicalLabels = map[int]string{1: "Left trigger · brake", 2: "Right trigger · maneuver", 3: "Left thumb · boost", 4: "Right thumb · boost mode", 5: "Ability selector · 1", 6: "Ability selector · 2", 7: "Ability selector · 3", 8: "Ability selector · 4", 9: "Left toggle · ability", 10: "Right button · recovery", 11: "Right button · camera", 12: "Right toggle · ability", 13: "Left extra · view build", 14: "Left extra · pause"}

func defaultConfig() Config {
	m := map[int]string{1: "pad:LT", 2: "key:SPACE", 3: "key:SHIFT", 4: "key:CTRL", 5: "key:1", 6: "key:2", 7: "key:3", 8: "key:4", 9: "key:E", 10: "key:R", 11: "key:INSERT", 12: "key:E", 13: "key:TAB", 14: "key:ESC"}
	for i := 15; i <= 20; i++ {
		m[i] = "none"
	}
	return Config{Version: 1, Left: AxisConfig{Code: 0, Idle: 16384, Full: 32767, Deadzone: .02, Curve: 1}, Right: AxisConfig{Code: 1, Idle: 16384, Full: 32767, Deadzone: .02, Curve: 1}, ThrottleCurve: 1, ThrottleLimit: 1, PreserveThrust: .5, SteeringCurve: 1, SteeringGain: 1, SteeringDeadzone: .03, BrakeCutsThrottle: true, Mappings: m}
}
func finiteRange(x, min, max float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= min && x <= max
}
func (c Config) Validate() error {
	if c.Mappings == nil {
		return errors.New("profile is missing button mappings")
	}
	if c.Version != 1 {
		return errors.New("unsupported profile version")
	}
	for _, a := range []AxisConfig{c.Left, c.Right} {
		if a.Code < 0 || a.Code > 63 || !finiteRange(a.Idle, -2147483648, 2147483647) || !finiteRange(a.Full, -2147483648, 2147483647) || math.Abs(a.Full-a.Idle) < 10 {
			return errors.New("each handle needs distinct idle and full positions (at least 10 raw counts apart)")
		}
		if !finiteRange(a.Deadzone, 0, .25) || !finiteRange(a.Curve, .25, 4) {
			return errors.New("invalid handle deadzone or response curve")
		}
	}
	if c.Left.Code == c.Right.Code {
		return errors.New("select a different axis for each handle")
	}
	if !finiteRange(c.ThrottleCurve, .25, 4) || !finiteRange(c.ThrottleLimit, 0, 1) || !finiteRange(c.PreserveThrust, 0, 1) || !finiteRange(c.SteeringCurve, .25, 4) || !finiteRange(c.SteeringGain, .1, 4) || !finiteRange(c.SteeringDeadzone, 0, .4) {
		return errors.New("tuning value outside supported range")
	}
	if len(c.DeviceID) > 512 {
		return errors.New("invalid device identity")
	}
	for n, v := range c.Mappings {
		if n < 1 || n > 128 {
			return errors.New("invalid button number")
		}
		found := false
		for _, a := range actions {
			if a.ID == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown action %q", v)
		}
	}
	return nil
}
func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
func normalize(raw float64, a AxisConfig) float64 {
	x := clamp((raw-a.Idle)/(a.Full-a.Idle), 0, 1)
	return math.Pow(clamp((x-a.Deadzone)/(1-a.Deadzone), 0, 1), a.Curve)
}

type Output struct {
	Left     float64         `json:"left"`
	Right    float64         `json:"right"`
	Steering float64         `json:"steering"`
	Throttle float64         `json:"throttle"`
	Brake    float64         `json:"brake"`
	Held     map[string]bool `json:"held"`
}

// Independent mixer: thrust blends the mean and maximum lever position.
// This is intentionally not claimed to duplicate JaffaQuake's hybrid algorithm.
func mix(c Config, leftRaw, rightRaw float64, buttons map[int]bool) Output {
	l, r := normalize(leftRaw, c.Left), normalize(rightRaw, c.Right)
	s := l - r
	s = math.Copysign(math.Pow(clamp((math.Abs(s)-c.SteeringDeadzone)/(1-c.SteeringDeadzone), 0, 1), c.SteeringCurve), s)
	s = clamp(s*c.SteeringGain, -1, 1)
	if c.InvertSteering {
		s = -s
	}
	t := ((l+r)/2)*(1-c.PreserveThrust) + math.Max(l, r)*c.PreserveThrust
	o := Output{Left: l, Right: r, Steering: s, Throttle: math.Pow(t, c.ThrottleCurve) * c.ThrottleLimit, Held: map[string]bool{}}
	for n, pressed := range buttons {
		if pressed {
			a := c.Mappings[n]
			if a != "" && a != "none" {
				o.Held[a] = true
			}
		}
	}
	if o.Held["pad:RT"] {
		o.Throttle = c.ThrottleLimit
	}
	if o.Held["pad:LT"] {
		o.Brake = 1
	}
	if c.BrakeCutsThrottle && (o.Brake > 0 || o.Held["key:S"]) {
		o.Throttle = 0
	}
	return o
}

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]{0,47}$`)

func saveProfile(dir, name string, c Config) error {
	if !profileName.MatchString(name) {
		return errors.New("profile name: use 1–48 letters, numbers, spaces, hyphens or underscores")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".profile-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name+".json"))
}
func loadProfile(dir, name string) (Config, error) {
	if !profileName.MatchString(name) {
		return Config{}, errors.New("invalid profile name")
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return Config{}, err
	}
	if len(b) > 65536 {
		return Config{}, errors.New("profile too large")
	}
	var c Config
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
