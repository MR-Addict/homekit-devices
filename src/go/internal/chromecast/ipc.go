package chromecast

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/jkiddo/atvremote/pkg/v2/remote"
)

const maxIPCRequest = 2 << 20

type Response struct {
	Version   int     `json:"version"`
	Status    *Status `json:"status,omitempty"`
	ErrorCode string  `json:"error_code,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type ipcServer struct {
	listener net.Listener
	path     string
	session  *Session
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func startIPC(s *Session) (*ipcServer, error) {
	path := s.cfg.Chromecast.KeyPath + ".sock"
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("IPC path is not a socket: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on Chromecast IPC socket: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	ipc := &ipcServer{listener: listener, path: path, session: s, conns: make(map[net.Conn]struct{})}
	ipc.wg.Add(1)
	go ipc.accept()
	return ipc, nil
}
func (i *ipcServer) accept() {
	defer i.wg.Done()
	for {
		conn, err := i.listener.Accept()
		if err != nil {
			return
		}
		i.mu.Lock()
		if i.session.ctx.Err() != nil {
			i.mu.Unlock()
			conn.Close()
			continue
		}
		i.conns[conn] = struct{}{}
		i.wg.Add(1)
		i.mu.Unlock()
		go i.serve(conn)
	}
}
func errorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrBusy), errors.Is(err, ErrOppositePower):
		return "busy"
	case errors.Is(err, ErrQueueFull):
		return "queue_full"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrUnsupported):
		return "unsupported"
	case errors.Is(err, ErrStopped), errors.Is(err, ErrUnavailable):
		return "unavailable"
	}
	var auth *remote.InvalidAuth
	if errors.As(err, &auth) {
		return "auth_required"
	}
	var closed *remote.ConnectionClosed
	if errors.As(err, &closed) {
		return "unavailable"
	}
	return "command_failed"
}
func (i *ipcServer) serve(conn net.Conn) {
	defer i.wg.Done()
	defer func() { conn.Close(); i.mu.Lock(); delete(i.conns, conn); i.mu.Unlock() }()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(io.LimitReader(conn, maxIPCRequest+1))
	line, err := reader.ReadBytes('\n')
	response := Response{Version: 1}
	q := Request{}
	if err == nil && len(line) <= maxIPCRequest {
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&q)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("expected one JSON request")
			}
		}
		if err == nil {
			err = q.validate()
		}
	} else if len(line) > maxIPCRequest {
		err = errors.New("IPC request exceeds 2 MiB")
	}
	if err != nil {
		response.ErrorCode = "invalid_request"
		response.Error = err.Error()
		json.NewEncoder(conn).Encode(response)
		return
	}
	ctx, cancel := context.WithTimeout(i.session.ctx, q.timeout(i.session.cfg))
	defer cancel()
	conn.SetDeadline(time.Now().Add(q.timeout(i.session.cfg) + time.Second))
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); _, _ = reader.ReadByte(); cancel() }()
	defer func() { conn.Close(); <-monitorDone }()
	response.Status, err = i.session.Do(ctx, q)
	if err != nil {
		response.ErrorCode = errorCode(err)
		response.Error = err.Error()
	}
	json.NewEncoder(conn).Encode(response)
}
func (i *ipcServer) close() {
	i.listener.Close()
	i.mu.Lock()
	for conn := range i.conns {
		conn.Close()
	}
	i.mu.Unlock()
	i.wg.Wait()
	// This still runs under the credential lock.
	os.Remove(i.path)
}

// Forward returns handled=false only if no service accepted a connection.
// Once connected, any error is final: falling back could duplicate a command.
func Forward(ctx context.Context, cfg AppConfig, q Request) (Response, bool, error) {
	var response Response
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", cfg.Chromecast.KeyPath+".sock")
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return response, false, nil
		}
		return response, true, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	payload, err := json.Marshal(q)
	if err != nil {
		return response, true, err
	}
	if len(payload)+1 > maxIPCRequest {
		return response, true, errors.New("IPC request exceeds 2 MiB")
	}
	payload = append(payload, '\n')
	n, err := conn.Write(payload)
	if err == nil && n != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return response, true, err
	}
	decoder := json.NewDecoder(io.LimitReader(conn, maxIPCRequest))
	if err = decoder.Decode(&response); err != nil {
		return response, true, fmt.Errorf("service result unknown; command will not be retried: %w", err)
	}
	if response.Version != 1 {
		return response, true, errors.New("unsupported service IPC version")
	}
	if response.Error != "" {
		return response, true, &IPCError{Code: response.ErrorCode, Message: response.Error}
	}
	return response, true, nil
}

type IPCError struct{ Code, Message string }

func (e *IPCError) Error() string { return e.Message }
func (e *IPCError) Unwrap() error {
	switch e.Code {
	case "timeout":
		return context.DeadlineExceeded
	case "canceled":
		return context.Canceled
	case "queue_full":
		return ErrQueueFull
	case "busy":
		return ErrBusy
	case "unsupported":
		return ErrUnsupported
	case "unavailable":
		return ErrUnavailable
	}
	return nil
}

// RunRequest preserves standalone operation when no service owns the identity.
func RunRequest(parent context.Context, cfg AppConfig, q Request) (*Status, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, q.timeout(cfg))
	defer cancel()
	response, handled, err := Forward(ctx, cfg, q)
	if handled {
		if q.Operation == "status" && response.Status == nil {
			status := Status{}
			if err != nil {
				status.Error = err.Error()
			}
			response.Status = &status
		}
		return response.Status, err
	}
	client := &Client{Config: cfg.Chromecast, Timeout: Duration(cfg.Control.Timeout)}
	if q.Operation == "status" {
		status, err := client.Status(ctx)
		return &status, err
	}
	return nil, client.WithSession(ctx, func(r *remote.AndroidTVRemote) error { return perform(ctx, r, cfg, q) })
}
