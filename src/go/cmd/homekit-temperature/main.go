package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/brutella/hap/accessory"

	"homekit-devices/internal/hapserver"
	"homekit-devices/internal/temperature"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	path := flag.String("config", "config.yaml", "path to YAML configuration")
	list := flag.Bool("list-sensors", false, "list Linux thermal sensors and exit without starting HomeKit")
	flag.Parse()
	if *list {
		sensors, err := temperature.Discover("/sys/class/thermal")
		if err != nil {
			return err
		}
		for _, s := range sensors {
			if s.Err != nil {
				fmt.Printf("%s\t%s\terror: %v\n", s.Type, s.Path, s.Err)
			} else {
				fmt.Printf("%s\t%s\t%.1f°C\n", s.Type, s.Path, s.Celsius)
			}
		}
		return nil
	}
	cfg, err := temperature.Load(*path)
	if err != nil {
		return err
	}
	monitor, err := temperature.New(cfg)
	if err != nil {
		return err
	}
	return hapserver.Run(context.Background(), cfg.HomeKit, []*accessory.A{monitor.Accessory}, monitor.Poll)
}
