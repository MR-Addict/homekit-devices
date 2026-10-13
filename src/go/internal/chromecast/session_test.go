package chromecast

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brutella/hap/characteristic"
	pb "github.com/jkiddo/atvremote/pkg/v2/proto"
	"github.com/jkiddo/atvremote/pkg/v2/remote"
)

type serviceDevice struct {
	cfg              AppConfig
	listener         net.Listener
	count            atomic.Int32
	frames           chan *pb.RemoteMessage
	mu               sync.Mutex
	conns            map[net.Conn]bool
	wg               sync.WaitGroup
	acknowledgePower atomic.Bool
	stallHandshake   atomic.Bool
}

func newServiceDevice(t *testing.T) *serviceDevice {
	t.Helper()
	// macOS Unix socket names are limited to 104 bytes.
	dir, err := os.MkdirTemp("", "cast-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg := AppConfig{Chromecast: Config{Host: "127.0.0.1", CertPath: filepath.Join(dir, "cert.pem"), KeyPath: filepath.Join(dir, "key.pem"), PowerMode: "toggle"}, Control: ControlConfig{Timeout: "2s", TransitionTimeout: "2s", Interval: "50ms"}}
	r := (&Client{Config: cfg.Chromecast}).NewRemote()
	if _, err := r.GenerateCertIfMissing(); err != nil {
		t.Fatal(err)
	}
	cert, err := tls.LoadX509KeyPair(cfg.Chromecast.CertPath, cfg.Chromecast.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Chromecast.APIPort = listener.Addr().(*net.TCPAddr).Port
	d := &serviceDevice{cfg: cfg, listener: listener, frames: make(chan *pb.RemoteMessage, 256), conns: make(map[net.Conn]bool)}
	d.acknowledgePower.Store(true)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			d.count.Add(1)
			d.mu.Lock()
			d.conns[c] = true
			d.wg.Add(1)
			d.mu.Unlock()
			go d.serve(c)
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		d.mu.Lock()
		for c := range d.conns {
			c.Close()
		}
		d.mu.Unlock()
		d.wg.Wait()
	})
	return d
}
func (d *serviceDevice) serve(c net.Conn) {
	defer d.wg.Done()
	defer func() { c.Close(); d.mu.Lock(); delete(d.conns, c); d.mu.Unlock() }()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(c)
	if d.stallHandshake.Load() {
		io.Copy(io.Discard, reader)
		return
	}
	features := remote.FeaturePing | remote.FeatureKey | remote.FeaturePower | remote.FeatureVolume | remote.FeatureAppLink | remote.FeatureIME | remote.FeatureVoice
	if sendRemote(c, &pb.RemoteMessage{RemoteConfigure: &pb.RemoteConfigure{Code1: int32(features)}}) != nil {
		return
	}
	if _, err := readRemote(reader); err != nil {
		return
	}
	if sendRemote(c, &pb.RemoteMessage{RemoteSetActive: &pb.RemoteSetActive{}}) != nil {
		return
	}
	if _, err := readRemote(reader); err != nil {
		return
	}
	on, muted := true, false
	if sendRemote(c, &pb.RemoteMessage{RemoteStart: &pb.RemoteStart{Started: on}}) != nil {
		return
	}
	if sendRemote(c, &pb.RemoteMessage{RemoteSetVolumeLevel: &pb.RemoteSetVolumeLevel{VolumeMax: 100, VolumeLevel: 20}}) != nil {
		return
	}
	if sendRemote(c, &pb.RemoteMessage{RemoteImeBatchEdit: &pb.RemoteImeBatchEdit{}}) != nil {
		return
	}
	for {
		m, err := readRemote(reader)
		if err != nil {
			return
		}
		d.frames <- m
		key := m.GetRemoteKeyInject().GetKeyCode()
		switch key {
		case pb.RemoteKeyCode_KEYCODE_POWER, pb.RemoteKeyCode_KEYCODE_SLEEP, pb.RemoteKeyCode_KEYCODE_WAKEUP:
			if d.acknowledgePower.Load() {
				on = !on
				sendRemote(c, &pb.RemoteMessage{RemoteStart: &pb.RemoteStart{Started: on}})
			}
		case pb.RemoteKeyCode_KEYCODE_VOLUME_MUTE:
			muted = !muted
			sendRemote(c, &pb.RemoteMessage{RemoteSetVolumeLevel: &pb.RemoteSetVolumeLevel{VolumeMax: 100, VolumeLevel: 20, VolumeMuted: muted}})
		case pb.RemoteKeyCode_KEYCODE_SEARCH:
			sendRemote(c, &pb.RemoteMessage{RemoteVoiceBegin: &pb.RemoteVoiceBegin{SessionId: 123}})
		}
	}
}
func (d *serviceDevice) disconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for c := range d.conns {
		c.Close()
	}
}
func startTestSession(t *testing.T, d *serviceDevice) *Session {
	t.Helper()
	s, err := StartSession(context.Background(), d.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	await(t, func() bool { status, err := s.Status(context.Background()); return err == nil && status.Available })
	return s
}
func await(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}
func nextFrame(t *testing.T, d *serviceDevice) *pb.RemoteMessage {
	t.Helper()
	select {
	case m := <-d.frames:
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("missing frame")
		return nil
	}
}

func TestPersistentSessionHomeKitCLIAndStatusShareOneConnection(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	c := NewControllerWithBackend(d.cfg, s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Poll(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	latencies := make([]time.Duration, 0, 20)
	for n := 0; n < 20; n++ {
		began := time.Now()
		if n%2 == 0 {
			if _, code := c.RemoteKey.RemoteWriteFunc(characteristic.RemoteKeyArrowUp, nil); code != 0 {
				t.Fatal(code)
			}
		} else {
			if _, err := RunRequest(ctx, d.cfg, Request{Version: 1, Operation: "key", Key: "DPAD_DOWN"}); err != nil {
				t.Fatal(err)
			}
		}
		latencies = append(latencies, time.Since(began))
		if _, err := s.Status(ctx); err != nil {
			t.Fatal(err)
		}
		expected := pb.RemoteKeyCode_KEYCODE_DPAD_DOWN
		if n%2 == 0 {
			expected = pb.RemoteKeyCode_KEYCODE_DPAD_UP
		}
		if m := nextFrame(t, d); m.GetRemoteKeyInject().GetKeyCode() != expected {
			t.Fatal("key order changed", m)
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("20 warm commands: p95=%v, connections=%d", latencies[18], d.count.Load())
	if latencies[18] > 500*time.Millisecond {
		t.Fatal("fixed delay or excessive warm command latency")
	}
	if d.count.Load() != 1 {
		t.Fatal("commands reconnected")
	}
	info, err := os.Stat(d.cfg.Chromecast.KeyPath + ".sock")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
}
func TestSessionFIFOQueueLimitAndCanceledLongPress(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	active, err := s.enqueue(ctx, Request{Version: 1, Operation: "key", Key: "DPAD_UP", Hold: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetDirection() != pb.RemoteDirection_START_LONG {
		t.Fatal(m)
	}
	queuedCtx, queuedCancel := context.WithCancel(context.Background())
	results := make([]*result, 64)
	for n := range results {
		results[n], err = s.enqueue(queuedCtx, Request{Version: 1, Operation: "key", Key: "HOME"})
		if err != nil {
			t.Fatal(n, err)
		}
	}
	if _, err = s.enqueue(context.Background(), Request{Version: 1, Operation: "key", Key: "BACK"}); !errors.Is(err, ErrQueueFull) {
		t.Fatal(err)
	}
	queuedCancel()
	cancel()
	if err := active.wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetDirection() != pb.RemoteDirection_END_LONG {
		t.Fatal("missing release", m)
	}
	for _, r := range results {
		if err := r.wait(context.Background()); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if err := s.SendKey(context.Background(), "BACK"); err != nil {
		t.Fatal(err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_BACK {
		t.Fatal("canceled command sent", m)
	}
	if d.count.Load() != 1 {
		t.Fatal("cancellation closed healthy session")
	}
}
func TestSessionPowerDedupAndSnapshotDoesNotWaitForTransition(t *testing.T) {
	d := newServiceDevice(t)
	d.acknowledgePower.Store(false)
	s := startTestSession(t, d)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	first, err := s.enqueue(ctx, Request{Version: 1, Operation: "power", On: false})
	if err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	second, err := s.enqueue(ctx, Request{Version: 1, Operation: "power", On: false})
	if err != nil || first != second {
		t.Fatal("not merged", err)
	}
	if _, err = s.enqueue(ctx, Request{Version: 1, Operation: "power", On: true}); !errors.Is(err, ErrOppositePower) {
		t.Fatal(err)
	}
	began := time.Now()
	status, err := s.Status(ctx)
	if err != nil || status.On == nil || !*status.On || time.Since(began) > 50*time.Millisecond {
		t.Fatal("snapshot blocked or optimistic", status, err)
	}
	if err := first.wait(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-d.frames:
		t.Fatal("power write retried")
	default:
	}
}
func TestSessionReconnectGenerationAndNoReplay(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	s.mu.Lock()
	old := s.current
	s.mu.Unlock()
	if err := s.SendKey(context.Background(), "HOME"); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	d.disconnect()
	await(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.current != nil && s.current != old })
	s.notify(old)
	old.remote.Disconnect()
	if err := s.SendKey(context.Background(), "BACK"); err != nil {
		t.Fatal(err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_BACK {
		t.Fatal("replayed command", m)
	}
	if d.count.Load() != 2 {
		t.Fatal(d.count.Load())
	}
}
func TestHandshakeTimeoutRetriesButMissingCertStops(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		d := newServiceDevice(t)
		d.stallHandshake.Store(true)
		d.cfg.Control.Timeout = "30ms"
		s, err := StartSession(context.Background(), d.cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		await(t, func() bool { return d.count.Load() >= 2 })
		d.stallHandshake.Store(false)
		await(t, func() bool { _, err := s.Status(context.Background()); return err == nil })
	})
	t.Run("auth", func(t *testing.T) {
		d := newServiceDevice(t)
		os.Remove(d.cfg.Chromecast.CertPath)
		s, err := StartSession(context.Background(), d.cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		await(t, func() bool {
			_, err := s.Status(context.Background())
			var auth *remote.InvalidAuth
			return errors.As(err, &auth)
		})
		began := time.Now()
		if err := s.SendKey(context.Background(), "HOME"); err == nil || time.Since(began) > time.Second {
			t.Fatal(err)
		}
		if d.count.Load() != 0 {
			t.Fatal("authentication retried")
		}
	})
}
func TestSessionConfirmsMuteAndCloseReleasesIdentity(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	if err := s.SetMute(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	if err := s.SetMute(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := StartSession(context.Background(), d.cfg); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	client := &Client{Config: d.cfg.Chromecast}
	if err := client.Pair(context.Background(), nil, io.Discard); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Lstat(d.cfg.Chromecast.KeyPath + ".sock"); !os.IsNotExist(err) {
		t.Fatal("socket survived", err)
	}
	if err := s.SendKey(context.Background(), "HOME"); !errors.Is(err, ErrStopped) {
		t.Fatal(err)
	}
	if _, err := RunRequest(context.Background(), d.cfg, Request{Version: 1, Operation: "key", Key: "BACK"}); err != nil {
		t.Fatal("standalone failed", err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_BACK {
		t.Fatal(m)
	}
}

func TestQueuedDeadlineReturnsBeforeActiveCommandAndDoesNotSend(t *testing.T) {
	d := newServiceDevice(t)
	s := startTestSession(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	active, err := s.enqueue(ctx, Request{Version: 1, Operation: "key", Key: "DPAD_UP", Hold: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	expired, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	began := time.Now()
	if err := s.SendKey(expired, "HOME"); !errors.Is(err, context.DeadlineExceeded) || time.Since(began) > 300*time.Millisecond {
		t.Fatal(err)
	}
	cancel()
	active.wait(context.Background())
	nextFrame(t, d)
	if err := s.SendKey(context.Background(), "BACK"); err != nil {
		t.Fatal(err)
	}
	if m := nextFrame(t, d); m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_BACK {
		t.Fatal("expired queued key sent", m)
	}
}
func TestControllerPowerUsesSessionDedupAndStopsPrePollRequests(t *testing.T) {
	d := newServiceDevice(t)
	d.acknowledgePower.Store(false)
	s := startTestSession(t, d)
	c := NewControllerWithBackend(d.cfg, s)
	c.Sample(context.Background())
	first, err := s.enqueue(context.Background(), Request{Version: 1, Operation: "power", On: false})
	if err != nil {
		t.Fatal(err)
	}
	nextFrame(t, d)
	// Cached state says 'on'; this must still reject an opposite pending CLI command.
	if err := c.Request(true); !errors.Is(err, ErrOppositePower) {
		t.Fatal(err)
	}
	if err := c.Request(false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { c.Poll(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pre-Poll request survived controller shutdown")
	}
	// Canceling a merged observer must not cancel the original CLI command.
	select {
	case <-first.done:
		t.Fatal("duplicate observer canceled original power command")
	default:
	}
}
