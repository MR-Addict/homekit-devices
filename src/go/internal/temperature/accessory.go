package temperature

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
)

type Monitor struct {
	Accessory *accessory.A
	Current   *characteristic.CurrentTemperature
	Fault     *characteristic.StatusFault
	path      string
	interval  time.Duration
}

func New(c Config) (*Monitor, error) {
	interval, err := c.Temperature.Duration()
	if err != nil {
		return nil, err
	}
	initial, err := Read(c.Temperature.Path)
	if err != nil {
		return nil, fmt.Errorf("initial temperature: %w", err)
	}
	serial := c.HomeKit.SerialNumber
	if serial == "" {
		serial = "ROUTER-TEMPERATURE-1"
	}
	a := accessory.New(accessory.Info{Name: c.Temperature.Name, SerialNumber: serial, Manufacturer: c.HomeKit.Manufacturer, Model: c.HomeKit.Model, Firmware: c.HomeKit.Firmware}, accessory.TypeSensor)
	a.Id = 1
	sensor := service.NewTemperatureSensor()
	fault := characteristic.NewStatusFault()
	sensor.AddC(fault.C)
	a.AddS(sensor.S)
	sensor.CurrentTemperature.SetValue(initial)
	return &Monitor{Accessory: a, Current: sensor.CurrentTemperature, Fault: fault, path: c.Temperature.Path, interval: interval}, nil
}

// Sample leaves the last valid reading unchanged on failure.
func (m *Monitor) Sample() error {
	value, err := Read(m.path)
	if err != nil {
		m.Fault.SetValue(characteristic.StatusFaultGeneralFault)
		return err
	}
	m.Current.SetValue(value)
	m.Fault.SetValue(characteristic.StatusFaultNoFault)
	return nil
}
func (m *Monitor) Poll(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Sample(); err != nil {
				log.Printf("temperature sensor fault: %v", err)
			}
		}
	}
}
