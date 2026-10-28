package wol

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"homekit-devices/internal/poweragent"
)

func TestAgentClientProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		valid      bool
	}{
		{"running", `{"hostname":"pc","boot_id":"id","state":"running"}`, 200, true},
		{"shutting_down", `{"hostname":"pc","boot_id":"id","state":"shutting_down"}`, 200, true},
		{"missing fields", `{"state":"running"}`, 200, false},
		{"invalid json", `broken`, 200, false},
		{"unauthorized", ``, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing authentication")
				}
				if r.URL.Path == "/api/shutdown" {
					if r.Method != "POST" {
						t.Error("wrong method")
					}
					w.WriteHeader(202)
					return
				}
				if r.URL.Path != "/api/status" || r.Method != "GET" {
					t.Error("wrong endpoint")
				}
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
			p, _ := strconv.Atoi(port)
			c := NewAgentClient(PowerOptions{Host: host, Port: p, Token: "secret"})
			defer c.Close()
			reachable, err := c.Status(context.Background())
			if !reachable || (err == nil) != tc.valid {
				t.Fatalf("status=%t %v", reachable, err)
			}
			if err := c.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestAgentTimeoutAndRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.WriteHeader(200)
			return
		}
		http.Redirect(w, r, "/redirect", 302)
	}))
	defer srv.Close()
	c := NewAgentClient(PowerOptions{Host: "192.0.2.1", Token: "secret"})
	defer c.Close()
	if c.base != "http://192.0.2.1:32045" {
		t.Fatal(c.base)
	}
	c.base = srv.URL
	reachable, err := c.Status(context.Background())
	if !reachable || err == nil {
		t.Fatal("redirect accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if _, err := c.Status(ctx); err == nil {
		t.Fatal("deadline ignored")
	}
}
func TestAgentConfiguration(t *testing.T) {
	for _, tc := range []struct {
		token string
		port  int
		valid bool
	}{{"secret", 0, true}, {"secret", 1234, true}, {"", 0, false}, {"a\nb", 0, false}, {"secret", 65536, false}, {"secret", -1, false}} {
		cfg := Config{Devices: []DeviceConfig{{Name: "PC", MAC: "02:00:00:00:00:01", Options: &PowerOptions{Type: "agent", Host: "192.0.2.1", Token: tc.token, Port: tc.port}}}}
		cfg.applyDefaults()
		if err := cfg.Validate(); (err == nil) != tc.valid {
			t.Fatalf("valid=%t err=%v", tc.valid, err)
		}
	}
	c := NewAgentClient(PowerOptions{Host: "192.0.2.1"})
	if !strings.HasSuffix(c.base, strconv.Itoa(poweragent.DefaultPort)) {
		t.Fatal(c.base)
	}
	c.Close()
}
