package poweragent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeBackend struct {
	calls atomic.Int32
	err   error
}

func (b *fakeBackend) Shutdown(context.Context) error { b.calls.Add(1); return b.err }
func request(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAuthenticationAndMethods(t *testing.T) {
	b := &fakeBackend{}
	h := New("secret", "host", "boot", b).Handler()
	for _, path := range []string{"/api/status", "/api/shutdown"} {
		for _, token := range []string{"", "wrong"} {
			if w := request(h, "POST", path, token); w.Code != 401 {
				t.Fatalf("unauthorized=%d", w.Code)
			}
		}
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{{"POST", "/api/status", 405}, {"GET", "/api/shutdown", 405}, {"GET", "/v1/status", 404}} {
		if w := request(h, tc.method, tc.path, "secret"); w.Code != tc.code {
			t.Fatalf("%s %s = %d", tc.method, tc.path, w.Code)
		}
	}
	if b.calls.Load() != 0 {
		t.Fatal("unauthorized call executed shutdown")
	}
}
func TestConcurrentShutdownAndStatus(t *testing.T) {
	b := &fakeBackend{}
	h := New("secret", "host", "boot", b).Handler()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := request(h, "POST", "/api/shutdown", "secret"); w.Code != 202 {
				t.Errorf("shutdown=%d", w.Code)
			}
		}()
	}
	wg.Wait()
	if b.calls.Load() != 1 {
		t.Fatal("shutdown not merged")
	}
	var status Status
	if err := json.Unmarshal(request(h, "GET", "/api/status", "secret").Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "shutting_down" || status.BootID != "boot" || status.Hostname != "host" {
		t.Fatalf("status=%+v", status)
	}
}
func TestShutdownFailureCanRetry(t *testing.T) {
	b := &fakeBackend{err: errors.New("blocked")}
	h := New("secret", "host", "boot", b).Handler()
	if w := request(h, "POST", "/api/shutdown", "secret"); w.Code != 503 {
		t.Fatal(w.Code)
	}
	var status Status
	_ = json.Unmarshal(request(h, "GET", "/api/status", "secret").Body.Bytes(), &status)
	if status.State != "running" {
		t.Fatal("failure marked shutdown")
	}
	b.err = nil
	if w := request(h, "POST", "/api/shutdown", "secret"); w.Code != 202 {
		t.Fatal(w.Code)
	}
}
func TestConfig(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
		addr  string
	}{
		{"token: secret", true, ":32045"}, {"token: secret\nlisten_address: '127.0.0.1:1234'", true, "127.0.0.1:1234"},
		{"token: ''", false, ""}, {"token: 'a b'", false, ""}, {"token: secret\nlisten_address: invalid", false, ""}, {"token: secret\nunknown: true", false, ""},
	} {
		p := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(p, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(p)
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%t err=%v", tc.valid, err)
		}
		if tc.valid && c.ListenAddress != tc.addr {
			t.Fatal(c.ListenAddress)
		}
	}
}
