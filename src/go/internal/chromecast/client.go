// Package chromecast wraps the Android TV Remote v2 protocol, not Cast media.
package chromecast

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "github.com/jkiddo/atvremote/pkg/v2/proto"
	"github.com/jkiddo/atvremote/pkg/v2/remote"
)

type Config struct {
	Host      string `yaml:"host"`
	APIPort   int    `yaml:"api_port,omitempty"`
	CertPath  string `yaml:"cert_path"`
	KeyPath   string `yaml:"key_path"`
	PowerMode string `yaml:"power_mode"`
}
type Client struct {
	Config  Config
	Timeout time.Duration
}
type Status struct {
	Available bool               `json:"available"`
	On        *bool              `json:"on"`
	App       *string            `json:"app"`
	Device    *remote.DeviceInfo `json:"device,omitempty"`
	Volume    *remote.VolumeInfo `json:"volume,omitempty"`
	Features  remote.Feature     `json:"features"`
	Error     string             `json:"error,omitempty"`
}

func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (c *Client) NewRemote() *remote.AndroidTVRemote {
	port := c.Config.APIPort
	if port == 0 {
		port = 6466
	}
	return remote.New("HomeKit Chromecast Controller", c.Config.CertPath, c.Config.KeyPath, c.Config.Host, remote.WithAPIPort(port), remote.WithIME(true), remote.WithVoice(true), remote.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}
func (c *Client) WithSession(ctx context.Context, fn func(*remote.AndroidTVRemote) error) error {
	if _, bounded := ctx.Deadline(); !bounded {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.TimeoutOrDefault()+time.Second)
		defer cancel()
	}
	if _, err := os.Stat(c.Config.KeyPath); err != nil {
		return fmt.Errorf("Chromecast credentials missing; run homekit-chromecast pair: %w", err)
	}
	unlock, err := sessionLock(ctx, c.Config.KeyPath)
	if err != nil {
		return err
	}
	defer unlock()
	r := c.NewRemote()
	defer r.Disconnect()
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	connect, cancel := context.WithTimeout(ctx, timeout)
	err = r.Connect(connect)
	cancel()
	if err != nil {
		return err
	}
	// Connect's context covers its handshake only. Bound all subsequent I/O too.
	stop := context.AfterFunc(ctx, func() {
		// Give canceled key/voice operations a bounded opportunity to send their
		// release/end frame before closing a possibly blocked socket.
		time.Sleep(250 * time.Millisecond)
		r.Disconnect()
	})
	defer stop()
	if err = Sleep(ctx, time.Second); err != nil {
		return err
	}
	err = fn(r)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
func Snapshot(r *remote.AndroidTVRemote) Status {
	var s Status
	s.Features, s.Available = r.Features()
	if on, ok := r.IsOn(); ok {
		s.On = &on
	}
	if app, ok := r.CurrentApp(); ok && app != "" {
		s.App = &app
	}
	s.Device = r.DeviceInfo()
	s.Volume = r.VolumeInfo()
	if s.Volume != nil && s.Volume.Max == 0 {
		// Remote service sends partial volume messages without a usable range.
		// Expose these as unknown instead of inventing a 0/0 volume setting.
		s.Volume = nil
	}
	return s
}
func (c *Client) Status(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, c.TimeoutOrDefault())
	defer cancel()
	var s Status
	err := c.WithSession(ctx, func(r *remote.AndroidTVRemote) error {
		s = Snapshot(r)
		if s.On == nil {
			return errors.New("Chromecast power state unknown")
		}
		return nil
	})
	if err != nil {
		s.Available = false
		s.Error = err.Error()
	}
	return s, err
}
func (c *Client) TimeoutOrDefault() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 5 * time.Second
}
func Require(r *remote.AndroidTVRemote, f remote.Feature) error {
	features, known := r.Features()
	if !known {
		return errors.New("Chromecast disconnected")
	}
	if features&f != f {
		return fmt.Errorf("device does not support requested remote feature (%d)", f)
	}
	return nil
}

// SetPower never sends a toggle without a fresh, known state. A lost write is
// never retried: the device may already have acted on it.
func (c *Client) SetPower(ctx context.Context, on bool) error {
	if _, bounded := ctx.Deadline(); !bounded {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	return c.WithSession(ctx, func(r *remote.AndroidTVRemote) error {
		if err := Require(r, remote.FeaturePower|remote.FeatureKey); err != nil {
			return err
		}
		current, ok := r.IsOn()
		if !ok {
			return errors.New("Chromecast power unknown; refusing POWER toggle")
		}
		if current == on {
			return nil
		}
		key := "POWER"
		if c.Config.PowerMode == "discrete" {
			key = "SLEEP"
			if on {
				key = "WAKEUP"
			}
		}
		if err := r.SendKeyCommand(key); err != nil {
			return err
		}
		for {
			if err := Sleep(ctx, 100*time.Millisecond); err != nil {
				return err
			}
			value, known := r.IsOn()
			if !known {
				return errors.New("Chromecast disconnected during power transition")
			}
			if value == on {
				return nil
			}
		}
	})
}
func (c *Client) Pair(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(c.Config.KeyPath), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Config.CertPath), 0700); err != nil {
		return err
	}
	unlock, err := sessionLock(ctx, c.Config.KeyPath)
	if err != nil {
		return err
	}
	defer unlock()
	r := c.NewRemote()
	defer r.Disconnect()
	stop := context.AfterFunc(ctx, r.Disconnect)
	defer stop()
	if _, err := r.GenerateCertIfMissing(); err != nil {
		return err
	}
	if err := os.Chmod(c.Config.KeyPath, 0600); err != nil {
		return err
	}
	if err := r.StartPairing(ctx); err != nil {
		return err
	}
	fmt.Fprintln(output, "Enter the six-character pairing code shown on Chromecast:")
	// Interrupt a blocked terminal read when pairing times out or is canceled.
	if closer, ok := input.(io.Closer); ok {
		stopInput := context.AfterFunc(ctx, func() { closer.Close() })
		defer stopInput()
	}
	code, err := bufio.NewReader(input).ReadString('\n')
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return r.FinishPairing(strings.TrimSpace(code))
}
func Keys() []string {
	names := make([]string, 0, len(pb.RemoteKeyCode_value))
	for name := range pb.RemoteKeyCode_value {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func Key(ctx context.Context, r *remote.AndroidTVRemote, name string, hold time.Duration) error {
	if err := Require(r, remote.FeatureKey); err != nil {
		return err
	}
	if _, err := remote.ParseKeyCode(name); err != nil {
		return err
	}
	if hold < 0 || hold > 30*time.Second {
		return errors.New("hold must be between 0 and 30s")
	}
	if hold == 0 {
		return r.SendKeyCommand(name)
	}
	if err := r.SendKeyCommandDirection(name, "START_LONG"); err != nil {
		return err
	}
	err := Sleep(ctx, hold)
	// On cancellation the session may already have closed. Always attempt release.
	return errors.Join(err, r.SendKeyCommandDirection(name, "END_LONG"))
}
func Text(ctx context.Context, r *remote.AndroidTVRemote, text string) error {
	if err := Require(r, remote.FeatureIME); err != nil {
		return err
	}
	if text == "" {
		return errors.New("text cannot be empty")
	}
	for !r.IMEReady() {
		if err := Sleep(ctx, 100*time.Millisecond); err != nil {
			return fmt.Errorf("input field not ready: %w", err)
		}
	}
	return r.SendText(text)
}
func Launch(r *remote.AndroidTVRemote, app string) error {
	if err := Require(r, remote.FeatureAppLink); err != nil {
		return err
	}
	if strings.TrimSpace(app) == "" {
		return errors.New("application ID or link is required")
	}
	return r.SendLaunchAppCommand(app)
}

// Voice reads raw signed 16-bit little-endian mono PCM at 8 kHz, paced at real
// time. Input is bounded to 60 seconds and must not contain a WAV header.
func Voice(ctx context.Context, r *remote.AndroidTVRemote, input io.Reader) error {
	if err := Require(r, remote.FeatureVoice|remote.FeatureKey); err != nil {
		return err
	}
	audio, err := io.ReadAll(io.LimitReader(input, 960001))
	if err != nil {
		return err
	}
	if len(audio) == 0 || len(audio) > 960000 || len(audio)%2 != 0 {
		return errors.New("voice requires even-length raw PCM, between 1 sample and 60 seconds")
	}
	if len(audio) >= 4 && string(audio[:4]) == "RIFF" {
		return errors.New("voice expects raw PCM, not a WAV file")
	}
	stream, err := r.StartVoice(ctx, 2*time.Second)
	if err != nil {
		return err
	}
	var result error
	for len(audio) > 0 {
		n := 8192
		if n > len(audio) {
			n = len(audio)
		}
		if _, err = stream.SendChunk(audio[:n]); err != nil {
			result = err
			break
		}
		audio = audio[n:]
		// The library pads short chunks to 8 KB, so pace the padded duration.
		pacedBytes := n
		if pacedBytes < 8192 {
			pacedBytes = 8192
		}
		if err = Sleep(ctx, time.Duration(pacedBytes)*time.Second/16000); err != nil {
			result = err
			break
		}
	}
	return errors.Join(result, stream.End())
}
func WriteJSON(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}

// SendKey shares the same bounded session and credential lock as CLI commands.
func (c *Client) SendKey(ctx context.Context, key string) error {
	return c.WithSession(ctx, func(r *remote.AndroidTVRemote) error {
		if key == "VOLUME_UP" || key == "VOLUME_DOWN" {
			if err := Require(r, remote.FeatureVolume); err != nil {
				return err
			}
		}
		return Key(ctx, r, key, 0)
	})
}
func (c *Client) LaunchApp(ctx context.Context, app string) error {
	return c.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Launch(r, app) })
}
func (c *Client) SetMute(ctx context.Context, muted bool) error {
	return c.WithSession(ctx, func(r *remote.AndroidTVRemote) error {
		if err := Require(r, remote.FeatureVolume|remote.FeatureKey); err != nil {
			return err
		}
		volume := Snapshot(r).Volume
		if volume == nil {
			return errors.New("Chromecast mute state unknown; refusing toggle")
		}
		if volume.Muted == muted {
			return nil
		}
		return Key(ctx, r, "VOLUME_MUTE", 0)
	})
}
