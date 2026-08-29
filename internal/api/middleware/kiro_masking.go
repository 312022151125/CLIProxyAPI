package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// KiroMaskingMiddleware wraps responses for Claude models and replaces kiro.dev -> claude.ai
// and kiro -> claude in the body, so clients perceive the service purely as Claude.
// The replacement is applied to both streaming and non-streaming responses when the requested model is a Claude model.
func KiroMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldMaskRequest(c) {
			c.Writer = &kiroMaskingResponseWriter{ResponseWriter: c.Writer}
		}
		c.Next()
	}
}

// shouldMaskRequest checks whether the incoming request is targeting a Claude model.
func shouldMaskRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}

	// 1. Check query parameter `model`
	if queryModel := c.Query("model"); queryModel != "" {
		if isClaudeModel(queryModel) {
			return true
		}
	}

	// 2. Check URL path
	path := strings.ToLower(c.Request.URL.Path)
	if strings.Contains(path, "claude") {
		return true
	}

	// 3. Check JSON request body for `model` field
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			// Restore the body so downstream handlers and middleware can read it.
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			model := gjson.GetBytes(bodyBytes, "model").String()
			if isClaudeModel(model) {
				return true
			}
		}
	}

	return false
}

// isClaudeModel returns true if the model name indicates a Claude model (e.g. claude-, claude-haiku-4-5, etc.).
func isClaudeModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "claude")
}

// kiroMaskingResponseWriter intercepts Write/WriteString to transform the response
// body before it reaches the client. It also removes Content-Length because the
// transformations change byte length; Go's HTTP server then uses chunked encoding.
type kiroMaskingResponseWriter struct {
	gin.ResponseWriter
}

func (w *kiroMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKiro(string(data)))
}

func (w *kiroMaskingResponseWriter) WriteString(str string) (int, error) {
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKiro(str))
}

func (w *kiroMaskingResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

// maskKiro applies the replacements in order: the kiro.dev domain must be replaced
// before the bare "kiro" token so it never becomes claude.dev.
func maskKiro(s string) string {
	s = strings.ReplaceAll(s, "kiro.dev", "claude.ai")
	s = strings.ReplaceAll(s, "Kiro.dev", "Claude.ai")
	s = strings.ReplaceAll(s, "KIRO.DEV", "CLAUDE.AI")
	s = strings.ReplaceAll(s, "kiro", "claude")
	s = strings.ReplaceAll(s, "Kiro", "Claude")
	s = strings.ReplaceAll(s, "KIRO", "CLAUDE")
	return s
}
