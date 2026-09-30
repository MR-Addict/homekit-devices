package hapserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
)

// Run owns the HomeKit server and an optional background worker. It waits for
// the worker to exit before returning, so no polling survives server shutdown.
func Run(parent context.Context, cfg Config, accessories []*accessory.A, worker func(context.Context)) error {
	if problems := cfg.Problems(); len(problems) > 0 {
		return fmt.Errorf("HomeKit config: %v", problems)
	}
	if len(accessories) == 0 {
		return fmt.Errorf("at least one accessory is required")
	}
	if err := os.MkdirAll(cfg.StoragePath, 0700); err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	server, err := hap.NewServer(hap.NewFsStore(cfg.StoragePath), accessories[0], accessories[1:]...)
	if err != nil {
		return fmt.Errorf("create HomeKit server: %w", err)
	}
	server.Pin = cfg.Pin
	if cfg.ListenAddress != "" {
		server.Addr = cfg.ListenAddress
	}
	if len(cfg.Interfaces) > 0 {
		server.Ifaces = cfg.Interfaces
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	if worker != nil {
		go func() { defer close(done); worker(ctx) }()
	} else {
		close(done)
	}
	log.Printf("HomeKit %q ready; pair with pin %s-%s-%s", cfg.Name, cfg.Pin[:3], cfg.Pin[3:5], cfg.Pin[5:])
	err = server.ListenAndServe(ctx)
	shuttingDown := ctx.Err() != nil
	cancel()
	<-done
	if err != nil && !shuttingDown && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve HomeKit: %w", err)
	}
	return nil
}
