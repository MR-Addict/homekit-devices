package poweragent

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultPort = 32045

type Config struct {
	ListenAddress string `yaml:"listen_address"`
	Token         string `yaml:"token"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read agent config: %w", err)
	}
	var c Config
	d := yaml.NewDecoder(strings.NewReader(string(data)))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil {
		return c, errors.New("invalid agent YAML configuration")
	}
	if c.Token == "" || strings.ContainsAny(c.Token, " \t\r\n") {
		return c, errors.New("token must be a nonempty bearer token")
	}
	if c.ListenAddress == "" {
		c.ListenAddress = fmt.Sprintf(":%d", DefaultPort)
	}
	_, port, err := net.SplitHostPort(c.ListenAddress)
	if err != nil || port == "" {
		return c, errors.New("listen_address must be host:port")
	}
	return c, nil
}

type Status struct {
	Hostname string `json:"hostname"`
	BootID   string `json:"boot_id"`
	State    string `json:"state"`
}

// Backend separates the HTTP protocol from operating-system power control.
type Backend interface{ Shutdown(context.Context) error }

type Agent struct {
	mu        sync.Mutex
	tokenHash [32]byte
	status    Status
	backend   Backend
}

func New(token, hostname, bootID string, backend Backend) *Agent {
	return &Agent{tokenHash: sha256.Sum256([]byte("Bearer " + token)), status: Status{Hostname: hostname, BootID: bootID, State: "running"}, backend: backend}
}

func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		status := a.status
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.status.State != "shutting_down" {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := a.backend.Shutdown(ctx); err != nil {
				http.Error(w, "system refused shutdown", http.StatusServiceUnavailable)
				return
			}
			a.status.State = "shutting_down"
		}
		w.WriteHeader(http.StatusAccepted)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(hash[:], a.tokenHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}
