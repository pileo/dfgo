package llm

import "testing"

func TestGetModelInfo(t *testing.T) {
	t.Run("known model", func(t *testing.T) {
		info, ok := GetModelInfo("claude-sonnet-4-6")
		if !ok {
			t.Fatal("expected to find claude-sonnet-4-6")
		}
		if info.Provider != "anthropic" {
			t.Errorf("provider = %q, want %q", info.Provider, "anthropic")
		}
		if info.DisplayName != "Claude Sonnet 4.6" {
			t.Errorf("display name = %q, want %q", info.DisplayName, "Claude Sonnet 4.6")
		}
		if info.ContextWindow != 200000 {
			t.Errorf("context window = %d, want %d", info.ContextWindow, 200000)
		}
		if info.MaxOutputTokens != 64000 {
			t.Errorf("max output tokens = %d, want %d", info.MaxOutputTokens, 64000)
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		_, ok := GetModelInfo("nonexistent-model")
		if ok {
			t.Fatal("expected unknown model to return false")
		}
	})
}

func TestGetModelInfoCapability(t *testing.T) {
	info, ok := GetModelInfo("claude-sonnet-4-6")
	if !ok {
		t.Fatal("expected to find model")
	}

	if !info.HasCapability(CapTools) {
		t.Error("expected model to have tools capability")
	}
	if !info.HasCapability(CapVision) {
		t.Error("expected model to have vision capability")
	}
	if !info.HasCapability(CapThinking) {
		t.Error("expected model to have thinking capability")
	}

	// GPT-4.1 does not have thinking.
	gpt, ok := GetModelInfo("gpt-4.1")
	if !ok {
		t.Fatal("expected to find gpt-4.1 model")
	}
	if gpt.HasCapability(CapThinking) {
		t.Error("expected gpt-4.1 to not have thinking capability")
	}
}

func TestListModels(t *testing.T) {
	t.Run("all models", func(t *testing.T) {
		all := ListModels("")
		if len(all) != len(models) {
			t.Errorf("got %d models, want %d", len(all), len(models))
		}
	})

	t.Run("filter by provider", func(t *testing.T) {
		anthropic := ListModels("anthropic")
		if len(anthropic) != 3 {
			t.Errorf("got %d anthropic models, want 3", len(anthropic))
		}
		for _, m := range anthropic {
			if m.Provider != "anthropic" {
				t.Errorf("got provider %q in anthropic list", m.Provider)
			}
		}

		openai := ListModels("openai")
		if len(openai) != 4 {
			t.Errorf("got %d openai models, want 4", len(openai))
		}

		gemini := ListModels("gemini")
		if len(gemini) != 2 {
			t.Errorf("got %d gemini models, want 2", len(gemini))
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		unknown := ListModels("unknown")
		if len(unknown) != 0 {
			t.Errorf("got %d models for unknown provider, want 0", len(unknown))
		}
	})
}

func TestGetLatestModel(t *testing.T) {
	tests := []struct {
		provider string
		wantID   string
		wantOK   bool
	}{
		{"anthropic", "claude-sonnet-4-6", true},
		{"openai", "o3", true},
		{"gemini", "gemini-2.5-pro", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			info, ok := GetLatestModel(tt.provider)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && info.ID != tt.wantID {
				t.Errorf("id = %q, want %q", info.ID, tt.wantID)
			}
		})
	}
}
