package lgnode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

var (
	jsonMarshal    = json.Marshal
	newHTTPRequest = http.NewRequestWithContext
)

type Driver struct {
	node           *domain.Node
	cfg            *config.Config
	httpClient     *http.Client
	streamClient   *http.Client
	circuitBreaker *circuitbreaker.CircuitBreaker
}

func NewDriver(node *domain.Node, cfg *config.Config) (*Driver, error) {
	return &Driver{
		node:           node,
		cfg:            cfg,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		streamClient:   &http.Client{},
		circuitBreaker: circuitbreaker.New(5, 30*time.Second),
	}, nil
}

func (d *Driver) Capabilities() []domain.CommandType {
	return d.node.EnabledCmds
}

func (d *Driver) TestConnection(ctx context.Context) error {
	base := resolveLocalAgentURL(d.node.AgentURL)
	return d.circuitBreaker.Call(func() error {
		req, err := newHTTPRequest(ctx, "GET", agentURL(base, "/agent/v1/health"), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+d.node.AgentToken)
		resp, err := d.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health check failed: %d", resp.StatusCode)
		}
		return nil
	})
}

func (d *Driver) Ping(ctx context.Context, target string, count int) (*domain.PingResult, error) {
	if !d.node.CanExecute(domain.CmdPing) {
		return nil, domain.ErrCommandDisabled
	}

	body := map[string]interface{}{
		"target": target,
		"count":  count,
	}

	if onLine := domain.GetOnLine(ctx); onLine != nil {
		var result domain.PingResult
		err := d.doAgentStreamRequest(ctx, "/agent/v1/ping/stream", body, onLine, &result)
		return &result, err
	}

	var result domain.PingResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/ping", body, &result)
	return &result, err
}

func (d *Driver) Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if !d.node.CanExecute(domain.CmdTraceroute) {
		return nil, domain.ErrCommandDisabled
	}

	body := map[string]interface{}{
		"target":   target,
		"max_hops": maxHops,
	}

	if onLine := domain.GetOnLine(ctx); onLine != nil {
		var result domain.TracerouteResult
		err := d.doAgentStreamRequest(ctx, "/agent/v1/traceroute/stream", body, onLine, &result)
		return &result, err
	}

	var result domain.TracerouteResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/traceroute", body, &result)
	return &result, err
}

func (d *Driver) BGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error) {
	if !d.node.CanExecute(domain.CmdBGPRoute) {
		return nil, domain.ErrCommandDisabled
	}

	var result domain.BGPResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/bgp/route", map[string]interface{}{
		"prefix": prefix,
	}, &result)
	return &result, err
}

func (d *Driver) doAgentRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	return d.circuitBreaker.Call(func() error {
		jsonBody, err := jsonMarshal(body)
		if err != nil {
			return err
		}

		req, err := newHTTPRequest(ctx, method, agentURL(resolveLocalAgentURL(d.node.AgentURL), path), bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+d.node.AgentToken)

		resp, err := d.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("agent request failed: %d", resp.StatusCode)
		}

		return json.NewDecoder(resp.Body).Decode(result)
	})
}

func (d *Driver) doAgentStreamRequest(ctx context.Context, path string, body interface{}, onLine func(string), result interface{}) error {
	return d.circuitBreaker.Call(func() error {
		jsonBody, err := jsonMarshal(body)
		if err != nil {
			return err
		}

		req, err := newHTTPRequest(ctx, "POST", agentURL(resolveLocalAgentURL(d.node.AgentURL), path), bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+d.node.AgentToken)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := d.streamClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return d.doAgentRequest(ctx, "POST", strings.TrimSuffix(path, "/stream"), body, result)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("agent stream request failed: %d", resp.StatusCode)
		}

		var streamErr error
		gotResult := false
		err = readSSE(resp.Body, func(event, data string) error {
			switch event {
			case "output", "":
				if event == "" && !strings.Contains(data, `"line"`) {
					return nil
				}
				line, err := parseOutputLine(data)
				if err != nil {
					return nil
				}
				if line != "" {
					onLine(line)
				}
			case "result":
				if err := json.Unmarshal([]byte(data), result); err != nil {
					return err
				}
				gotResult = true
			case "error":
				streamErr = parseStreamError(data)
			}
			return nil
		})
		if err != nil && err != io.EOF {
			return err
		}
		if streamErr != nil {
			return streamErr
		}
		if !gotResult {
			return fmt.Errorf("agent stream ended without result")
		}
		return nil
	})
}

func agentURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}
