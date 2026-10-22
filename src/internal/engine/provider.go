package engine

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/Darida/openrouterclient/src/internal/catalog"
	"github.com/Darida/openrouterclient/src/internal/chat"
	"github.com/Darida/openrouterclient/src/internal/cost"
	"github.com/Darida/openrouterclient/src/model"
)

// PickProvider returns the cheapest of req.Model's endpoints that can serve a
// chat request of req's size, reading only OpenRouter's public listings.
func (e *Engine) PickProvider(ctx context.Context, req model.ProviderRequest) (model.ProviderChoice, error) {
	choice, err := e.pickProvider(req)
	return choice, publicError(ctx, nil, err)
}

type pricedEndpoint struct {
	endpoint catalog.Endpoint
	price    catalog.Price
	costUSD  float64
}

func (e *Engine) pickProvider(req model.ProviderRequest) (model.ProviderChoice, error) {
	if err := validateProviderRequest(req); err != nil {
		return model.ProviderChoice{}, err
	}
	all, err := e.catalog.AllCandidates()
	if err != nil {
		return model.ProviderChoice{}, err
	}
	if !slices.ContainsFunc(all, func(m catalog.Model) bool { return m.ID == req.Model }) {
		return model.ProviderChoice{}, fmt.Errorf("engine: %q is not a structured-output model in OpenRouter's catalog", req.Model)
	}
	endpoints, err := e.catalog.Endpoints(req.Model)
	if err != nil {
		return model.ProviderChoice{}, err
	}
	servable, err := servableEndpoints(req, endpoints)
	if err != nil {
		return model.ProviderChoice{}, err
	}
	// Ties go to the lower tag, so the same listing always yields the same pick.
	cheapest := slices.MinFunc(servable, func(a, b pricedEndpoint) int {
		return cmp.Or(cmp.Compare(a.costUSD, b.costUSD), cmp.Compare(a.endpoint.Tag, b.endpoint.Tag))
	})
	e.logger.Info("openrouter: provider picked", "model", req.Model, "provider", cheapest.endpoint.Tag, "of", len(servable), "costUSD", cheapest.costUSD)
	return model.ProviderChoice{
		Model:                 req.Model,
		Provider:              cheapest.endpoint.Tag,
		ProviderName:          cheapest.endpoint.ProviderName,
		CostUSD:               cheapest.costUSD,
		PromptUSDPerToken:     cheapest.price.Prompt,
		CompletionUSDPerToken: cheapest.price.Completion,
		RequestUSD:            cheapest.price.Request,
	}, nil
}

// servableEndpoints keeps, step by step, the endpoints that support every
// parameter a chat request sends, fit its tokens, and price within its caps,
// and errors naming the first step that leaves none.
func servableEndpoints(req model.ProviderRequest, endpoints []catalog.Endpoint) ([]pricedEndpoint, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("engine: OpenRouter lists no endpoint for %q", req.Model)
	}
	supporting := slices.DeleteFunc(slices.Clone(endpoints), func(ep catalog.Endpoint) bool {
		return !supportsAll(ep, chat.RequiredParameters)
	})
	if len(supporting) == 0 {
		return nil, fmt.Errorf("engine: no endpoint of %q supports all of %v", req.Model, chat.RequiredParameters)
	}
	fitting := slices.DeleteFunc(supporting, func(ep catalog.Endpoint) bool { return !fits(ep, req) })
	if len(fitting) == 0 {
		return nil, fmt.Errorf("engine: no endpoint of %q has room for %d prompt + %d output tokens", req.Model, req.InputTokens, req.OutputTokens)
	}
	var capped []pricedEndpoint
	for _, ep := range fitting {
		price := cost.EndpointPrice(ep, req.InputTokens)
		if price.Prompt*1e6 > chat.MaxPromptUSDPerMillion || price.Completion*1e6 > chat.MaxCompletionUSDPerMillion {
			continue
		}
		capped = append(capped, pricedEndpoint{endpoint: ep, price: price, costUSD: cost.EndpointEstimate(price, req.InputTokens, req.OutputTokens)})
	}
	if len(capped) == 0 {
		return nil, fmt.Errorf("engine: every endpoint of %q able to serve the request is priced above $%d / $%d per 1M prompt / completion tokens", req.Model, chat.MaxPromptUSDPerMillion, chat.MaxCompletionUSDPerMillion)
	}
	return capped, nil
}

func supportsAll(ep catalog.Endpoint, parameters []string) bool {
	for _, p := range parameters {
		if !slices.Contains(ep.SupportedParameters, p) {
			return false
		}
	}
	return true
}

func fits(ep catalog.Endpoint, req model.ProviderRequest) bool {
	if req.InputTokens+req.OutputTokens > ep.ContextTokens {
		return false
	}
	if ep.MaxPromptTokens != nil && req.InputTokens > *ep.MaxPromptTokens {
		return false
	}
	return ep.MaxCompletionTokens == nil || req.OutputTokens <= *ep.MaxCompletionTokens
}
