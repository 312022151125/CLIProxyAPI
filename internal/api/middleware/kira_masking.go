package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// KiraMaskingMiddleware wraps all responses and replaces kiraai.vn -> llmgate.app
// and Kira AI -> Model AI in the body, so clients perceive the service without Kira branding.
// The replacement is applied to all responses regardless of model.
func KiraMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &kiraMaskingResponseWriter{ResponseWriter: c.Writer}
		c.Next()
	}
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
