package handlers

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

// maybeFallbackModel returns the next model to try when the requested model is unavailable.
// Thinking suffixes are preserved when present.
func maybeFallbackModel(model string) string {
	if fb := maybeFallbackClaudeOpusModel(model); fb != "" {
		return fb
	}
	return maybeFallbackGLMModel(model)
}

// maybeFallbackClaudeOpusModel degrades claude-opus-4-8 -> claude-opus-4-7 -> claude-opus-4-6 when unavailable.
func maybeFallbackClaudeOpusModel(model string) string {
	parsed := thinking.ParseSuffix(model)
	base := parsed.ModelName
	var next string
	switch {
	case strings.HasPrefix(base, "claude-opus-4-8"):
		next = "claude-opus-4-7"
	case strings.HasPrefix(base, "claude-opus-4-7"):
		next = "claude-opus-4-6"
	default:
		return ""
	}
	return withThinkingSuffix(next, parsed)
}

// maybeFallbackGLMModel degrades glm-5.2 -> glm-5.1 -> glm-5 when unavailable.
func maybeFallbackGLMModel(model string) string {
	parsed := thinking.ParseSuffix(model)
	base := parsed.ModelName
	var next string
	switch {
	case strings.HasPrefix(base, "glm-5.2"):
		next = "glm-5.1"
	case strings.HasPrefix(base, "glm-5.1"):
		next = "glm-5"
	default:
		return ""
	}
	return withThinkingSuffix(next, parsed)
}

func withThinkingSuffix(model string, parsed thinking.SuffixResult) string {
	if parsed.HasSuffix {
		return model + "(" + parsed.RawSuffix + ")"
	}
	return model
}
