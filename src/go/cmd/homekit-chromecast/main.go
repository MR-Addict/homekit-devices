package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/brutella/hap/accessory"
	"github.com/jkiddo/atvremote/pkg/v2/remote"
	"homekit-devices/internal/chromecast"
	"homekit-devices/internal/hapserver"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	flags := flag.NewFlagSet("homekit-chromecast", flag.ContinueOnError)
	path := flags.String("config", "config.yaml", "YAML configuration path")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "homekit-chromecast [-config FILE] COMMAND\nCommands: serve discover pair status watch on off keys key text launch volume mute voice")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		args = []string{"serve"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if args[0] == "keys" && len(args) == 1 {
		return chromecast.WriteJSON(os.Stdout, chromecast.Keys())
	}
	if args[0] == "discover" && len(args) == 1 {
		devices, err := remote.Discover(ctx, 3*time.Second)
		if err != nil {
			return err
		}
		return chromecast.WriteJSON(os.Stdout, devices)
	}
	cfg, err := chromecast.Load(*path)
	if err != nil {
		return err
	}
	if args[0] == "serve" {
		if len(args) != 1 {
			return errors.New("serve takes no arguments")
		}
		controller := chromecast.NewController(cfg)
		return hapserver.Run(ctx, cfg.HomeKit, []*accessory.A{controller.Accessory.A}, controller.Poll)
	}
	return cast(ctx, cfg, args)
}

func cast(ctx context.Context, cfg chromecast.AppConfig, args []string) error {
	if len(args) == 0 {
		return errors.New("command required")
	}
	client := &chromecast.Client{Config: cfg.Chromecast, Timeout: chromecast.Duration(cfg.Control.Timeout)}
	switch args[0] {
	case "pair":
		if len(args) != 1 {
			return errors.New("pair takes no arguments")
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		return client.Pair(ctx, os.Stdin, os.Stderr)
	case "status":
		if len(args) != 1 {
			return errors.New("status takes no arguments")
		}
		s, err := client.Status(ctx)
		return errors.Join(err, chromecast.WriteJSON(os.Stdout, s))
	case "watch":
		if len(args) != 1 {
			return errors.New("watch takes no arguments")
		}
		for {
			s, _ := client.Status(ctx)
			if err := chromecast.WriteJSON(os.Stdout, s); err != nil {
				return err
			}
			if err := chromecast.Sleep(ctx, chromecast.Duration(cfg.Control.Interval)); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
		}
	case "on", "off":
		if len(args) != 1 {
			return errors.New("power takes no arguments")
		}
		op, cancel := context.WithTimeout(ctx, chromecast.Duration(cfg.Control.TransitionTimeout))
		defer cancel()
		return client.SetPower(op, args[0] == "on")
	case "key", "text", "launch", "volume", "mute", "voice":
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	// Each command has an overall deadline, including its session and write.
	timeout := chromecast.Duration(cfg.Control.Timeout) + time.Second
	var hold time.Duration
	var name string
	var input io.Reader
	volumeSteps := 1
	switch args[0] {
	case "key":
		f := flag.NewFlagSet("key", flag.ContinueOnError)
		h := f.Duration("hold", 0, "long press duration (max 30s)")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if len(f.Args()) != 1 {
			return errors.New("key [-hold 2s] KEYCODE_NAME")
		}
		hold = *h
		name = strings.ToUpper(f.Args()[0])
		if hold < 0 || hold > 30*time.Second {
			return errors.New("hold must be 0..30s")
		}
		timeout += hold
	case "text", "launch":
		if len(args) != 2 {
			return fmt.Errorf("%s requires exactly one quoted argument", args[0])
		}
	case "volume":
		if len(args) < 2 || len(args) > 3 || args[1] != "up" && args[1] != "down" {
			return errors.New("volume up|down [steps]")
		}
		if len(args) == 3 {
			var err error
			volumeSteps, err = strconv.Atoi(args[2])
			if err != nil || volumeSteps < 1 || volumeSteps > 100 {
				return errors.New("volume steps must be 1..100")
			}
		}
		timeout += time.Duration(volumeSteps) * 50 * time.Millisecond
	case "mute":
		if len(args) != 1 {
			return errors.New("mute takes no arguments")
		}
	case "voice":
		if len(args) != 2 {
			return errors.New("voice RAW_PCM_FILE (or - for stdin)")
		}
		if args[1] == "-" {
			input = os.Stdin
		} else {
			file, err := os.Open(args[1])
			if err != nil {
				return err
			}
			defer file.Close()
			input = file
		}
		timeout = 70 * time.Second
	}
	// Read stdin before connecting: no socket is held while waiting for EOF.
	if args[0] == "voice" {
		readCtx, readCancel := context.WithTimeout(ctx, 70*time.Second)
		defer readCancel()
		var stopRead func() bool
		if closer, ok := input.(io.Closer); ok {
			stopRead = context.AfterFunc(readCtx, func() { closer.Close() })
			defer stopRead()
		}
		audio, err := io.ReadAll(io.LimitReader(input, 960001))
		if readCtx.Err() != nil {
			return readCtx.Err()
		}
		if err != nil {
			return err
		}
		input = strings.NewReader(string(audio))
	}
	op, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.WithSession(op, func(r *remote.AndroidTVRemote) error {
		switch args[0] {
		case "key":
			return chromecast.Key(op, r, name, hold)
		case "text":
			return chromecast.Text(op, r, args[1])
		case "launch":
			return chromecast.Launch(r, args[1])
		case "mute":
			if err := chromecast.Require(r, remote.FeatureVolume); err != nil {
				return err
			}
			return chromecast.Key(op, r, "VOLUME_MUTE", 0)
		case "volume":
			if err := chromecast.Require(r, remote.FeatureVolume); err != nil {
				return err
			}
			key := "VOLUME_UP"
			if args[1] == "down" {
				key = "VOLUME_DOWN"
			}
			for i := 0; i < volumeSteps; i++ {
				if err := chromecast.Key(op, r, key, 0); err != nil {
					return err
				}
				if err := chromecast.Sleep(op, 50*time.Millisecond); err != nil {
					return err
				}
			}
			return nil
		case "voice":
			return chromecast.Voice(op, r, input)
		}
		return nil
	})
}
