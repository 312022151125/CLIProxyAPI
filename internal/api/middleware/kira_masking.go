package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// KiraMaskingMiddleware wraps responses for Gemini models and replaces kiraai.vn -> llmgate.app
// and Kira AI -> Model AI in the body, so clients perceive the service without Kira branding.
// The replacement is applied to both streaming and non-streaming responses when the requested model is a Gemini model.
func KiraMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldMaskKiraRequest(c) {
			c.Writer = &kiraMaskingResponseWriter{ResponseWriter: c.Writer}
		}
		c.Next()
	}
}

// shouldMaskKiraRequest checks whether the incoming request is targeting a Gemini model.
func shouldMaskKiraRequest(c *gin.Context) bool {
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

// isGeminiModel returns true if the model name indicates a Gemini model (e.g. gemini-2.5-flash, etc.).
func isGeminiModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "gemini")
}

// kiraMaskingResponseWriter intercepts Write/WriteString to transform the response
// body before it reaches the client. It also removes Content-Length because the
// transformations change byte length; Go's HTTP server then uses chunked encoding.
type kiraMaskingResponseWriter struct {
	gin.ResponseWriter
}

func (w *kiraMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKira(string(data)))
}

func (w *kiraMaskingResponseWriter) WriteString(str string) (int, error) {
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskKira(str))
}

func (w *kiraMaskingResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

// maskKira applies the replacements in order: the kiraai.vn domain must be replaced
// before the bare "Kira AI" token.
func maskKira(s string) string {
	s = strings.ReplaceAll(s, "kiraai.vn", "llmgate.app")
	s = strings.ReplaceAll(s, "KIRAAI.VN", "LLMGATE.APP")
	s = strings.ReplaceAll(s, "Kira AI", "Model AI")
	s = strings.ReplaceAll(s, "kira ai", "model ai")
	s = strings.ReplaceAll(s, "KIRA AI", "MODEL AI")
	return s
}
