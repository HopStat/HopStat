package lgnode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yourorg/lg-looking-glass/internal/circuitbreaker"
	"github.com/yourorg/lg-looking-glass/internal/config"
	"github.com/yourorg/lg-looking-glass/internal/domain"
)

type Driver struct {
	node          *domain.Node
	cfg           *config.Config
	httpClient    *http.Client
	circuitBreaker *circuitbreaker.CircuitBreaker
}

func NewDriver(node *domain.Node, cfg *config.Config) (*Driver, error) {
	return &Driver{
		node:          node,
		cfg:           cfg,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		circuitBreaker: circuitbreaker.New(5, 30*time.Second),
	}, nil
}

func (d *Driver) Capabilities() []domain.CommandType {
	return d.node.EnabledCmds
}

func (d *Driver) TestConnection(ctx context.Context) error {
	return d.circuitBreaker.Call(func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", d.node.AgentURL+"/agent/v1/health", nil)
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

	var result domain.PingResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/ping", map[string]interface{}{
		"target": target,
		"count":  count,
	}, &result)
	return &result, err
}

func (d *Driver) Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if !d.node.CanExecute(domain.CmdTraceroute) {
		return nil, domain.ErrCommandDisabled
	}

	var result domain.TracerouteResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/traceroute", map[string]interface{}{
		"target":   target,
		"max_hops": maxHops,
	}, &result)
	return &result, err
}

func (d *Driver) MTR(ctx context.Context, target string, cycles int) (*domain.MTRResult, error) {
	if !d.node.CanExecute(domain.CmdMTR) {
		return nil, domain.ErrCommandDisabled
	}

	var result domain.MTRResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/mtr", map[string]interface{}{
		"target": target,
		"cycles": cycles,
	}, &result)
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

func (d *Driver) ASPath(ctx context.Context, asn uint32) (*domain.ASPathResult, error) {
	if !d.node.CanExecute(domain.CmdASPath) {
		return nil, domain.ErrCommandDisabled
	}

	var result domain.ASPathResult
	err := d.doAgentRequest(ctx, "POST", "/agent/v1/bgp/aspath", map[string]interface{}{
		"asn": asn,
	}, &result)
	return &result, err
}

func (d *Driver) doAgentRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	return d.circuitBreaker.Call(func() error {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, d.node.AgentURL+path, bytes.NewReader(jsonBody))
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