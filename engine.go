package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type AxisInfo struct {
	Code  int    `json:"code"`
	Name  string `json:"name"`
	Min   int32  `json:"min"`
	Max   int32  `json:"max"`
	Value int32  `json:"value"`
}
type DeviceInfo struct {
	Path    string     `json:"path"`
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Vendor  uint16     `json:"vendor"`
	Product uint16     `json:"product"`
	Axes    []AxisInfo `json:"axes"`
	Buttons int        `json:"buttons"`
}
type Physical interface {
	Info() DeviceInfo
	Read() (map[int]float64, map[int]bool, error)
	Grab(bool) error
	Close() error
}
type Virtual interface {
	Send(Output) error
	Close() error
}
type Snapshot struct {
	Version string          `json:"version"`
	Config  Config          `json:"config"`
	Device  *DeviceInfo     `json:"device"`
	Raw     map[int]float64 `json:"raw"`
	Buttons map[int]bool    `json:"buttons"`
	Preview Output          `json:"preview"`
	Active  bool            `json:"active"`
	Armed   bool            `json:"armed"`
	Message string          `json:"message"`
	Demo    bool            `json:"demo"`
}
type Engine struct {
	mu       sync.Mutex
	cfg      Config
	physical Physical
	virtual  Virtual
	raw      map[int]float64
	buttons  map[int]bool
	armed    bool
	message  string
	demo     bool
	lastTick time.Time
	readyAt  time.Time
}

func newEngine(demo bool) *Engine {
	return &Engine{cfg: defaultConfig(), raw: map[int]float64{}, buttons: map[int]bool{}, message: "Connect your controller, select it, then calibrate both handles.", demo: demo}
}
func (e *Engine) stopLocked(message string) {
	if e.virtual != nil {
		e.virtual.Send(Output{Held: map[string]bool{}})
		e.virtual.Close()
		e.virtual = nil
	}
	if e.physical != nil {
		e.physical.Grab(false)
	}
	e.armed = false
	e.message = message
}
func (e *Engine) stop(message string) { e.mu.Lock(); defer e.mu.Unlock(); e.stopLocked(message) }
func (e *Engine) shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked("Stopped")
	if e.physical != nil {
		e.physical.Close()
		e.physical = nil
	}
}
func (e *Engine) selectDevice(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked("Output stopped.")
	if e.physical != nil {
		e.physical.Close()
		e.physical = nil
	}
	e.raw = map[int]float64{}
	e.buttons = map[int]bool{}
	p, err := openPhysical(path)
	if err != nil {
		e.message = err.Error()
		return err
	}
	e.physical = p
	info := p.Info()
	if e.cfg.DeviceID != info.ID {
		e.cfg.DeviceID = info.ID
		for i, a := range info.Axes {
			if i > 1 {
				break
			}
			ac := AxisConfig{Code: a.Code, Idle: (float64(a.Min) + float64(a.Max)) / 2, Full: float64(a.Max), Deadzone: .02, Curve: 1}
			if i == 0 {
				e.cfg.Left = ac
			} else {
				e.cfg.Right = ac
			}
		}
	}
	e.raw, e.buttons, err = p.Read()
	if err != nil {
		p.Close()
		e.physical = nil
		return err
	}
	e.message = "Connected. Capture idle and full travel for each handle before starting."
	return nil
}
func (e *Engine) apply(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.physical != nil && e.physical.Info().ID != c.DeviceID {
		return errors.New("this profile belongs to a different controller; disconnect before loading it")
	}
	e.stopLocked("Settings applied. Start output when ready.")
	e.cfg = c
	return nil
}
func (e *Engine) start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.demo {
		return errors.New("demo mode cannot send controller input")
	}
	if e.virtual != nil {
		return nil
	}
	if e.physical == nil {
		return errors.New("select a controller first")
	}
	if err := e.cfg.Validate(); err != nil {
		return err
	}
	if !e.cfg.Left.Calibrated || !e.cfg.Right.Calibrated {
		return errors.New("calibrate both handles first")
	}
	for _, a := range []AxisConfig{e.cfg.Left, e.cfg.Right} {
		found := false
		for _, i := range e.physical.Info().Axes {
			if i.Code == a.Code {
				found = true
				lo, hi := float64(i.Min), float64(i.Max)
				if a.Idle < lo || a.Idle > hi || a.Full < lo || a.Full > hi {
					return errors.New("calibration is outside this device's reported range")
				}
			}
		}
		if !found {
			return errors.New("configured handle axis is missing")
		}
	}
	if err := e.physical.Grab(true); err != nil {
		return fmt.Errorf("cannot reserve controller exclusively: %w", err)
	}
	v, err := openVirtual()
	if err != nil {
		e.physical.Grab(false)
		return err
	}
	e.virtual = v
	e.armed = false
	e.lastTick = time.Now()
	e.readyAt = e.lastTick.Add(time.Second)
	e.message = "Waiting: return both handles to idle and release every button/toggle."
	return nil
}
func (e *Engine) tick(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.virtual != nil && !e.lastTick.IsZero() && now.Sub(e.lastTick) > 500*time.Millisecond {
		e.stopLocked("Output stopped after a pause or suspend. Start again when ready.")
	}
	e.lastTick = now
	if e.physical == nil {
		return
	}
	raw, buttons, err := e.physical.Read()
	if err != nil {
		e.stopLocked("Controller disconnected or input lost. Refresh and reconnect. " + err.Error())
		e.physical.Close()
		e.physical = nil
		e.raw = map[int]float64{}
		e.buttons = map[int]bool{}
		return
	}
	e.raw, e.buttons = raw, buttons
	if e.virtual == nil {
		return
	}
	o := mix(e.cfg, raw[e.cfg.Left.Code], raw[e.cfg.Right.Code], buttons)
	if !e.armed {
		// Allow desktop/Steam consumers to discover the new virtual devices.
		if now.Before(e.readyAt) {
			return
		}
		idle := o.Left <= .005 && o.Right <= .005
		for _, b := range buttons {
			if b {
				idle = false
			}
		}
		if !idle {
			return
		}
		e.armed = true
		e.message = "Output active. Keep this app running while you play."
	}
	if err = e.virtual.Send(o); err != nil {
		e.stopLocked("Virtual controller failed: " + err.Error())
	}
}
func (e *Engine) snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	raw := map[int]float64{}
	for k, v := range e.raw {
		raw[k] = v
	}
	b := map[int]bool{}
	for k, v := range e.buttons {
		b[k] = v
	}
	var d *DeviceInfo
	if e.physical != nil {
		info := e.physical.Info()
		d = &info
	}
	// Config/maps are immutable once applied; mutable live state is copied above.
	o := Output{Held: map[string]bool{}}
	if d != nil || e.demo {
		o = mix(e.cfg, raw[e.cfg.Left.Code], raw[e.cfg.Right.Code], b)
	}
	return Snapshot{Version: version, Config: e.cfg, Device: d, Raw: raw, Buttons: b, Preview: o, Active: e.virtual != nil, Armed: e.armed, Message: e.message, Demo: e.demo}
}
func (e *Engine) run(done <-chan struct{}) {
	t := time.NewTicker(4 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			e.tick(time.Now())
		}
	}
}
