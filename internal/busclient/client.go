// Package busclient holds the few calls this service makes to the bus.
//
// The catalogue and the agent rows live in different services now: the bus owns
// the agents, so deleting a type has to ask it to clear the references first.
package busclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client posts to the bus's internal endpoints.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client. An empty base URL leaves it disabled: a deployment
// without a bus is a configuration mistake, and the caller reports it.
func New(baseURL, token string) *Client {
	return &Client{base: baseURL, token: token, http: &http.Client{Timeout: 5 * time.Second}}
}

// Enabled reports whether a base URL was configured.
func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// ClearAgentTypeRefs asks the bus to unset type_id on the agents pointing at the
// type about to be deleted.
func (c *Client) ClearAgentTypeRefs(ctx context.Context, typeID string) error {
	if !c.Enabled() {
		return fmt.Errorf("bus is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v1/internal/agent-types/"+url.PathEscape(typeID)+"/clear-refs", nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ask bus to clear type refs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bus answered %d clearing type refs", resp.StatusCode)
	}
	return nil
}
