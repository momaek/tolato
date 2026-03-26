package agentsdk

// ProviderConfig holds configuration for the agent-sdk-go based provider.
type ProviderConfig struct {
	// ProviderType selects the LLM backend: "openai", "anthropic", or "gemini".
	ProviderType string `json:"providerType" yaml:"providerType"`

	// Model is the model identifier (e.g. "gpt-4o", "claude-sonnet-4-20250514", "gemini-2.0-flash").
	Model string `json:"model" yaml:"model"`

	// APIKey is the authentication key for the selected provider.
	APIKey string `json:"apiKey" yaml:"apiKey"`

	// Endpoint is an optional custom API endpoint (useful for proxies or Azure).
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// Temperature controls randomness. 0 = deterministic, 1 = creative.
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`

	// MaxTokens limits the length of generated responses.
	MaxTokens int `json:"maxTokens,omitempty" yaml:"maxTokens,omitempty"`

	// EnableThinking enables extended thinking / reasoning tokens
	// (Anthropic extended thinking, Gemini thoughts).
	EnableThinking bool `json:"enableThinking,omitempty" yaml:"enableThinking,omitempty"`

	// ReasoningBudget is the optional token budget for reasoning (Anthropic only, minimum 1024).
	ReasoningBudget int `json:"reasoningBudget,omitempty" yaml:"reasoningBudget,omitempty"`

	// RunnerTimeoutSeconds is the maximum time a runner goroutine may live.
	// Default: 1800 (30 minutes).
	RunnerTimeoutSeconds int `json:"runnerTimeoutSeconds,omitempty" yaml:"runnerTimeoutSeconds,omitempty"`
}

func (c ProviderConfig) runnerTimeout() int {
	if c.RunnerTimeoutSeconds > 0 {
		return c.RunnerTimeoutSeconds
	}
	return 1800
}
