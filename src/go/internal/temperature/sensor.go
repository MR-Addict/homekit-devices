package temperature

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Read converts the Linux thermal ABI's millidegrees to HomeKit Celsius.
// HomeKit CurrentTemperature supports 0–100°C; reject out-of-range readings.
func Read(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read temperature %s: %w", path, err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || n < 0 || n > 100000 {
		return 0, fmt.Errorf("invalid temperature in %s: expected integer millidegrees in 0..100000", path)
	}
	return float64(n) / 1000, nil
}

type Sensor struct {
	Type    string
	Path    string
	Celsius float64
	Err     error
}

// Discover also returns unreadable zones so users can diagnose missing sensors.
func Discover(root string) ([]Sensor, error) {
	zones, err := filepath.Glob(filepath.Join(root, "thermal_zone*"))
	if err != nil {
		return nil, err
	}
	var sensors []Sensor
	for _, zone := range zones {
		s := Sensor{Path: filepath.Join(zone, "temp")}
		kind, err := os.ReadFile(filepath.Join(zone, "type"))
		if err != nil {
			s.Type = filepath.Base(zone)
		} else {
			s.Type = strings.TrimSpace(string(kind))
		}
		s.Celsius, s.Err = Read(s.Path)
		sensors = append(sensors, s)
	}
	if len(sensors) == 0 {
		return nil, fmt.Errorf("no thermal sensors found in %s", root)
	}
	return sensors, nil
}
