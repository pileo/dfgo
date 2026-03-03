package llm

// Capability flags for model features.
type Capability string

const (
	CapTools    Capability = "tools"
	CapVision   Capability = "vision"
	CapStreaming Capability = "streaming"
	CapThinking Capability = "thinking"
	CapCaching  Capability = "caching"
)

// ModelInfo holds static metadata about a known model.
type ModelInfo struct {
	ID              string       // e.g. "claude-sonnet-4-20250514"
	Provider        string       // "anthropic", "openai", "gemini"
	DisplayName     string       // e.g. "Claude Sonnet 4"
	ContextWindow   int          // max input tokens
	MaxOutputTokens int          // max output tokens (0 = use provider default)
	Capabilities    []Capability // supported features
	InputCostPer1M  float64      // USD per 1M input tokens (0 = unknown)
	OutputCostPer1M float64      // USD per 1M output tokens (0 = unknown)
}

// HasCapability reports whether the model supports the given capability.
func (m ModelInfo) HasCapability(cap Capability) bool {
	for _, c := range m.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

var models = []ModelInfo{
	// Anthropic
	{
		ID:              "claude-opus-4-20250514",
		Provider:        "anthropic",
		DisplayName:     "Claude Opus 4",
		ContextWindow:   200000,
		MaxOutputTokens: 32000,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming, CapThinking, CapCaching},
		InputCostPer1M:  15.0,
		OutputCostPer1M: 75.0,
	},
	{
		ID:              "claude-sonnet-4-20250514",
		Provider:        "anthropic",
		DisplayName:     "Claude Sonnet 4",
		ContextWindow:   200000,
		MaxOutputTokens: 16000,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming, CapThinking, CapCaching},
		InputCostPer1M:  3.0,
		OutputCostPer1M: 15.0,
	},
	{
		ID:              "claude-haiku-3-5-20241022",
		Provider:        "anthropic",
		DisplayName:     "Claude 3.5 Haiku",
		ContextWindow:   200000,
		MaxOutputTokens: 8192,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming, CapCaching},
		InputCostPer1M:  0.80,
		OutputCostPer1M: 4.0,
	},
	// OpenAI
	{
		ID:              "gpt-4o",
		Provider:        "openai",
		DisplayName:     "GPT-4o",
		ContextWindow:   128000,
		MaxOutputTokens: 16384,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming},
		InputCostPer1M:  2.50,
		OutputCostPer1M: 10.0,
	},
	{
		ID:              "gpt-4o-mini",
		Provider:        "openai",
		DisplayName:     "GPT-4o mini",
		ContextWindow:   128000,
		MaxOutputTokens: 16384,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming},
		InputCostPer1M:  0.15,
		OutputCostPer1M: 0.60,
	},
	{
		ID:              "o3-mini",
		Provider:        "openai",
		DisplayName:     "o3-mini",
		ContextWindow:   200000,
		MaxOutputTokens: 100000,
		Capabilities:    []Capability{CapTools, CapStreaming, CapThinking},
		InputCostPer1M:  1.10,
		OutputCostPer1M: 4.40,
	},
	// Gemini
	{
		ID:              "gemini-2.0-flash",
		Provider:        "gemini",
		DisplayName:     "Gemini 2.0 Flash",
		ContextWindow:   1048576,
		MaxOutputTokens: 8192,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming},
		InputCostPer1M:  0.10,
		OutputCostPer1M: 0.40,
	},
	{
		ID:              "gemini-2.5-pro-preview-05-06",
		Provider:        "gemini",
		DisplayName:     "Gemini 2.5 Pro",
		ContextWindow:   1048576,
		MaxOutputTokens: 65536,
		Capabilities:    []Capability{CapTools, CapVision, CapStreaming, CapThinking},
		InputCostPer1M:  1.25,
		OutputCostPer1M: 10.0,
	},
}

var latestModels = map[string]string{
	"anthropic": "claude-sonnet-4-20250514",
	"openai":    "gpt-4o",
	"gemini":    "gemini-2.5-pro-preview-05-06",
}

// modelIndex is a lookup map from model ID to ModelInfo, built at init.
var modelIndex map[string]ModelInfo

func init() {
	modelIndex = make(map[string]ModelInfo, len(models))
	for _, m := range models {
		modelIndex[m.ID] = m
	}
}

// GetModelInfo returns metadata for the given model ID, or false if unknown.
func GetModelInfo(modelID string) (ModelInfo, bool) {
	m, ok := modelIndex[modelID]
	return m, ok
}

// ListModels returns all known models, optionally filtered by provider.
// Pass an empty string to return all models.
func ListModels(provider string) []ModelInfo {
	if provider == "" {
		out := make([]ModelInfo, len(models))
		copy(out, models)
		return out
	}
	var out []ModelInfo
	for _, m := range models {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// GetLatestModel returns the recommended latest model for a provider.
func GetLatestModel(provider string) (ModelInfo, bool) {
	id, ok := latestModels[provider]
	if !ok {
		return ModelInfo{}, false
	}
	return GetModelInfo(id)
}
