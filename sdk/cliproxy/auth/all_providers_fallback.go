package auth

import (
	"context"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// shouldAttemptAllProvidersFallback reports whether the conductor should retry
// the request across all providers that support the model after the primary set fails.
// Cross-provider fallback is always enabled.
func (m *Manager) shouldAttemptAllProvidersFallback() bool {
	return m != nil
}

// modelProvidersFallback returns every provider registered for the given model
// that is not already in current, appended after current so priority order is
// preserved. Backup (priority=-1) credentials are handled last by the existing
// hasUntriedBackupAuth guard inside executeMixedOnce.
func modelProvidersFallback(current []string, model string) []string {
	currentSet := make(map[string]struct{}, len(current))
	for _, p := range current {
		key := strings.ToLower(strings.TrimSpace(p))
		if key != "" {
			currentSet[key] = struct{}{}
		}
	}
	// GetModelProviders returns only providers that have the model registered,
	// sorted by credential count descending — most-covered providers come first.
	registered := registry.GetGlobalRegistry().GetModelProviders(model)
	extra := make([]string, 0, len(registered))
	for _, p := range registered {
		key := strings.ToLower(strings.TrimSpace(p))
		if key == "" {
			continue
		}
		if _, already := currentSet[key]; already {
			continue
		}
		extra = append(extra, p)
	}
	if len(extra) == 0 {
		return current
	}
	merged := make([]string, 0, len(current)+len(extra))
	merged = append(merged, current...)
	merged = append(merged, extra...)
	return merged
}

// tryAllProvidersFallbackExecute retries a non-streaming request across every
// provider that has the model registered, when the primary set is exhausted.
func (m *Manager) tryAllProvidersFallbackExecute(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, bool) {
	expanded := modelProvidersFallback(normalized, strings.TrimSpace(req.Model))
	if len(expanded) == len(normalized) {
		return cliproxyexecutor.Response{}, false
	}
	resp, err := m.runMixedRetry(ctx, expanded, req, opts)
	if err != nil {
		return cliproxyexecutor.Response{}, false
	}
	return resp, true
}

// tryAllProvidersFallbackExecuteStream retries a streaming request across every
// provider that has the model registered, when the primary set is exhausted.
func (m *Manager) tryAllProvidersFallbackExecuteStream(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, bool) {
	expanded := modelProvidersFallback(normalized, strings.TrimSpace(req.Model))
	if len(expanded) == len(normalized) {
		return nil, false
	}
	result, err := m.runStreamMixedRetry(ctx, expanded, req, opts)
	if err != nil {
		return nil, false
	}
	return result, true
}
