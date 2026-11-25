package main

import (
	"errors"
	"testing"
	"time"
)

type fakePhysical struct {
	raw             map[int]float64
	buttons         map[int]bool
	err             error
	grabbed, closed bool
}

func (p *fakePhysical) Info() DeviceInfo {
	return DeviceInfo{ID: "test", Axes: []AxisInfo{{Code: 0, Min: 0, Max: 1000}, {Code: 1, Min: 0, Max: 1000}}}
}
func (p *fakePhysical) Read() (map[int]float64, map[int]bool, error) { return p.raw, p.buttons, p.err }
func (p *fakePhysical) Grab(b bool) error                            { p.grabbed = b; return nil }
func (p *fakePhysical) Close() error                                 { p.closed = true; return nil }

type fakeVirtual struct {
	reports []Output
	closed  bool
	err     error
}

func (v *fakeVirtual) Send(o Output) error { v.reports = append(v.reports, o); return v.err }
func (v *fakeVirtual) Close() error        { v.closed = true; return nil }
func fakeEngine() (*Engine, *fakePhysical, *fakeVirtual) {
	e := newEngine(false)
	e.cfg = linear()
	p := &fakePhysical{raw: map[int]float64{0: 100, 1: 100}, buttons: map[int]bool{}, grabbed: true}
	v := &fakeVirtual{}
	e.physical = p
	e.virtual = v
	return e, p, v
}
func TestArmingRequiresIdleAndReleasedButtons(t *testing.T) {
	e, p, v := fakeEngine()
	now := time.Now()
	p.raw[0] = 1000
	e.tick(now)
	if e.armed || len(v.reports) > 0 {
		t.Fatal("armed with thrust applied")
	}
	p.raw[0] = 100
	p.buttons[9] = true
	e.tick(now.Add(time.Millisecond))
	if e.armed {
		t.Fatal("armed with toggle held")
	}
	p.buttons[9] = false
	e.tick(now.Add(2 * time.Millisecond))
	if !e.armed || len(v.reports) != 1 {
		t.Fatal("did not arm at idle")
	}
}
func TestDisconnectReleasesAndCloses(t *testing.T) {
	e, p, v := fakeEngine()
	e.armed = true
	p.err = errors.New("unplugged")
	e.tick(time.Now())
	if !v.closed || !p.closed || p.grabbed || e.virtual != nil || e.physical != nil {
		t.Fatal("disconnect did not clean up")
	}
	last := v.reports[len(v.reports)-1]
	if last.Throttle != 0 || len(last.Held) != 0 {
		t.Fatal("disconnect not neutral")
	}
}
func TestSettingsChangeStopsAndReleases(t *testing.T) {
	e, p, v := fakeEngine()
	c := linear()
	c.DeviceID = "test"
	c.Mappings[9] = "key:R"
	if err := e.apply(c); err != nil {
		t.Fatal(err)
	}
	if !v.closed || p.grabbed || e.armed || e.virtual != nil {
		t.Fatal("settings changed while output remained active")
	}
}
func TestSuspendGapStopsOutput(t *testing.T) {
	e, _, v := fakeEngine()
	e.lastTick = time.Now()
	e.tick(e.lastTick.Add(time.Second))
	if !v.closed || e.virtual != nil {
		t.Fatal("output survived suspend gap")
	}
}
func TestNewVirtualDeviceGetsDiscoveryTime(t *testing.T) {
	e, _, v := fakeEngine()
	now := time.Now()
	e.readyAt = now.Add(100 * time.Millisecond)
	e.tick(now)
	if e.armed || len(v.reports) != 0 {
		t.Fatal("output began before device discovery delay")
	}
	e.tick(now.Add(101 * time.Millisecond))
	if !e.armed {
		t.Fatal("idle device did not arm after discovery delay")
	}
}
func TestVirtualWriteFailureStopsOutput(t *testing.T) {
	e, _, v := fakeEngine()
	v.err = errors.New("write failed")
	e.tick(time.Now())
	if !v.closed || e.virtual != nil {
		t.Fatal("failed output was not closed")
	}
}
func TestSnapshotDoesNotLeakMutableInputMaps(t *testing.T) {
	e, _, _ := fakeEngine()
	e.raw[0] = 123
	s := e.snapshot()
	s.Raw[0] = 999
	if e.raw[0] != 123 {
		t.Fatal("snapshot changed engine input")
	}
}
