package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

//go:embed web/*
var assets embed.FS

type app struct {
	engine           *Engine
	dir, token, host string
	quit             chan struct{}
}

func (a *app) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var rest any
	if err := d.Decode(&rest); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	files, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(files)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Bridge-Token")), []byte(a.token)) != 1 {
			http.Error(w, "Session expired. Reopen the app's dashboard link.", http.StatusUnauthorized)
			return
		}
		if r.Method != "GET" && r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		fail := func(err error) { http.Error(w, err.Error(), 400) }
		switch r.URL.Path {
		case "/api/state":
			if r.Method != "GET" {
				http.Error(w, "GET required", 405)
				return
			}
			a.json(w, a.engine.snapshot())
		case "/api/devices":
			if r.Method != "GET" {
				http.Error(w, "GET required", 405)
				return
			}
			d, warnings := listDevices()
			a.json(w, map[string]any{"devices": d, "warnings": warnings})
		case "/api/meta":
			if r.Method != "GET" {
				http.Error(w, "GET required", 405)
				return
			}
			a.json(w, map[string]any{"actions": actions, "labels": physicalLabels, "profile_dir": a.dir, "version": version, "access": accessDiagnostics()})
		case "/api/profiles":
			if r.Method != "GET" {
				http.Error(w, "GET required", 405)
				return
			}
			entries, err := os.ReadDir(a.dir)
			if err != nil && !os.IsNotExist(err) {
				fail(err)
				return
			}
			names := []string{}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					n := strings.TrimSuffix(e.Name(), ".json")
					if profileName.MatchString(n) {
						names = append(names, n)
					}
				}
			}
			sort.Strings(names)
			a.json(w, names)
		default:
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			switch r.URL.Path {
			case "/api/select":
				var q struct {
					Path string `json:"path"`
				}
				if err := decode(w, r, &q); err != nil {
					fail(err)
					return
				}
				if err := a.engine.selectDevice(q.Path); err != nil {
					fail(err)
					return
				}
			case "/api/disconnect":
				a.engine.shutdown()
			case "/api/config":
				var c Config
				if err := decode(w, r, &c); err != nil {
					fail(err)
					return
				}
				if err := a.engine.apply(c); err != nil {
					fail(err)
					return
				}
			case "/api/start":
				if err := a.engine.start(); err != nil {
					fail(err)
					return
				}
			case "/api/stop":
				a.engine.stop("Output stopped. You can adjust settings safely.")
			case "/api/save":
				var q struct {
					Name   string `json:"name"`
					Config Config `json:"config"`
				}
				if err := decode(w, r, &q); err != nil {
					fail(err)
					return
				}
				if err := saveProfile(a.dir, q.Name, q.Config); err != nil {
					fail(err)
					return
				}
			case "/api/load":
				var q struct {
					Name string `json:"name"`
				}
				if err := decode(w, r, &q); err != nil {
					fail(err)
					return
				}
				c, err := loadProfile(a.dir, q.Name)
				if err != nil {
					fail(err)
					return
				}
				a.engine.shutdown()
				if err = a.engine.apply(c); err != nil {
					fail(err)
					return
				}
			case "/api/demo":
				var q struct {
					Left    float64      `json:"left"`
					Right   float64      `json:"right"`
					Buttons map[int]bool `json:"buttons"`
				}
				if err := decode(w, r, &q); err != nil {
					fail(err)
					return
				}
				if !finiteRange(q.Left, 0, 1) || !finiteRange(q.Right, 0, 1) {
					fail(errors.New("invalid demo input"))
					return
				}
				a.engine.mu.Lock()
				if !a.engine.demo {
					a.engine.mu.Unlock()
					fail(errors.New("demo is disabled"))
					return
				}
				c := a.engine.cfg
				a.engine.raw = map[int]float64{c.Left.Code: c.Left.Idle + q.Left*(c.Left.Full-c.Left.Idle), c.Right.Code: c.Right.Idle + q.Right*(c.Right.Full-c.Right.Idle)}
				a.engine.buttons = q.Buttons
				a.engine.mu.Unlock()
			case "/api/quit":
				a.engine.shutdown()
				select {
				case a.quit <- struct{}{}:
				default:
				}
			default:
				http.NotFound(w, r)
				return
			}
			a.json(w, map[string]bool{"ok": true})
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != a.host {
			http.Error(w, "invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+a.host {
			http.Error(w, "invalid origin", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func main() {
	demo := flag.Bool("demo", false, "preview without Linux input devices")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser automatically")
	port := flag.Int("port", 0, "local dashboard port (0 chooses a free port)")
	profile := flag.String("profile", "Default", "profile to load on launch")
	configDir := flag.String("config-dir", "", "override profile directory")
	selfTest := flag.Bool("self-test", false, "check the embedded assets and mixer without a display")
	setup := flag.String("install-access", "", "Linux only: install device access rules for vendor:product; run explicitly with sudo")
	flag.Parse()
	if *setup != "" {
		if err := installAccess(*setup); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *selfTest {
		c := defaultConfig()
		o := mix(c, c.Left.Full, c.Right.Full, nil)
		if err := c.Validate(); err != nil || o.Throttle != 1 || o.Steering != 0 {
			log.Fatal("mixer check failed")
		}
		if _, err := assets.ReadFile("web/index.html"); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Podracer Bridge " + version + ": embedded assets and mixer OK")
		return
	}
	if *configDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		*configDir = filepath.Join(base, "podracer-bridge", "profiles")
	}
	e := newEngine(*demo)
	if c, err := loadProfile(*configDir, *profile); err == nil {
		e.cfg = c
	} else if !os.IsNotExist(err) {
		e.message = "Saved profile could not be loaded: " + err.Error() + ". Defaults are in use."
	}
	if *demo {
		e.raw = map[int]float64{e.cfg.Left.Code: e.cfg.Left.Idle, e.cfg.Right.Code: e.cfg.Right.Idle}
		e.message = "Demo mode — no controller or keyboard output is sent."
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	a := &app{engine: e, dir: *configDir, token: hex.EncodeToString(secret), host: listener.Addr().String(), quit: make(chan struct{}, 1)}
	server := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan struct{})
	go e.run(done)
	url := "http://" + a.host + "/#" + a.token
	fmt.Printf("Podracer Bridge %s\nDashboard: %s\nKeep the app running while playing. Use Stop output before changing games.\n", version, url)
	if !*noBrowser {
		go func() {
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			} else {
				cmd = exec.Command("xdg-open", url)
			}
			if err := cmd.Run(); err != nil {
				log.Printf("Open the dashboard URL above in your browser: %v", err)
			}
		}()
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
			select {
			case a.quit <- struct{}{}:
			default:
			}
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, terminationSignal())
	select {
	case <-sig:
	case <-a.quit:
	}
	close(done)
	e.shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
