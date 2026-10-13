package chromecast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jkiddo/atvremote/pkg/v2/remote"
)

var (
	ErrUnavailable   = errors.New("Chromecast disconnected")
	ErrUnsupported   = errors.New("device does not support requested remote feature")
	ErrBusy          = errors.New("Chromecast credentials busy; use the running service or stop it before pairing")
	ErrQueueFull     = errors.New("Chromecast command queue full")
	ErrStopped       = errors.New("Chromecast service stopped")
	ErrOppositePower = errors.New("opposite power operation already in progress")
)

// Request is the version 1 local IPC command. Audio is base64 encoded by JSON.
// Hold is a duration in nanoseconds. Muted=nil requests the CLI's explicit toggle.
type Request struct {
	Version   int           `json:"version"`
	Operation string        `json:"operation"`
	Key       string        `json:"key,omitempty"`
	Hold      time.Duration `json:"hold,omitempty"`
	On        bool          `json:"on,omitempty"`
	Muted     *bool         `json:"muted,omitempty"`
	Text      string        `json:"text,omitempty"`
	App       string        `json:"app,omitempty"`
	Steps     int           `json:"steps,omitempty"`
	Audio     []byte        `json:"audio,omitempty"`
}

func (q Request) validate() error {
	if q.Version != 1 {
		return errors.New("unsupported IPC version")
	}
	switch q.Operation {
	case "status", "power", "mute":
	case "key":
		if _, err := remote.ParseKeyCode(q.Key); err != nil {
			return err
		}
		if q.Hold < 0 || q.Hold > 30*time.Second {
			return errors.New("hold must be 0..30s")
		}
	case "text":
		if q.Text == "" {
			return errors.New("text cannot be empty")
		}
	case "launch", "input":
		if len(bytes.TrimSpace([]byte(q.App))) == 0 {
			return errors.New("application ID or link is required")
		}
	case "volume":
		if q.Key != "VOLUME_UP" && q.Key != "VOLUME_DOWN" || q.Steps < 1 || q.Steps > 100 {
			return errors.New("volume requires up/down and 1..100 steps")
		}
	case "voice":
		if err := validateAudio(q.Audio); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown operation %q", q.Operation)
	}
	return nil
}

func validateAudio(audio []byte) error {
	if len(audio) == 0 || len(audio) > 960000 || len(audio)%2 != 0 {
		return errors.New("voice requires even-length raw PCM, between 1 sample and 60 seconds")
	}
	if bytes.HasPrefix(audio, []byte("RIFF")) {
		return errors.New("voice expects raw PCM, not a WAV file")
	}
	return nil
}

func (q Request) timeout(cfg AppConfig) time.Duration {
	base := Duration(cfg.Control.Timeout)
	switch q.Operation {
	case "power":
		return Duration(cfg.Control.TransitionTimeout)
	case "input":
		return Duration(cfg.Control.TransitionTimeout) + base
	case "key":
		return base + q.Hold
	case "volume":
		return base + time.Duration(q.Steps)*50*time.Millisecond
	case "voice":
		return 70 * time.Second
	default:
		return base
	}
}

// perform is shared by the long-lived service and standalone CLI. It never
// retries a write; a lost result does not imply the device did not act.
func perform(ctx context.Context, r *remote.AndroidTVRemote, cfg AppConfig, q Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch q.Operation {
	case "key":
		if q.Key == "VOLUME_UP" || q.Key == "VOLUME_DOWN" {
			if err := Require(r, remote.FeatureVolume); err != nil {
				return err
			}
		}
		return Key(ctx, r, q.Key, q.Hold)
	case "power":
		return setPower(ctx, r, cfg.Chromecast.PowerMode, q.On)
	case "text":
		return Text(ctx, r, q.Text)
	case "launch":
		return Launch(r, q.App)
	case "input":
		power, cancel := context.WithTimeout(ctx, Duration(cfg.Control.TransitionTimeout))
		err := setPower(power, r, cfg.Chromecast.PowerMode, true)
		cancel()
		if err != nil {
			return err
		}
		launch, cancel := context.WithTimeout(ctx, Duration(cfg.Control.Timeout))
		defer cancel()
		// Launch has no release frame; close this generation immediately if
		// its separate write budget expires after the power transition.
		stop := context.AfterFunc(launch, r.Disconnect)
		defer stop()
		err = Launch(r, q.App)
		if launch.Err() != nil {
			return launch.Err()
		}
		return err
	case "mute":
		if q.Muted != nil {
			return setMute(ctx, r, *q.Muted)
		}
		if err := Require(r, remote.FeatureVolume); err != nil {
			return err
		}
		return Key(ctx, r, "VOLUME_MUTE", 0)
	case "volume":
		if err := Require(r, remote.FeatureVolume); err != nil {
			return err
		}
		for i := 0; i < q.Steps; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := Key(ctx, r, q.Key, 0); err != nil {
				return err
			}
			if i+1 < q.Steps {
				if err := Sleep(ctx, 50*time.Millisecond); err != nil {
					return err
				}
			}
		}
		return nil
	case "voice":
		return Voice(ctx, r, bytes.NewReader(q.Audio))
	}
	return errors.New("operation does not send commands")
}

type generation struct{ remote *remote.AndroidTVRemote }
type result struct {
	done chan struct{}
	err  error
}

func (r *result) wait(ctx context.Context) error {
	select {
	case <-r.done:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type command struct {
	request Request
	ctx     context.Context
	cancel  context.CancelFunc
	stop    func() bool
	result  *result
	entered time.Time
}

// Session owns exactly one credential identity and FIFO command executor.
// Status reads and push notifications never enter the command queue.
type Session struct {
	cfg       AppConfig
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	current   *generation
	changed   chan struct{}
	updates   chan struct{}
	terminal  error
	queue     chan *command
	power     *command
	wg        sync.WaitGroup
	unlock    func()
	closeOnce sync.Once
	ipc       *ipcServer
}

func StartSession(parent context.Context, cfg AppConfig) (*Session, error) {
	if _, err := os.Stat(cfg.Chromecast.KeyPath); err != nil {
		return nil, fmt.Errorf("Chromecast credentials missing; run homekit-chromecast pair: %w", err)
	}
	unlock, err := trySessionLock(cfg.Chromecast.KeyPath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Session{cfg: cfg, ctx: ctx, cancel: cancel, changed: make(chan struct{}), updates: make(chan struct{}, 1), queue: make(chan *command, 64), unlock: unlock}
	s.ipc, err = startIPC(s)
	if err != nil {
		cancel()
		unlock()
		return nil, err
	}
	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.connections() }()
	go func() { defer s.wg.Done(); s.execute() }()
	return s, nil
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.ipc.close()
		s.mu.Lock()
		g := s.current
		s.mu.Unlock()
		if g != nil {
			g.remote.Disconnect()
		}
		s.wg.Wait()
		s.unlock()
	})
}
func (s *Session) Updates() <-chan struct{} { return s.updates }

// signalLocked wakes state waiters and the controller without blocking readers.
func (s *Session) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
	select {
	case s.updates <- struct{}{}:
	default:
	}
}
func (s *Session) notify(g *generation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == g {
		s.signalLocked()
	}
}
func (s *Session) connections() {
	delay := 100 * time.Millisecond
	for s.ctx.Err() == nil {
		g := &generation{remote: (&Client{Config: s.cfg.Chromecast}).NewRemote()}
		removers := []func(){
			g.remote.AddIsOnUpdatedCallback(func(bool) { s.notify(g) }),
			g.remote.AddCurrentAppUpdatedCallback(func(string) { s.notify(g) }),
			g.remote.AddVolumeInfoUpdatedCallback(func(remote.VolumeInfo) { s.notify(g) }),
		}
		started := time.Now()
		ctx, cancel := context.WithTimeout(s.ctx, Duration(s.cfg.Control.Timeout))
		err := g.remote.Connect(ctx)
		cancel()
		slog.Debug("Chromecast connect", "duration", time.Since(started), "error", err)
		if err == nil {
			s.mu.Lock()
			s.current = g
			s.terminal = nil
			s.signalLocked()
			s.mu.Unlock()
			delay = 100 * time.Millisecond
			select {
			case <-s.ctx.Done():
			case <-g.remote.ConnectionDone():
			}
			s.mu.Lock()
			if s.current == g {
				s.current = nil
				s.signalLocked()
			}
			s.mu.Unlock()
		}
		g.remote.Disconnect()
		for _, remove := range removers {
			remove()
		}
		if err != nil {
			s.mu.Lock()
			s.terminal = err
			s.signalLocked()
			s.mu.Unlock()
			var auth *remote.InvalidAuth
			if errors.As(err, &auth) {
				slog.Error("Chromecast authentication failed; stop the service and run pair", "error", err)
				return
			}
		}
		if Sleep(s.ctx, delay) != nil {
			return
		}
		delay = min(delay*2, 30*time.Second)
	}
}
func (s *Session) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{Error: err.Error()}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return Status{Error: ErrStopped.Error()}, ErrStopped
	}
	if s.current != nil {
		status := Snapshot(s.current.remote)
		if status.Available {
			return status, nil
		}
	}
	err := s.terminal
	if err == nil {
		err = ErrUnavailable
	}
	return Status{Error: err.Error()}, err
}
func (s *Session) connection(ctx context.Context) (*generation, error) {
	for {
		if s.ctx.Err() != nil {
			return nil, ErrStopped
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.mu.Lock()
		g, changed, terminal := s.current, s.changed, s.terminal
		s.mu.Unlock()
		if g != nil {
			if _, ok := g.remote.Features(); ok {
				return g, nil
			}
		}
		var auth *remote.InvalidAuth
		if errors.As(terminal, &auth) {
			return nil, terminal
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.ctx.Done():
			return nil, ErrStopped
		case <-changed:
		}
	}
}
func (s *Session) enqueue(parent context.Context, q Request) (*result, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, q.timeout(s.cfg))
	stop := context.AfterFunc(s.ctx, cancel)
	j := &command{request: q, ctx: ctx, cancel: cancel, stop: stop, result: &result{done: make(chan struct{})}, entered: time.Now()}
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(err error) (*result, error) { stop(); cancel(); return nil, err }
	if s.ctx.Err() != nil {
		return fail(ErrStopped)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if q.Operation == "power" && s.power != nil {
		if s.power.request.On != q.On {
			return fail(ErrOppositePower)
		}
		stop()
		cancel()
		return s.power.result, nil
	}
	select {
	case s.queue <- j:
		if q.Operation == "power" {
			s.power = j
		}
		return j.result, nil
	default:
		return fail(ErrQueueFull)
	}
}
func (s *Session) finish(j *command, err error) {
	s.mu.Lock()
	if s.power == j {
		s.power = nil
	}
	j.result.err = err
	close(j.result.done)
	s.mu.Unlock()
	j.stop()
	j.cancel()
}
func (s *Session) execute() {
	defer func() {
		for {
			select {
			case j := <-s.queue:
				s.finish(j, ErrStopped)
			default:
				return
			}
		}
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case j := <-s.queue:
			dequeued := time.Now()
			g, err := s.connection(j.ctx)
			ready := time.Now()
			if err == nil {
				finished := make(chan struct{})
				stop := context.AfterFunc(j.ctx, func() {
					timer := time.NewTimer(250 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-finished:
						return
					case <-timer.C:
						g.remote.Disconnect()
					}
				})
				err = perform(j.ctx, g.remote, s.cfg, j.request)
				close(finished)
				stop()
				if j.ctx.Err() != nil {
					err = j.ctx.Err()
				}
				// An unconfirmed stateful toggle makes the cached observation unsafe
				// for a later toggle. Reconnect to obtain a fresh handshake state;
				// the failed command itself is never replayed.
				stateful := j.request.Operation == "power" || j.request.Operation == "input" || j.request.Operation == "mute" && j.request.Muted != nil
				if stateful && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
					g.remote.Disconnect()
				}
			}
			slog.Debug("Chromecast command", "operation", j.request.Operation, "queue_wait", dequeued.Sub(j.entered), "connection_wait", ready.Sub(dequeued), "execution", time.Since(ready), "total", time.Since(j.entered), "error", err)
			s.finish(j, err)
		}
	}
}
func (s *Session) Do(parent context.Context, q Request) (*Status, error) {
	ctx, cancel := context.WithTimeout(parent, q.timeout(s.cfg))
	defer cancel()
	if err := q.validate(); err != nil {
		return nil, err
	}
	if q.Operation == "status" {
		status, err := s.Status(ctx)
		return &status, err
	}
	result, err := s.enqueue(ctx, q)
	if err != nil {
		return nil, err
	}
	return nil, result.wait(ctx)
}
func (s *Session) SetPower(ctx context.Context, on bool) error {
	_, err := s.Do(ctx, Request{Version: 1, Operation: "power", On: on})
	return err
}
func (s *Session) SendKey(ctx context.Context, key string) error {
	_, err := s.Do(ctx, Request{Version: 1, Operation: "key", Key: key})
	return err
}
func (s *Session) LaunchApp(ctx context.Context, app string) error {
	_, err := s.Do(ctx, Request{Version: 1, Operation: "launch", App: app})
	return err
}
func (s *Session) SetMute(ctx context.Context, muted bool) error {
	_, err := s.Do(ctx, Request{Version: 1, Operation: "mute", Muted: &muted})
	return err
}
