// Package provisionerclient calls the provisioning module's internal API.
//
// The container control plane is the admin module's (the console's restart,
// dispose and reprovision routes point here), but the queue and the container
// runtime API are the provisioner's: each action is a "desired state" write on
// the owner's side, which the provisioner's poll loop then executes. Asking the
// owner is the same shape configuration has (every module asks this service).
package provisionerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client calls the provisioner's internal API with the deployment's shared
// token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a client. An empty base URL leaves it disabled, and every action
// then fails loudly instead of pretending to have been accepted.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled reports whether a base URL was configured.
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

// StatusError carries the provisioner's own verdict so the handler can pass it
// through: 404 means "no such container request", which the console shows as a
// missing row rather than a gateway failure.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("the provisioner answered %d", e.Code)
	}
	return fmt.Sprintf("the provisioner answered %d: %s", e.Code, e.Body)
}

// CodeOf returns the HTTP status behind err, or 0 when it was not that kind of
// failure.
func CodeOf(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return 0
}

// Dispose asks for the user's container to be torn down.
func (c *Client) Dispose(ctx context.Context, userID string) error {
	return c.post(ctx, "/api/v1/internal/containers/"+url.PathEscape(userID)+"/dispose")
}

// Restart asks for the user's container to be recreated in place.
func (c *Client) Restart(ctx context.Context, userID string) error {
	return c.post(ctx, "/api/v1/internal/containers/"+url.PathEscape(userID)+"/restart")
}

// Reprovision revives a disposed or failed container request.
func (c *Client) Reprovision(ctx context.Context, userID string) error {
	return c.post(ctx, "/api/v1/internal/containers/"+url.PathEscape(userID)+"/reprovision")
}

func (c *Client) post(ctx context.Context, path string) error {
	if !c.Enabled() {
		return fmt.Errorf("the provisioner module is not configured")
	}
	body, err := json.Marshal(map[string]any{})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call the provisioner %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(detail))}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
