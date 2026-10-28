package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"homekit-devices/internal/poweragent"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	path := flag.String("config", "config.yaml", "path to agent YAML configuration")
	flag.Parse()
	cfg, err := poweragent.Load(*path)
	if err != nil {
		return err
	}
	backend, bootID, err := poweragent.NewSystemBackend()
	if err != nil {
		return err
	}
	defer backend.Close()
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.ListenAddress, Handler: poweragent.New(cfg.Token, hostname, bootID, backend).Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
			return
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("power-agent listening on %s", cfg.ListenAddress)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
