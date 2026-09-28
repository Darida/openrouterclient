package generationlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Timing struct {
	// How long a generation may take to appear in the log before ResolveModel panics.
	Window       time.Duration
	PollInterval time.Duration
}

type Client struct {
	URL    string
	APIKey string
	HTTP   *http.Client
	Timing Timing
}

// ResolveModel polls OpenRouter's generation log until it names the model
// behind generationID. Cutting our own connection doesn't stop the provider,
// so the record may not exist yet right after an abort. It panics if the
// window expires or the log answers with anything unexpected.
func (c *Client) ResolveModel(generationID string) string {
	deadline := time.Now().Add(c.Timing.Window)
	var lastErr string
	for {
		model, pending, err := c.lookupOnce(generationID, deadline)
		if model != "" {
			return model
		}
		if err != nil {
			lastErr = err.Error()
		} else if pending {
			lastErr = "generation not in log yet (HTTP 404)"
		}
		if !time.Now().Add(c.Timing.PollInterval).Before(deadline) {
			panic(fmt.Sprintf("generationlog: generation %q did not resolve to a model within %s; last result: %s", generationID, c.Timing.Window, lastErr))
		}
		time.Sleep(c.Timing.PollInterval)
	}
}

// Returns pending for a 404, and err for a transport failure worth retrying.
// Any other non-200 status, or a 200 without a model, panics.
func (c *Client) lookupOnce(generationID string, deadline time.Time) (model string, pending bool, err error) {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"?id="+url.QueryEscape(generationID), nil)
	if err != nil {
		panic(fmt.Sprintf("generationlog: build request: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", true, nil
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("generationlog: generation %q lookup returned HTTP %d: %s", generationID, resp.StatusCode, body))
	}
	var parsed struct {
		Data struct {
			Model string `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Data.Model == "" {
		panic(fmt.Sprintf("generationlog: generation %q lookup returned no data.model — body: %s", generationID, body))
	}
	return parsed.Data.Model, false, nil
}
