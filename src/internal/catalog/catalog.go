package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

const (
	fetchTimeout = 30 * time.Second
	cacheTTL     = time.Hour
)

// Catalog is OpenRouter's model list, fetched at most once per cacheTTL.
type Catalog struct {
	URL  string
	HTTP *http.Client

	mu        sync.Mutex
	fetchedAt time.Time
	models    []entry
}

// Candidates lists tier's models that support strict json_schema output. It
// panics on a candidate whose price is negative or unparseable, or whose
// context length is missing.
func (c *Catalog) Candidates(tier model.ModelTier) []Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) >= cacheTTL {
		c.refresh()
	}
	var candidates []Model
	for _, m := range c.models {
		if inTier(m.ID, tier) && !isBatchOnly(m.ID) && slices.Contains(m.SupportedParameters, "structured_outputs") && slices.Contains(m.Architecture.OutputModalities, "text") {
			candidates = append(candidates, Model{ID: m.ID, ContextTokens: contextTokens(m), PromptUSDPerToken: price(m.ID, "prompt", m.Pricing.Prompt), CompletionUSDPerToken: price(m.ID, "completion", m.Pricing.Completion)})
		}
	}
	return candidates
}

func (c *Catalog) refresh() {
	c.models, c.fetchedAt = c.fetch(), time.Now()
}

func (c *Catalog) fetch() []entry {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		panic(fmt.Sprintf("catalog: build request: %v", err))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		panic(fmt.Sprintf("catalog: fetch %s: %v", c.URL, err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Sprintf("catalog: read %s: %v", c.URL, err))
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("catalog: %s returned HTTP %d: %s", c.URL, resp.StatusCode, body))
	}
	var parsed struct {
		Data []entry `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		panic(fmt.Sprintf("catalog: %s returned no models: %v", c.URL, err))
	}
	for _, m := range parsed.Data {
		if m.ID == "" {
			panic(fmt.Sprintf("catalog: model entry without id: %+v", m))
		}
	}
	return parsed.Data
}

// OpenRouter's own "openrouter/…" entries are routers, not models, and list
// placeholder prices such as -1.
func inTier(id string, tier model.ModelTier) bool {
	free := strings.HasSuffix(id, ":free")
	switch tier {
	case model.ModelTierFree:
		return free
	case model.ModelTierPaid:
		return !free && !strings.HasPrefix(id, "openrouter/")
	}
	panic(fmt.Sprintf("catalog: unknown model tier %q", tier))
}

// A ":batch" variant rejects the chat/completions endpoint with a 404.
func isBatchOnly(id string) bool {
	return strings.HasSuffix(id, ":batch")
}

func contextTokens(m entry) int {
	if m.ContextLength <= 0 {
		panic(fmt.Sprintf("catalog: model %q has no context length: %+v", m.ID, m))
	}
	return m.ContextLength
}

func price(id, kind, raw string) float64 {
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 {
		panic(fmt.Sprintf("catalog: model %q has an invalid %s price %q", id, kind, raw))
	}
	return usd
}
