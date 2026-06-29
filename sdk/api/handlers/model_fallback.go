package handlers

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

// modelVersionFallbackRule: first matching prefix wins; Next is one hop only.
type modelVersionFallbackRule struct {
	Prefix string // matched against thinking.ParseSuffix(model).ModelName via strings.HasPrefix
	Next   string // next base model id (no suffix); suffix re-applied by helper
}

var modelVersionFallbackRules = []modelVersionFallbackRule{
	{Prefix: "claude-opus-4-8", Next: "claude-opus-4-7"},
	{Prefix: "claude-opus-4-7", Next: "claude-opus-4-6"},
	{Prefix: "claude-sonnet-4-6", Next: "claude-sonnet-4-5"},
	{Prefix: "glm-5.2", Next: "glm-5.1"},
	{Prefix: "glm-5.1", Next: "glm-5"},
	{Prefix: "kimi-k2.6", Next: "kimi-k2.5"},
}

// maybeFallbackModel returns the next model to try when the requested model is unavailable.
// Thinking suffixes are preserved when present.
func maybeFallbackModel(model string) string {
	parsed := thinking.ParseSuffix(model)
	base := parsed.ModelName
	for _, rule := range modelVersionFallbackRules {
		if strings.HasPrefix(base, rule.Prefix) {
			return withThinkingSuffix(rule.Next, parsed)
		}
	}
	return ""
}

func withThinkingSuffix(model string, parsed thinking.SuffixResult) string {
	if parsed.HasSuffix {
		return model + "(" + parsed.RawSuffix + ")"
	}
	return model
}
