package catalog

// entry is the part of one /api/v1/models item this package reads.
type entry struct {
	ID                  string   `json:"id"`
	ContextLength       int      `json:"context_length"`
	SupportedParameters []string `json:"supported_parameters"`
	Architecture        struct {
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	// USD per token, as decimal strings.
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// Model is a candidate a request may be sent to.
type Model struct {
	ID string
	// ContextTokens bounds prompt plus output tokens in one request.
	ContextTokens         int
	PromptUSDPerToken     float64
	CompletionUSDPerToken float64
}
