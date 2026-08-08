package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// openAICompat429KeyRotationEnabled reports whether runtime config allows
// exhausting every OpenAI-compatible API key before giving up on a 429.
func (m *Manager) openAICompat429KeyRotationEnabled() bool {
	if m == nil {
		return true
	}
	cfg, _ := m.runtimeConfig.Load().(*internalconfig.Config)
	return cfg == nil || cfg.OpenAICompat429KeyRotation == nil || *cfg.OpenAICompat429KeyRotation
}

// openAICompatProvidersWithUntriedAuth returns the subset of providers that still
// have an untried, enabled OpenAI-compatible API key credential available.
func (m *Manager) openAICompatProvidersWithUntriedAuth(providers []string, tried map[string]struct{}) []string {
	if m == nil || len(providers) == 0 {
		return nil
	}
	providerSet := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		providerKey := strings.TrimSpace(strings.ToLower(provider))
		if providerKey != "" {
			providerSet[providerKey] = struct{}{}
		}
	}
	if len(providerSet) == 0 {
		return nil
	}
	found := make(map[string]struct{}, len(providerSet))
	m.mu.RLock()
	for _, auth := range m.auths {
		if auth == nil || auth.Disabled || !isOpenAICompatAPIKeyAuth(auth) {
			continue
		}
		if _, used := tried[auth.ID]; used {
			continue
		}
		providerKey := executorKeyFromAuth(auth)
		if _, ok := providerSet[providerKey]; ok {
			found[providerKey] = struct{}{}
		}
	}
	m.mu.RUnlock()
	if len(found) == 0 {
		return nil
	}
	out := make([]string, 0, len(found))
	added := make(map[string]struct{}, len(found))
	for _, provider := range providers {
		providerKey := strings.TrimSpace(strings.ToLower(provider))
		if _, ok := found[providerKey]; ok {
			if _, exists := added[providerKey]; !exists {
				out = append(out, provider)
				added[providerKey] = struct{}{}
			}
		}
	}
	return out
}

// ResolveOpenAICompatColonEffortModel splits a "model:effort" request into its base
// model and reasoning effort when the OpenAI-compatible provider config declares that
// effort level for the model. It returns the original model untouched otherwise.
func ResolveOpenAICompatColonEffortModel(cfg *internalconfig.Config, auth *Auth, requestedModel string) (string, string, bool) {
	requestedModel = strings.TrimSpace(requestedModel)
	separator := strings.LastIndex(requestedModel, ":")
	if separator <= 0 || separator == len(requestedModel)-1 {
		return requestedModel, "", false
	}
	effort := strings.ToLower(strings.TrimSpace(requestedModel[separator+1:]))
	if !isOpenAICompatColonEffort(effort) {
		return requestedModel, "", false
	}
	providerKey := ""
	compatName := ""
	if auth != nil && auth.Attributes != nil {
		providerKey = strings.TrimSpace(auth.Attributes["provider_key"])
		compatName = strings.TrimSpace(auth.Attributes["compat_name"])
	}
	entry := resolveOpenAICompatConfigForAuth(cfg, auth, providerKey, compatName)
	if entry == nil {
		return requestedModel, "", false
	}
	baseModel := strings.TrimSpace(requestedModel[:separator])
	for i := range entry.Models {
		model := entry.Models[i]
		name := strings.TrimSpace(model.Name)
		alias := strings.TrimSpace(model.Alias)
		fullMatch := strings.EqualFold(name, requestedModel) || strings.EqualFold(alias, requestedModel)
		baseMatch := strings.EqualFold(name, baseModel) || strings.EqualFold(alias, baseModel)
		if !fullMatch && !baseMatch {
			continue
		}
		levels := []string{"low", "medium", "high"}
		if model.Thinking != nil {
			levels = model.Thinking.Levels
		}
		if thinking.HasLevel(levels, effort) {
			return baseModel, effort, true
		}
		if fullMatch {
			return requestedModel, "", false
		}
	}
	return requestedModel, "", false
}

func isOpenAICompatColonEffort(effort string) bool {
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

// mixedRetryPassExhaustedError marks an error as "every credential in this pass was
// tried", so the outer retry loop stops instead of starting another full pass.
type mixedRetryPassExhaustedError struct {
	cause error
}

func (e *mixedRetryPassExhaustedError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *mixedRetryPassExhaustedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func unwrapMixedRetryPassExhausted(err error) (error, bool) {
	var exhausted *mixedRetryPassExhaustedError
	if !errors.As(err, &exhausted) || exhausted == nil {
		return nil, false
	}
	return exhausted.cause, true
}

// runMixedRetry runs the full retry cycle for a non-streaming execution.
// It returns the first successful response, or the last error if all attempts fail.
func (m *Manager) runMixedRetry(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	_, maxRetryCredentials, maxWait := m.retrySettings()
	var lastErr error
	retryModel := authSelectionModelFromOptions(opts, req.Model)
	for attempt := 0; ; attempt++ {
		resp, errExec := m.executeMixedOnce(ctx, normalized, req, opts, maxRetryCredentials)
		if errExec == nil {
			return resp, nil
		}
		if exhaustedErr, ok := unwrapMixedRetryPassExhausted(errExec); ok {
			return cliproxyexecutor.Response{}, exhaustedErr
		}
		lastErr = errExec
		wait, shouldRetry := m.shouldRetryAfterError(errExec, attempt, normalized, retryModel, maxWait)
		if !shouldRetry {
			break
		}
		if errWait := waitForCooldown(ctx, wait, maxWait); errWait != nil {
			return cliproxyexecutor.Response{}, errWait
		}
	}
	if lastErr != nil {
		return cliproxyexecutor.Response{}, lastErr
	}
	return cliproxyexecutor.Response{}, &Error{Code: "auth_not_found", Message: "no auth available"}
}

// runStreamMixedRetry runs the full retry cycle for a streaming execution.
// It returns the first successful StreamResult, or the last error if all attempts fail.
func (m *Manager) runStreamMixedRetry(ctx context.Context, normalized []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	_, maxRetryCredentials, maxWait := m.retrySettings()
	var lastErr error
	retryModel := authSelectionModelFromOptions(opts, req.Model)
	for attempt := 0; ; attempt++ {
		result, errStream := m.executeStreamMixedOnce(ctx, normalized, req, opts, maxRetryCredentials)
		if errStream == nil {
			return result, nil
		}
		if exhaustedErr, ok := unwrapMixedRetryPassExhausted(errStream); ok {
			return nil, exhaustedErr
		}
		lastErr = errStream
		wait, shouldRetry := m.shouldRetryAfterError(errStream, attempt, normalized, retryModel, maxWait)
		if !shouldRetry {
			break
		}
		if errWait := waitForCooldown(ctx, wait, maxWait); errWait != nil {
			return nil, errWait
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
}

// isRetryableStatusCode reports whether a status justifies retrying on another credential.
func isRetryableStatusCode(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusPaymentRequired,
		http.StatusForbidden:
		return true
	}
	return false
}

// ModelSupportError reports whether the error indicates the model is unsupported.
func ModelSupportError(err error) bool {
	return isModelSupportError(err)
}

func isContextWindowExceededError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	tokens := [...]string{
		"context_too_large",
		"context_length_exceeded",
		"context window",
		"context length",
		"too many tokens",
	}
	for _, t := range tokens {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func isStaticFileNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, `"code":"not-found"`) && strings.Contains(msg, "failed to read static file")
}

func isStaticFileNotFoundResultError(err *Error) bool {
	if err == nil {
		return false
	}
	return isStaticFileNotFoundError(err)
}

// apiKeyPaymentExhaustedCooldown parks an API key credential effectively forever once
// the provider reports the balance is exhausted, since it will not recover on its own.
const apiKeyPaymentExhaustedCooldown = 10 * 365 * 24 * time.Hour

func shouldApplyAPIKeyPaymentExhaustedCooldown(auth *Auth, statusCode int) bool {
	if auth == nil || statusCode != http.StatusPaymentRequired {
		return false
	}
	return isAPIKeyAuth(auth)
}

func applyAPIKeyPaymentExhaustedCooldown(auth *Auth, state *ModelState, resultErr *Error, now time.Time) {
	if auth == nil {
		return
	}
	next := now.Add(apiKeyPaymentExhaustedCooldown)
	message := "payment_required"
	if resultErr != nil && strings.TrimSpace(resultErr.Message) != "" {
		message = strings.TrimSpace(resultErr.Message)
	}
	quota := QuotaState{
		Exceeded:      true,
		Reason:        "payment_required",
		NextRecoverAt: next,
	}
	auth.Unavailable = true
	auth.Status = StatusError
	auth.NextRetryAfter = next
	auth.StatusMessage = message
	auth.Quota = quota
	auth.UpdatedAt = now
	if resultErr != nil {
		auth.LastError = cloneError(resultErr)
	}
	if state != nil {
		state.Unavailable = true
		state.Status = StatusError
		state.NextRetryAfter = next
		state.StatusMessage = message
		state.Quota = quota
		state.UpdatedAt = now
		if resultErr != nil {
			state.LastError = cloneError(resultErr)
		}
	}
}
