package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// BrandMaskingMiddleware wraps all responses and replaces upstream vendor
// branding (kiraai.vn, opencode2api) with llmgate.app and Kira AI with Model AI
// in the body, so clients perceive the service without third-party branding.
// The replacement is applied to all responses regardless of model.
func BrandMaskingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &brandMaskingResponseWriter{ResponseWriter: c.Writer}
		c.Next()
	}
}

// brandMaskingResponseWriter intercepts Write/WriteString to transform the response
// body before it reaches the client. It also removes Content-Length because the
// transformations change byte length; Go's HTTP server then uses chunked encoding.
type brandMaskingResponseWriter struct {
	gin.ResponseWriter
}

func (w *brandMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskBrand(string(data)))
}

func (w *brandMaskingResponseWriter) WriteString(str string) (int, error) {
	w.Header().Del("Content-Length")
	return w.ResponseWriter.WriteString(maskBrand(str))
}

func (w *brandMaskingResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

// maskBrand applies the replacements in order: the kiraai.vn domain must be replaced
// before the bare "Kira AI" token. Upstream error payloads pass through the response
// writer unchanged, so branding strings they carry (e.g. opencode2api) are rewritten here.
func maskBrand(s string) string {
	s = strings.ReplaceAll(s, "kiraai.vn", "llmgate.app")
	s = strings.ReplaceAll(s, "KIRAAI.VN", "LLMGATE.APP")
	s = strings.ReplaceAll(s, "opencode2api", "llmgate.app")
	s = strings.ReplaceAll(s, "OPENCODE2API", "LLMGATE.APP")
	s = strings.ReplaceAll(s, "Kira AI", "Model AI")
	s = strings.ReplaceAll(s, "kira ai", "model ai")
	s = strings.ReplaceAll(s, "KIRA AI", "MODEL AI")
	return s
}
