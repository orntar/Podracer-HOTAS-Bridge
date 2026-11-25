package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIAuthAndOrigin(t *testing.T) {
	a := &app{engine: newEngine(true), dir: t.TempDir(), token: "secret", host: "127.0.0.1:9876", quit: make(chan struct{}, 1)}
	for _, test := range []struct {
		name, host, token, origin, method, path, body string
		status                                        int
	}{{"unauthenticated", a.host, "", "", "GET", "/api/state", "", 401}, {"valid", a.host, "secret", "", "GET", "/api/state", "", 200}, {"foreign origin", a.host, "secret", "https://other.example", "POST", "/api/stop", "{}", 403}, {"dns rebinding", "other.example", "secret", "", "GET", "/api/state", "", 403}, {"GET cannot start", a.host, "secret", "", "GET", "/api/start", "", 405}, {"demo cannot output", a.host, "secret", "", "POST", "/api/start", "{}", 400}, {"trailing JSON", a.host, "secret", "", "POST", "/api/demo", "{\"left\":0,\"right\":0}{}", 400}, {"asset", a.host, "", "", "GET", "/", "", 200}} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, "http://"+test.host+test.path, strings.NewReader(test.body))
			r.Header.Set("X-Bridge-Token", test.token)
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			w := httptest.NewRecorder()
			a.handler().ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
