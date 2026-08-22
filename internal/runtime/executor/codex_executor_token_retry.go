package executor

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"
)

// refreshCodexAuthForTokenInvalidatedRetry refreshes a Codex credential before a
// token_invalidated retry. It is a variable so tests can stub the refresh.
var refreshCodexAuthForTokenInvalidatedRetry = func(ctx context.Context, e *CodexExecutor, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	return e.Refresh(ctx, auth)
}

// isCodexTokenInvalidatedResponse reports whether an upstream 401 means the access
// token was invalidated server-side, which a refresh can recover from.
func isCodexTokenInvalidatedResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusUnauthorized {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "token_invalidated") || strings.Contains(lower, "authentication token has been invalidated")
}

// retryAfterCodexTokenInvalidated refreshes the credential and replays the request
// once after an upstream token_invalidated 401. It closes current before issuing the
// replay, and reports retried=false when the refresh did not yield a usable auth.
func (e *CodexExecutor) retryAfterCodexTokenInvalidated(
	ctx context.Context,
	auth *cliproxyauth.Auth,
	from sdktranslator.Format,
	pathSuffix string,
	req cliproxyexecutor.Request,
	originalPayloadSource []byte,
	body []byte,
	httpClient *http.Client,
	current *http.Response,
) (*http.Response, *cliproxyauth.Auth, codexIdentityConfuseState, bool, error) {
	var identityState codexIdentityConfuseState
	refreshedAuth, refreshErr := refreshCodexAuthForTokenInvalidatedRetry(ctx, e, auth)
	if refreshErr != nil {
		helps.LogWithRequestID(ctx).Debugf("codex token_invalidated refresh retry failed: %v", refreshErr)
		return nil, auth, identityState, false, nil
	}
	if refreshedAuth == nil {
		return nil, auth, identityState, false, nil
	}
	auth = refreshedAuth
	apiKey, baseURL := codexCreds(auth)
	if baseURL == "" {
		baseURL = "https://chatgpt.com/backend-api/codex"
	}
	url := strings.TrimSuffix(baseURL, "/") + pathSuffix
	retryReq, retryUpstreamBody, retryIdentityState, retryReqErr := e.cacheHelper(ctx, from, url, auth, req, originalPayloadSource, body)
	if retryReqErr != nil {
		return nil, auth, identityState, false, retryReqErr
	}
	identityState = retryIdentityState
	applyCodexHeaders(retryReq, auth, apiKey, true, e.cfg)
	applyCodexIdentityConfuseHeaders(retryReq.Header, &identityState)
	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   retryReq.Header.Clone(),
		Body:      retryUpstreamBody,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})
	if current != nil {
		if errClose := current.Body.Close(); errClose != nil {
			log.Errorf("codex executor: close response body error: %v", errClose)
		}
	}
	retryResp, errDo := httpClient.Do(retryReq)
	if errDo != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, errDo)
		return nil, auth, identityState, false, errDo
	}
	helps.RecordAPIResponseMetadata(ctx, e.cfg, retryResp.StatusCode, retryResp.Header.Clone())
	return retryResp, auth, identityState, true, nil
}

// applyCodexFastServiceTier applies fast-service-tier policy at the very last
// moment before sending, so it survives all prior payload processing.
//
// When fast-service-tier is enabled, service_tier is forced to "priority".
// When fast-service-tier is disabled, any service_tier the caller sent is
// stripped so that fast-mode requests never reach the upstream.
func applyCodexFastServiceTier(cfg *config.Config, body []byte) []byte {
	if cfg == nil || len(body) == 0 {
		return body
	}
	if cfg.FastServiceTier {
		body, _ = sjson.SetBytes(body, "service_tier", "priority")
		return body
	}
	// fast-service-tier=false: remove any service_tier the caller supplied.
	body, _ = sjson.DeleteBytes(body, "service_tier")
	return body
}
