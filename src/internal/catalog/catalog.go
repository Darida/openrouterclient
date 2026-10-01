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

	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

const (
	fetchTimeout = 30 * time.Second
	cacheTTL     = time.Hour
	// OpenRouter lists routers, which aren't models, at this placeholder price.
	routerPrice = "-1"
)

// Catalog is OpenRouter's model list, fetched at most once per cacheTTL.
type Catalog struct {
	URL  string
	HTTP *http.Client
	// Describes the catalog's raw body in panic messages.
	Replies replyfile.Saver

	mu        sync.Mutex
	fetchedAt time.Time
	models    []entry
	body      []byte
}

// Candidates lists tier's models that support strict json_schema output,
// skipping routers. It panics on a candidate whose price is negative or
// unparseable, or whose context length is missing.
func (c *Catalog) Candidates(tier model.ModelTier) []Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) >= cacheTTL {
		c.refresh()
	}
	var candidates []Model
	for _, m := range c.models {
		if inTier(m.ID, tier) && !isBatchOnly(m.ID) && !isRouter(m) && slices.Contains(m.SupportedParameters, "structured_outputs") && slices.Contains(m.Architecture.OutputModalities, "text") {
			candidates = append(candidates, Model{ID: m.ID, ContextTokens: c.contextTokens(m), PromptUSDPerToken: c.price(m.ID, "prompt", m.Pricing.Prompt), CompletionUSDPerToken: c.price(m.ID, "completion", m.Pricing.Completion)})
		}
	}
	return candidates
}

func (c *Catalog) refresh() {
	c.body = c.fetch()
	c.models, c.fetchedAt = c.parse(c.body), time.Now()
}

func (c *Catalog) fetch() []byte {
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
		panic(fmt.Sprintf("catalog: %s returned HTTP %d: %s", c.URL, resp.StatusCode, c.Replies.Describe("", "catalog", body)))
	}
	return body
}

func (c *Catalog) parse(body []byte) []entry {
	var parsed struct {
		Data []entry `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		panic(fmt.Sprintf("catalog: %s returned no models: %v — %s", c.URL, err, c.Replies.Describe("", "catalog", body)))
	}
	for _, m := range parsed.Data {
		if m.ID == "" {
			panic(fmt.Sprintf("catalog: model entry without id: %s", c.Replies.Describe("", "catalog", body)))
		}
	}
	return parsed.Data
}

// OpenRouter's own "openrouter/…" entries are routers, not models.
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

func isRouter(m entry) bool {
	return m.Pricing.Prompt == routerPrice || m.Pricing.Completion == routerPrice
}

// A ":batch" variant rejects the chat/completions endpoint with a 404.
func isBatchOnly(id string) bool {
	return strings.HasSuffix(id, ":batch")
}

func (c *Catalog) contextTokens(m entry) int {
	if m.ContextLength <= 0 {
		panic(fmt.Sprintf("catalog: model %q has no context length: %s", m.ID, c.Replies.Describe("", "catalog", c.body)))
	}
	return m.ContextLength
}

func (c *Catalog) price(id, kind, raw string) float64 {
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 {
		panic(fmt.Sprintf("catalog: model %q has an invalid %s price %q: %s", id, kind, raw, c.Replies.Describe("", "catalog", c.body)))
	}
	return usd
}
