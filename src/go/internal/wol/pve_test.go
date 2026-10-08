package wol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPVEClientDiscoveryStatusAndShutdown(t *testing.T) {
	discoveries, shutdowns := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken=homekit@pve!power=secret" {
			t.Error("missing token header")
		}
		switch r.URL.Path {
		case "/api2/json/nodes":
			discoveries++
			w.Write([]byte(`{"data":[{"node":"actual-node"}]}`))
		case "/api2/json/nodes/actual-node/status":
			if r.Method == http.MethodGet {
				w.Write([]byte(`{"data":{"uptime":42}}`))
			} else {
				r.ParseForm()
				if r.Form.Get("command") != "shutdown" {
					t.Error("wrong shutdown command")
				}
				shutdowns++
				w.Write([]byte(`{"data":null}`))
			}
		default:
			t.Error(r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := NewPVEClient(PowerOptions{Host: "192.0.2.10", Token: "homekit@pve!power=secret"})
	defer p.Close()
	p.base = server.URL + "/api2/json" // Keep production transport: accepts PVE self-signed TLS.
	for i := 0; i < 2; i++ {
		if reachable, err := p.Status(context.Background()); !reachable || err != nil {
			t.Fatalf("status: %t %v", reachable, err)
		}
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if discoveries != 1 || shutdowns != 1 {
		t.Fatalf("discovery=%d shutdown=%d", discoveries, shutdowns)
	}
}
func TestPVEFailuresAreReachableAndNeverLeakToken(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"empty nodes", `{"data":[]}`, 200},
		{"multiple nodes", `{"data":[{"node":"a"},{"node":"b"}]}`, 200},
		{"malformed", `not-json`, 200},
		{"unauthorized", `secret-token`, 401},
		{"forbidden", `secret-token`, 403},
		{"server error", `secret-token`, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer s.Close()
			p := NewPVEClient(PowerOptions{Token: "secret-token"})
			defer p.Close()
			p.base = s.URL
			reachable, err := p.Status(context.Background())
			if !reachable || err == nil {
				t.Fatalf("expected reachable API fault: %t %v", reachable, err)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatal("credential leaked")
			}
			if p.node != "" {
				t.Fatal("failed discovery cached")
			}
		})
	}
}
func TestPVEInvalidStatusAndCanceledRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":null}`)) }))
	defer s.Close()
	p := NewPVEClient(PowerOptions{})
	defer p.Close()
	p.node = "pve"
	p.base = s.URL
	if reachable, err := p.Status(context.Background()); !reachable || err == nil {
		t.Fatal("invalid status accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reachable, err := p.Status(ctx); reachable || err == nil {
		t.Fatal("canceled request accepted")
	}
}
func TestPingCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pingHost(ctx, "192.0.2.10"); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected canceled probe error")
	}
}
