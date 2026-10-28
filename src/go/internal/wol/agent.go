package wol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"homekit-devices/internal/poweragent"
)

func newPowerBackend(o PowerOptions) powerBackend {
	if o.Type == "agent" {
		return NewAgentClient(o)
	}
	return NewPVEClient(o)
}

type AgentClient struct {
	base, token string
	client      *http.Client
}

func NewAgentClient(o PowerOptions) *AgentClient {
	port := o.Port
	if port == 0 {
		port = poweragent.DefaultPort
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &AgentClient{base: "http://" + net.JoinHostPort(o.Host, strconv.Itoa(port)), token: o.Token, client: &http.Client{Transport: tr, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *AgentClient) request(ctx context.Context, method, path string, code int, result any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return false, errors.New("construct agent request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, errors.New("agent unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != code {
		return true, fmt.Errorf("agent HTTP %d", resp.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(result); err != nil {
			return true, errors.New("invalid agent response")
		}
	}
	return true, nil
}

func (c *AgentClient) Status(ctx context.Context) (bool, error) {
	var status poweragent.Status
	reachable, err := c.request(ctx, "GET", "/api/status", http.StatusOK, &status)
	if err == nil && (status.Hostname == "" || status.BootID == "" || (status.State != "running" && status.State != "shutting_down")) {
		err = errors.New("invalid agent status")
	}
	return reachable, err
}
func (c *AgentClient) Shutdown(ctx context.Context) error {
	_, err := c.request(ctx, "POST", "/api/shutdown", http.StatusAccepted, nil)
	return err
}
func (c *AgentClient) Close() { c.client.CloseIdleConnections() }
