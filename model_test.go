package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %v, want %v", got, want)
	}
}
func linear() Config {
	c := defaultConfig()
	c.Left = AxisConfig{Code: 0, Idle: 100, Full: 1000, Curve: 1}
	c.Right = AxisConfig{Code: 1, Idle: 100, Full: 1000, Curve: 1}
	c.SteeringDeadzone = 0
	return c
}
func TestMixerTravelAndSteering(t *testing.T) {
	c := linear()
	for _, test := range []struct {
		name                  string
		l, r, throttle, steer float64
	}{{"idle", 100, 100, 0, 0}, {"full", 1000, 1000, 1, 0}, {"half", 550, 550, .5, 0}, {"left leads", 1000, 100, .75, 1}, {"right leads", 100, 1000, .75, -1}, {"outside stops", 1200, 1200, 1, 0}} {
		t.Run(test.name, func(t *testing.T) {
			o := mix(c, test.l, test.r, nil)
			near(t, o.Throttle, test.throttle)
			near(t, o.Steering, test.steer)
		})
	}
}
func TestPreserveThrust(t *testing.T) {
	c := linear()
	for _, test := range []struct{ blend, want float64 }{{0, .5}, {.5, .75}, {1, 1}} {
		c.PreserveThrust = test.blend
		near(t, mix(c, 1000, 100, nil).Throttle, test.want)
	}
}
func TestReversedCalibration(t *testing.T) {
	a := AxisConfig{Idle: 1000, Full: 100, Curve: 1}
	near(t, normalize(1000, a), 0)
	near(t, normalize(100, a), 1)
	near(t, normalize(550, a), .5)
	near(t, normalize(1100, a), 0)
	near(t, normalize(0, a), 1)
}
func TestDeadzoneCurvesAndLimits(t *testing.T) {
	c := linear()
	c.Left.Deadzone = .1
	near(t, normalize(180, c.Left), 0)
	near(t, normalize(595, c.Left), .5)
	c = linear()
	c.ThrottleCurve = 2
	c.ThrottleLimit = .8
	near(t, mix(c, 550, 550, nil).Throttle, .2)
	c.SteeringGain = 4
	near(t, mix(c, 1000, 100, nil).Steering, 1)
	c.InvertSteering = true
	near(t, mix(c, 1000, 100, nil).Steering, -1)
}
func TestBrakeAndSharedButtons(t *testing.T) {
	c := linear()
	o := mix(c, 1000, 1000, map[int]bool{1: true})
	near(t, o.Brake, 1)
	near(t, o.Throttle, 0)
	c.BrakeCutsThrottle = false
	near(t, mix(c, 1000, 1000, map[int]bool{1: true}).Throttle, 1)
	for _, held := range []map[int]bool{{9: true, 12: true}, {9: false, 12: true}, {9: true, 12: false}} {
		if !mix(c, 100, 100, held).Held["key:E"] {
			t.Fatal("shared ability released too early")
		}
	}
	if mix(c, 100, 100, map[int]bool{}).Held["key:E"] {
		t.Fatal("ability remained held")
	}
}
func TestAbilityCycleOutputs(t *testing.T) {
	c := linear()
	for n, a := range map[int]string{5: "key:1", 6: "key:2", 7: "key:3", 8: "key:4"} {
		o := mix(c, 100, 100, map[int]bool{n: true})
		if !o.Held[a] || len(o.Held) != 1 {
			t.Fatalf("button %d: %#v", n, o.Held)
		}
	}
}
func TestInvalidConfig(t *testing.T) {
	cases := []func(*Config){func(c *Config) { c.Left.Full = c.Left.Idle }, func(c *Config) { c.Left.Code = c.Right.Code }, func(c *Config) { c.ThrottleCurve = math.NaN() }, func(c *Config) { c.PreserveThrust = 2 }, func(c *Config) { c.SteeringGain = math.Inf(1) }, func(c *Config) { c.Mappings[1] = "key:NOT_REAL" }, func(c *Config) { c.Mappings[0] = "none" }, func(c *Config) { c.Version = 99 }}
	for i, change := range cases {
		c := defaultConfig()
		change(&c)
		if c.Validate() == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
	}
}
func TestProfileRoundTripOverwriteAndTraversal(t *testing.T) {
	dir := t.TempDir()
	c := defaultConfig()
	c.Left.Calibrated = true
	c.DeviceID = "2341:8037:Arduino:"
	if err := saveProfile(dir, "Default", c); err != nil {
		t.Fatal(err)
	}
	c.SteeringGain = 2
	if err := saveProfile(dir, "Default", c); err != nil {
		t.Fatal(err)
	}
	got, err := loadProfile(dir, "Default")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(c)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		t.Fatal("profile changed on round-trip")
	}
	for _, name := range []string{"../escape", "/tmp/escape", "..", "a/b", "a\\b", ""} {
		if saveProfile(dir, name, c) == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".profile-*"))
	if len(matches) > 0 {
		t.Fatal("temporary files left behind")
	}
}
func TestCorruptProfileIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Broken.json")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, err := loadProfile(dir, "Broken"); err == nil {
		t.Fatal("accepted corrupt profile")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken" {
		t.Fatal("corrupt profile was modified")
	}
}
