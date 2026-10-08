package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIRejectsMalformedArgumentsBeforeConnecting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(p, []byte("chromecast:\n  host: localhost\n"), 0600)
	for _, args := range [][]string{{"nonsense"}, {"status", "extra"}, {"key", "-hold", "31s", "POWER"}, {"text"}, {"launch", "one", "two"}, {"volume", "sideways"}, {"volume", "up", "0"}, {"volume", "down", "101"}, {"volume", "up", "many"}, {"on", "extra"}, {"mute", "extra"}} {
		if e := run(append([]string{"-config", p}, args...)); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
