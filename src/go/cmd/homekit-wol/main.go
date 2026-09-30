package main

import (
	"context"
	"flag"
	"log"

	"homekit-devices/internal/hapserver"
	"homekit-devices/internal/wol"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	path := flag.String("config", "config.yaml", "path to YAML configuration")
	flag.Parse()
	cfg, err := wol.Load(*path)
	if err != nil {
		return err
	}
	accessories, err := wol.BuildAccessories(cfg)
	if err != nil {
		return err
	}
	for _, d := range cfg.Devices {
		log.Printf("configured wake target %q (%s) via %s:%d", d.Name, d.MAC, d.BroadcastIP, d.Port)
	}
	return hapserver.Run(context.Background(), cfg.HomeKit, accessories, nil)
}
