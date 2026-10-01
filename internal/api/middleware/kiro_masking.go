package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// maskProfile describes how kiro branding is rewritten for a given request.
type maskProfile struct {
	domain string // replacement for kiro.dev
	token  string // replacement for the bare "kiro" token
}

var (
	// claudeMaskProfile hides kiro behind Claude branding.
	claudeMaskProfile = maskProfile{domain: "claude.ai", token: "claude"}
	// openAIMaskProfile hides kiro behind OpenAI branding.
	openAIMaskProfile = maskProfile{domain: "openai.com", token: "gpt"}
)

// KiroMaskingMiddleware wraps responses for masked models and replaces kiro.dev
// and kiro in the body, so clients perceive the service as Claude or OpenAI
// depending on the requested model. The replacement is applied to both
// streaming and non-streaming responses.
func KiroMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if profile, ok := maskProfileForRequest(c); ok {
			c.Writer = &kiroMaskingResponseWriter{ResponseWriter: c.Writer, profile: profile}
		}
		c.Next()
	}
}

// maskProfileForRequest checks whether the incoming request targets a masked
// model and returns the branding profile to apply.
func maskProfileForRequest(c *gin.Context) (maskProfile, bool) {
	if c == nil || c.Request == nil {
		return maskProfile{}, false
	}

	// 1. Check query parameter `model`
	if queryModel := c.Query("model"); queryModel != "" {
		if profile, ok := profileForModel(queryModel); ok {
			return profile, true
		}
	}

	// 2. Check URL path
	path := strings.ToLower(c.Request.URL.Path)
	if strings.Contains(path, "claude") {
		return claudeMaskProfile, true
	}
	if matchesGPT56Model(path) {
		return openAIMaskProfile, true
	}

	// 3. Check JSON request body for `model` field
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			// Restore the body so downstream handlers and middleware can read it.
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			if profile, ok := profileForModel(gjson.GetBytes(bodyBytes, "model").String()); ok {
				return profile, true
			}
		}
	}

	return maskProfile{}, false
}

// profileForModel returns the branding profile for a masked model name:
// Claude models map to Claude, gpt-5.6-* models map to OpenAI.
func profileForModel(model string) (maskProfile, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return maskProfile{}, false
	}
	if strings.Contains(m, "claude") {
		return claudeMaskProfile, true
	}
	if matchesGPT56Model(m) {
		return openAIMaskProfile, true
	}
	return maskProfile{}, false
}

// matchesGPT56Model reports whether m names a gpt-5.6 model, ignoring any
// namespace or vendor segment (e.g. "gpt-5.6-sol", "vendor/gpt-5.6-sol").
func matchesGPT56Model(m string) bool {
	if idx := strings.LastIndex(m, "/"); idx >= 0 {
		m = m[idx+1:]
	}
	const prefix = "gpt-5.6"
	if !strings.HasPrefix(m, prefix) {
		return false
	}
	rest := m[len(prefix):]
	return rest == "" || rest[0] == '-' || rest[0] == '.'
}

// kiroMaskingResponseWriter intercepts Write/WriteString to transform the response
// body before it reaches the client. It also removes Content-Length because the
// transformations change byte length; Go's HTTP server then uses chunked encoding.
type kiroMaskingResponseWriter struct {
	gin.ResponseWriter
	profile maskProfile
}

func (w *kiroMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKiro(string(data), w.profile))
}

func (w *kiroMaskingResponseWriter) WriteString(str string) (int, error) {
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKiro(str, w.profile))
}

func (w *kiroMaskingResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

// maskKiro applies the replacements in order: the kiro.dev domain must be replaced
// before the bare "kiro" token so it never becomes <token>.dev.
func maskKiro(s string, profile maskProfile) string {
	s = strings.ReplaceAll(s, "kiro.dev", profile.domain)
	s = strings.ReplaceAll(s, "Kiro.dev", titleFirst(profile.domain))
	s = strings.ReplaceAll(s, "KIRO.DEV", strings.ToUpper(profile.domain))
	s = strings.ReplaceAll(s, "kiro", profile.token)
	s = strings.ReplaceAll(s, "Kiro", titleFirst(profile.token))
	s = strings.ReplaceAll(s, "KIRO", strings.ToUpper(profile.token))
	return s
}

// titleFirst upper-cases the first character, matching Kiro -> Claude casing.
func titleFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
