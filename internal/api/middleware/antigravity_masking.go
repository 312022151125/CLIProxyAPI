package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// AntigravityMaskingMiddleware wraps responses for Gemini models and replaces
// antigravity -> gemini in the body, so clients perceive the service as Gemini.
// The replacement is applied to both streaming and non-streaming responses when
// the requested model is a Gemini model.
func AntigravityMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldMaskAntigravityRequest(c) {
			c.Writer = &antigravityMaskingResponseWriter{ResponseWriter: c.Writer}
		}
		c.Next()
	}
}

// shouldMaskAntigravityRequest checks whether the incoming request is targeting a Gemini model.
func shouldMaskAntigravityRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}

	// 1. Check query parameter `model`
	if queryModel := c.Query("model"); queryModel != "" {
		if isGeminiModel(queryModel) {
			return true
		}
	}

	// 2. Check URL path
	path := strings.ToLower(c.Request.URL.Path)
	if strings.Contains(path, "gemini") {
		return true
	}

	// 3. Check JSON request body for `model` field
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			// Restore the body so downstream handlers and middleware can read it.
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			model := gjson.GetBytes(bodyBytes, "model").String()
			if isGeminiModel(model) {
				return true
			}
		}
	}

	return false
}

// isGeminiModel returns true if the model name indicates a Gemini model.
func isGeminiModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "gemini")
}

// antigravityMaskingResponseWriter intercepts Write/WriteString to transform the
// response body before it reaches the client. It also removes Content-Length
// because the transformations change byte length; Go's HTTP server then uses
// chunked encoding.
type antigravityMaskingResponseWriter struct {
	gin.ResponseWriter
}

func (w *antigravityMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskAntigravity(string(data)))
}

func (w *antigravityMaskingResponseWriter) WriteString(str string) (int, error) {
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskAntigravity(str))
}

func (w *antigravityMaskingResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

// maskAntigravity replaces every casing of "antigravity" with "gemini".
func maskAntigravity(s string) string {
	s = strings.ReplaceAll(s, "antigravity", "gemini")
	s = strings.ReplaceAll(s, "Antigravity", "Gemini")
	s = strings.ReplaceAll(s, "ANTIGRAVITY", "GEMINI")
	return s
}
