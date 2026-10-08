package wol

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// PVEClient is owned by one device controller. Node discovery is retried until
// successful, then cached. Responses and credentials are never included in errors.
type PVEClient struct {
	host, token, node, base string
	client                  *http.Client
}

func NewPVEClient(o PowerOptions) *PVEClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Explicit product choice: use PVE's existing HTTPS certificate without
	// requiring local certificate files. This does not authenticate the peer.
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	// The LAN host must be reached directly, not through environment proxies.
	transport.Proxy = nil
	return &PVEClient{host: o.Host, token: o.Token, base: "https://" + o.Host + ":8006/api2/json", client: &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// request reports reachability separately from successful API execution.
func (p *PVEClient) request(ctx context.Context, method, path string, body url.Values, result any) (bool, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(body.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, reader)
	if err != nil {
		return false, errors.New("construct PVE request")
	}
	req.Header.Set("Authorization", "PVEAPIToken="+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, errors.New("PVE request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return true, fmt.Errorf("PVE API HTTP %d", resp.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result); err != nil {
			return true, errors.New("invalid PVE API response")
		}
	}
	return true, nil
}
func (p *PVEClient) discover(ctx context.Context) (bool, error) {
	if p.node != "" {
		return false, nil
	}
	var reply struct {
		Data []struct {
			Node string `json:"node"`
		} `json:"data"`
	}
	reachable, err := p.request(ctx, http.MethodGet, "/nodes", nil, &reply)
	if err != nil {
		return reachable, err
	}
	if len(reply.Data) != 1 || reply.Data[0].Node == "" {
		return true, errors.New("PVE requires exactly one discoverable node")
	}
	p.node = reply.Data[0].Node
	return true, nil
}
func (p *PVEClient) Status(ctx context.Context) (bool, error) {
	if reachable, err := p.discover(ctx); err != nil {
		return reachable, err
	}
	var reply struct {
		Data *struct {
			Uptime *float64 `json:"uptime"`
		} `json:"data"`
	}
	reachable, err := p.request(ctx, http.MethodGet, "/nodes/"+url.PathEscape(p.node)+"/status", nil, &reply)
	if err == nil && (reply.Data == nil || reply.Data.Uptime == nil) {
		err = errors.New("invalid PVE node status")
	}
	return reachable, err
}
func (p *PVEClient) Shutdown(ctx context.Context) error {
	if _, err := p.discover(ctx); err != nil {
		return err
	}
	_, err := p.request(ctx, http.MethodPost, "/nodes/"+url.PathEscape(p.node)+"/status", url.Values{"command": {"shutdown"}}, nil)
	return err
}
func (p *PVEClient) Close() { p.client.CloseIdleConnections() }

func pingHost(ctx context.Context, host string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// BusyBox/OpenWrt and Linux iputils both accept these arguments.
	err := exec.CommandContext(ctx, "ping", "-n", "-c", "1", "-W", "2", host).Run()
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, errors.New("ICMP probe canceled or timed out")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, errors.New("ICMP probe failed")
}
