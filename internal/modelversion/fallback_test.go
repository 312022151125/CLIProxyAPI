package modelversion

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func registerTestModels(t *testing.T, clientID, provider string, ids ...string) {
	t.Helper()
	reg := registry.GetGlobalRegistry()
	models := make([]*registry.ModelInfo, 0, len(ids))
	for _, id := range ids {
		models = append(models, &registry.ModelInfo{ID: id})
	}
	reg.RegisterClient(clientID, provider, models)
	t.Cleanup(func() { reg.UnregisterClient(clientID) })
}

func TestNextFallbackModel(t *testing.T) {
	const client = "modelversion-fallback-test"
	registerTestModels(t, client, "openai",
		"claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8",
		"claude-sonnet-4-5", "claude-sonnet-4-6",
		"glm-5", "glm-5.1", "glm-5.2",
		"kimi-k2.5", "kimi-k2.6", "kimi-k2.7", "kimi-k2.7-code", "kimi-k2.7-code-highspeed",
		"gpt-5.4", "gpt-5.4-mini", "gpt-5.5",
	)

	tests := []struct {
		in   string
		want string
	}{
		{"claude-opus-4-8", "claude-opus-4-7"},
		{"claude-opus-4-7", "claude-opus-4-6"},
		{"claude-opus-4-6", ""},
		{"claude-opus-4-8(high)", "claude-opus-4-7(high)"},
		{"claude-opus-4-7(16384)", "claude-opus-4-6(16384)"},
		{"claude-opus-4-8-20250801", "claude-opus-4-7"},
		{"claude-sonnet-4-6", "claude-sonnet-4-5"},
		{"claude-sonnet-4-6(high)", "claude-sonnet-4-5(high)"},
		{"claude-sonnet-4-5", ""},
		{"glm-5.2", "glm-5.1"},
		{"glm-5.1", "glm-5"},
		{"glm-5", ""},
		{"kimi-k2.6", "kimi-k2.5"},
		{"kimi-k2.6(8192)", "kimi-k2.5(8192)"},
		{"kimi-k2.7", "kimi-k2.6"},
		{"kimi-k2.7-code", "kimi-k2.6"},
		{"kimi-k2.7-code-highspeed", "kimi-k2.6"},
		{"kimi-k2.7(high)", "kimi-k2.6(high)"},
		{"kimi-k2.5", ""},
		{"gpt-5.5", "gpt-5.4"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := Next(tt.in, nil); got != tt.want {
				t.Fatalf("Next(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestChainMultiHop(t *testing.T) {
	const client = "modelversion-chain-test"
	registerTestModels(t, client, "openai", "glm-5", "glm-5.1", "glm-5.2")
	chain := Chain("glm-5.2", nil)
	want := []string{"glm-5.1", "glm-5"}
	if len(chain) != len(want) {
		t.Fatalf("Chain() = %v, want %v", chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("chain[%d] = %q, want %q", i, chain[i], want[i])
		}
	}
}

func TestNextRespectsProviderFilter(t *testing.T) {
	const clientA = "modelversion-provider-a"
	const clientB = "modelversion-provider-b"
	registerTestModels(t, clientA, "codex", "gpt-5.5")
	registerTestModels(t, clientB, "openai", "gpt-5.4")
	got := Next("gpt-5.5", []string{"codex"})
	if got != "" {
		t.Fatalf("Next with codex-only filter = %q, want empty", got)
	}
	got = Next("gpt-5.5", []string{"codex", "openai"})
	if got != "gpt-5.4" {
		t.Fatalf("Next with both providers = %q, want gpt-5.4", got)
	}
}
