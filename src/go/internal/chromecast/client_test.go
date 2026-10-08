package chromecast

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/jkiddo/atvremote/pkg/v2/proto"
	"github.com/jkiddo/atvremote/pkg/v2/remote"
	"google.golang.org/protobuf/proto"
)

func sendRemote(w io.Writer, m *pb.RemoteMessage) error {
	b, e := proto.Marshal(m)
	if e != nil {
		return e
	}
	_, e = w.Write(append(binary.AppendUvarint(nil, uint64(len(b))), b...))
	return e
}
func readRemote(r *bufio.Reader) (*pb.RemoteMessage, error) {
	n, e := binary.ReadUvarint(r)
	if e != nil {
		return nil, e
	}
	if n > 1<<20 {
		return nil, errors.New("oversized frame")
	}
	b := make([]byte, n)
	if _, e = io.ReadFull(r, b); e != nil {
		return nil, e
	}
	m := new(pb.RemoteMessage)
	return m, proto.Unmarshal(b, m)
}

// Tests speak actual TLS/protobuf to the pinned library, including its patched
// feature/IME surfaces. No physical Chromecast is involved.
func fakeDevice(t *testing.T, features remote.Feature, on bool, ready bool, handle func(net.Conn, *bufio.Reader) error) (*Client, <-chan error) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{Host: "127.0.0.1", CertPath: filepath.Join(dir, "cert.pem"), KeyPath: filepath.Join(dir, "key.pem"), PowerMode: "toggle"}
	client := &Client{Config: cfg, Timeout: 3 * time.Second}
	r := client.NewRemote()
	if _, e := r.GenerateCertIfMissing(); e != nil {
		t.Fatal(e)
	}
	cert, e := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS12})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { listener.Close() })
	client.Config.APIPort = listener.Addr().(*net.TCPAddr).Port
	done := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		if e = sendRemote(conn, &pb.RemoteMessage{RemoteConfigure: &pb.RemoteConfigure{Code1: int32(features)}}); e != nil {
			done <- e
			return
		}
		if _, e = readRemote(reader); e != nil {
			done <- e
			return
		}
		if e = sendRemote(conn, &pb.RemoteMessage{RemoteSetActive: &pb.RemoteSetActive{}}); e != nil {
			done <- e
			return
		}
		if _, e = readRemote(reader); e != nil {
			done <- e
			return
		}
		if e = sendRemote(conn, &pb.RemoteMessage{RemoteStart: &pb.RemoteStart{Started: on}}); e != nil {
			done <- e
			return
		}
		if ready {
			if e = sendRemote(conn, &pb.RemoteMessage{RemoteImeBatchEdit: &pb.RemoteImeBatchEdit{}}); e != nil {
				done <- e
				return
			}
		}
		done <- handle(conn, reader)
	}()
	return client, done
}
func eofOnly(_ net.Conn, r *bufio.Reader) error {
	_, e := readRemote(r)
	if errors.Is(e, io.EOF) {
		return nil
	}
	return errors.New("unexpected command or missing close")
}
func TestPowerUsesFreshStateAndNoDuplicateToggle(t *testing.T) {
	client, done := fakeDevice(t, remote.FeatureKey|remote.FeaturePower, true, false, eofOnly)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := client.SetPower(ctx, true); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	client, done = fakeDevice(t, remote.FeatureKey|remote.FeaturePower, false, false, func(c net.Conn, r *bufio.Reader) error {
		m, e := readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_POWER {
			return errors.New("expected POWER")
		}
		if e = sendRemote(c, &pb.RemoteMessage{RemoteStart: &pb.RemoteStart{Started: true}}); e != nil {
			return e
		}
		return eofOnly(c, r)
	})
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := client.SetPower(ctx, true); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestUnsupportedDoesNotSendKey(t *testing.T) {
	client, done := fakeDevice(t, remote.FeaturePower, true, false, eofOnly)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	e := client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Key(ctx, r, "HOME", 0) })
	if e == nil || !strings.Contains(e.Error(), "does not support") {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
func TestTextWaitsForIMEAndAcceptsZeroCounters(t *testing.T) {
	client, done := fakeDevice(t, remote.FeatureIME|remote.FeatureKey, true, true, func(c net.Conn, r *bufio.Reader) error {
		m, e := readRemote(r)
		if e != nil {
			return e
		}
		edits := m.GetRemoteImeBatchEdit().GetEditInfo()
		if len(edits) != 1 || edits[0].GetTextFieldStatus().GetValue() != "hello" {
			return errors.New("wrong text payload")
		}
		return eofOnly(c, r)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Text(ctx, r, "hello") }); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	client, done = fakeDevice(t, remote.FeatureIME|remote.FeatureKey, true, false, eofOnly)
	ctx, cancel = context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if e := client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Text(ctx, r, "hello") }); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestCanceledLongPressReleasesBeforeDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	client, done := fakeDevice(t, remote.FeatureKey, true, false, func(c net.Conn, r *bufio.Reader) error {
		m, e := readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteKeyInject().GetDirection() != pb.RemoteDirection_START_LONG {
			return errors.New("missing long press start")
		}
		cancel()
		m, e = readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteKeyInject().GetDirection() != pb.RemoteDirection_END_LONG {
			return errors.New("missing long press release")
		}
		return eofOnly(c, r)
	})
	if e := client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Key(ctx, r, "DPAD_DOWN", 3*time.Second) }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestDisconnectedStateNeverToggles(t *testing.T) {
	client, done := fakeDevice(t, remote.FeatureKey|remote.FeaturePower, true, false, func(net.Conn, *bufio.Reader) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := client.SetPower(ctx, false); e == nil {
		t.Fatal("disconnected power treated as known")
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func TestVoiceEndsSessionAndPadsShortAudio(t *testing.T) {
	client, done := fakeDevice(t, remote.FeatureVoice|remote.FeatureKey, true, false, func(c net.Conn, r *bufio.Reader) error {
		m, e := readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_SEARCH {
			return errors.New("missing voice SEARCH")
		}
		if e = sendRemote(c, &pb.RemoteMessage{RemoteVoiceBegin: &pb.RemoteVoiceBegin{SessionId: 123}}); e != nil {
			return e
		}
		m, e = readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteVoiceBegin().GetSessionId() != 123 {
			return errors.New("missing voice begin acknowledgement")
		}
		m, e = readRemote(r)
		if e != nil {
			return e
		}
		p := m.GetRemoteVoicePayload()
		if p.GetSessionId() != 123 || len(p.GetSamples()) != 8192 {
			return errors.New("voice payload not padded or wrong session")
		}
		m, e = readRemote(r)
		if e != nil {
			return e
		}
		if m.GetRemoteVoiceEnd().GetSessionId() != 123 {
			return errors.New("voice session not ended")
		}
		return eofOnly(c, r)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if e := client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return Voice(ctx, r, strings.NewReader("\x01\x00")) }); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func TestHomeKitMuteUsesFreshVolumeAndNeverBlindlyToggles(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		known, muted, target, send, wantErr bool
	}{
		{name: "unknown", target: true, wantErr: true},
		{name: "already-muted", known: true, muted: true, target: true},
		{name: "mute", known: true, target: true, send: true},
		{name: "unmute", known: true, muted: true, send: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, done := fakeDevice(t, remote.FeatureKey|remote.FeatureVolume, true, false, func(c net.Conn, r *bufio.Reader) error {
				if tc.known {
					if err := sendRemote(c, &pb.RemoteMessage{RemoteSetVolumeLevel: &pb.RemoteSetVolumeLevel{VolumeMax: 100, VolumeLevel: 20, VolumeMuted: tc.muted}}); err != nil {
						return err
					}
				}
				if tc.send {
					m, err := readRemote(r)
					if err != nil {
						return err
					}
					if m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_VOLUME_MUTE {
						return errors.New("missing mute key")
					}
				}
				return eofOnly(c, r)
			})
			err := client.SetMute(context.Background(), tc.target)
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestHomeKitClientVolumeAndLaunch(t *testing.T) {
	t.Run("volume", func(t *testing.T) {
		t.Parallel()
		client, done := fakeDevice(t, remote.FeatureKey|remote.FeatureVolume, true, false, func(c net.Conn, r *bufio.Reader) error {
			m, err := readRemote(r)
			if err != nil {
				return err
			}
			if m.GetRemoteKeyInject().GetKeyCode() != pb.RemoteKeyCode_KEYCODE_VOLUME_DOWN {
				return errors.New("wrong volume key")
			}
			return eofOnly(c, r)
		})
		if err := client.SendKey(context.Background(), "VOLUME_DOWN"); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("launch", func(t *testing.T) {
		t.Parallel()
		client, done := fakeDevice(t, remote.FeatureAppLink, true, false, func(c net.Conn, r *bufio.Reader) error {
			m, err := readRemote(r)
			if err != nil {
				return err
			}
			if m.GetRemoteAppLinkLaunchRequest() == nil {
				return errors.New("missing launch request")
			}
			return eofOnly(c, r)
		})
		if err := client.LaunchApp(context.Background(), "com.google.android.youtube.tv"); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	for _, command := range []string{"volume", "mute", "launch"} {
		t.Run("unsupported-"+command, func(t *testing.T) {
			t.Parallel()
			client, done := fakeDevice(t, remote.FeatureKey, true, false, eofOnly)
			var err error
			switch command {
			case "volume":
				err = client.SendKey(context.Background(), "VOLUME_UP")
			case "mute":
				err = client.SetMute(context.Background(), true)
			case "launch":
				err = client.LaunchApp(context.Background(), "com.netflix.ninja")
			}
			if err == nil || !strings.Contains(err.Error(), "does not support") {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
