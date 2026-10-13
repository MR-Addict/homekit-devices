package chromecast

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIPCRejectsMalformedOversizedAndInvalidAudio(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	cases := []string{
		`{"version":2,"operation":"key","key":"HOME"}`,
		`{"version":1,"operation":"key","key":"HOME","unexpected":true}`,
		`{"version":1,"operation":"key","key":"HOME","hold":31000000000}`,
		`{"version":1,"operation":"volume","key":"VOLUME_UP","steps":101}`,
		`{"version":1,"operation":"voice","audio":"UklGRg=="}`,
		`{"version":1,"operation":"text","text":"` + strings.Repeat("x", maxIPCRequest) + `"}`,
	}
	for _, input := range cases {
		conn, err := net.Dial("unix", d.cfg.Chromecast.KeyPath+".sock")
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		// Reading concurrently lets the server reject before the entire oversized
		// payload has finished writing, without deadlocking the test writer.
		written := make(chan struct{})
		go func() { defer close(written); conn.Write([]byte(input + "\n")) }()
		var response Response
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		<-written
		if response.ErrorCode != "invalid_request" {
			t.Fatal(response)
		}
	}
	if d.count.Load() != 1 {
		t.Fatal("bad request changed connection")
	}
	if _, err := s.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-d.frames:
		t.Fatal("invalid request sent a command")
	default:
	}
}
func TestIPCDisconnectCancelsActivePressWithoutClosingSession(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	conn, err := net.Dial("unix", d.cfg.Chromecast.KeyPath+".sock")
	if err != nil {
		t.Fatal(err)
	}
	json.NewEncoder(conn).Encode(Request{Version: 1, Operation: "key", Key: "DPAD_UP", Hold: 2 * time.Second})
	nextFrame(t, d)
	conn.Close()
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetDirection().String() != "END_LONG" {
		t.Fatal(m)
	}
	if err := s.SendKey(context.Background(), "BACK"); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	if d.count.Load() != 1 {
		t.Fatal("client disconnect closed connection")
	}
}
func TestIPCUnknownResultNeverFallsBack(t *testing.T) {
	d := newServiceDevice(t)
	path := d.cfg.Chromecast.KeyPath + ".sock"
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		bufio.NewReader(c).ReadBytes('\n')
	}()
	_, err = RunRequest(context.Background(), d.cfg, Request{Version: 1, Operation: "key", Key: "HOME"})
	<-done
	if err == nil || !strings.Contains(err.Error(), "will not be retried") {
		t.Fatal(err)
	}
	if d.count.Load() != 0 {
		t.Fatal("fell back to direct connection after accepted request")
	}
}
func TestMissingIPCWithOwnedLockFailsFastAndSocketPathIsPreserved(t *testing.T) {
	d := newServiceDevice(t)
	unlock, err := trySessionLock(d.cfg.Chromecast.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	_, err = RunRequest(context.Background(), d.cfg, Request{Version: 1, Operation: "key", Key: "HOME"})
	if !errors.Is(err, ErrBusy) || time.Since(began) > time.Second {
		t.Fatal(err)
	}
	unlock()
	path := d.cfg.Chromecast.KeyPath + ".sock"
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := StartSession(context.Background(), d.cfg); err == nil {
		s.Close()
		t.Fatal("removed non-socket")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatal("overwrote socket path")
	}
	unlock, err = trySessionLock(d.cfg.Chromecast.KeyPath)
	if err != nil {
		t.Fatal("failed startup leaked lock", err)
	}
	unlock()
}
func TestIPCVoiceTextVolumeAndApplicationUseSharedConnection(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	for _, q := range []Request{
		{Version: 1, Operation: "text", Text: "hello"},
		{Version: 1, Operation: "volume", Key: "VOLUME_DOWN", Steps: 2},
		{Version: 1, Operation: "input", App: "https://www.youtube.com"},
		{Version: 1, Operation: "voice", Audio: []byte{1, 0}},
	} {
		if _, err := RunRequest(context.Background(), d.cfg, q); err != nil {
			t.Fatal(q.Operation, err)
		}
	}
	if d.count.Load() != 1 {
		t.Fatal("CLI reconnected")
	}
	if _, err := s.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
}
