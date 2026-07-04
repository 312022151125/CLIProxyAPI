package auth

import (
	"context"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/modelversion"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// hasCodexProvider reports whether any of the given providers is the Codex OAuth provider.
func hasCodexProvider(providers []string) bool {
	for _, p := range providers {
		if strings.EqualFold(strings.TrimSpace(p), "codex") {
			return true
		}
	}
	return false
}

// buildCodexFallbackRequest constructs a fallback request substituting the fallback model
// and recording the original model name for transparent response rewriting.
// Returns the fallback request, fallback opts, and ok=true if a registry-backed downgrade exists.
func buildCodexFallbackRequest(req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Request, cliproxyexecutor.Options, bool) {
	parsed := thinking.ParseSuffix(req.Model)
	fbModel := modelversion.Next(req.Model, []string{"codex"})
	if fbModel == "" {
		return req, opts, false
	}

	fbReq := req
	fbReq.Model = fbModel

	fbMeta := make(map[string]any, len(opts.Metadata)+2)
	for k, v := range opts.Metadata {
		fbMeta[k] = v
	}
	fbMeta[cliproxyexecutor.CodexFallbackDisplayModelMetadataKey] = parsed.ModelName
	fbMeta[cliproxyexecutor.RequestedModelMetadataKey] = thinking.ParseSuffix(fbModel).ModelName

	fbOpts := opts
	fbOpts.Metadata = fbMeta

	return fbReq, fbOpts, true
}

// tryCodexModelFallbackExecute attempts a non-streaming execution with the Codex model fallback.
// It runs the full retry cycle for the fallback model.
// Returns (response, true) on success, (zero, false) if no fallback applies or all attempts fail.
func (m *Manager) tryCodexModelFallbackExecute(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, bool) {
	if !hasCodexProvider(normalized) {
		return cliproxyexecutor.Response{}, false
	}
	fbReq, fbOpts, ok := buildCodexFallbackRequest(req, opts)
	if !ok {
		return cliproxyexecutor.Response{}, false
	}
	resp, err := m.runMixedRetry(ctx, normalized, fbReq, fbOpts)
	if err != nil {
		return cliproxyexecutor.Response{}, false
	}
	return resp, true
}

// tryCodexModelFallbackExecuteStream attempts a streaming execution with the Codex model fallback.
// It runs the full retry cycle for the fallback model.
// Returns (result, true) on success, (nil, false) if no fallback applies or all attempts fail.
func (m *Manager) tryCodexModelFallbackExecuteStream(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, bool) {
	if !hasCodexProvider(normalized) {
		return nil, false
	}
	fbReq, fbOpts, ok := buildCodexFallbackRequest(req, opts)
	if !ok {
		return nil, false
	}
	result, err := m.runStreamMixedRetry(ctx, normalized, fbReq, fbOpts)
	if err != nil {
		return nil, false
	}
	return result, true
}
