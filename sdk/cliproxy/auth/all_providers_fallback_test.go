package auth

import (
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func boolPtr(b bool) *bool { return &b }

func TestFallbackToAllProvidersEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *internalconfig.Config
		expected bool
	}{
		{"nil config → default enabled", nil, true},
		{"nil field → default enabled", &internalconfig.Config{}, true},
		{"explicit true → enabled", &internalconfig.Config{FallbackToAllProviders: boolPtr(true)}, true},
		{"explicit false → disabled", &internalconfig.Config{FallbackToAllProviders: boolPtr(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fallbackToAllProvidersEnabled(tt.cfg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestShouldAttemptAllProvidersFallback(t *testing.T) {
	t.Run("nil manager returns false", func(t *testing.T) {
		var m *Manager
		if m.shouldAttemptAllProvidersFallback() {
			t.Error("nil manager should return false")
		}
	})
	t.Run("nil FallbackToAllProviders → default enabled", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		if !m.shouldAttemptAllProvidersFallback() {
			t.Error("should be enabled by default")
		}
	})
	t.Run("explicit false → disabled", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		m.runtimeConfig.Store(&internalconfig.Config{FallbackToAllProviders: boolPtr(false)})
		if m.shouldAttemptAllProvidersFallback() {
			t.Error("should be disabled")
		}
	})
	t.Run("explicit true → enabled", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		m.runtimeConfig.Store(&internalconfig.Config{FallbackToAllProviders: boolPtr(true)})
		if !m.shouldAttemptAllProvidersFallback() {
			t.Error("should be enabled")
		}
	})
}

func TestModelProvidersFallback(t *testing.T) {
	t.Run("no extra providers registered for model → current unchanged", func(t *testing.T) {
		current := []string{"gemini"}
		result := modelProvidersFallback(current, "unknown-model-xyz")
		if len(result) != len(current) {
			t.Errorf("expected len %d, got %d: %v", len(current), len(result), result)
		}
	})
	t.Run("current providers preserved at front", func(t *testing.T) {
		current := []string{"codex", "claude"}
		result := modelProvidersFallback(current, "some-model")
		if result[0] != "codex" || result[1] != "claude" {
			t.Errorf("first two must be codex,claude; got %v", result)
		}
	})
	t.Run("no duplicates", func(t *testing.T) {
		current := []string{"gemini", "codex"}
		result := modelProvidersFallback(current, "some-model")
		seen := make(map[string]int)
		for _, p := range result {
			seen[p]++
		}
		for p, count := range seen {
			if count > 1 {
				t.Errorf("provider %q appears %d times", p, count)
			}
		}
	})
}
