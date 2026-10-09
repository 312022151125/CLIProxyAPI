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
	contentLengthStripped bool
}

func (w *brandMaskingResponseWriter) Write(data []byte) (int, error) {
	// Strip Content-Length before the underlying writer emits headers implicitly.
	w.stripContentLength()
	return w.ResponseWriter.Write(maskBrand(data))
}

func (w *brandMaskingResponseWriter) WriteString(str string) (int, error) {
	w.stripContentLength()
	if !containsFoldString(str, "kira") && !containsFoldString(str, "opencode") {
		return w.ResponseWriter.WriteString(str)
	}
	return w.ResponseWriter.Write(maskBrand([]byte(str)))
}

func (w *brandMaskingResponseWriter) WriteHeader(code int) {
	w.stripContentLength()
	w.ResponseWriter.WriteHeader(code)
}

// stripContentLength removes Content-Length once. It must run on every entry
// point that can emit headers, because a later chunk may still rewrite the body.
func (w *brandMaskingResponseWriter) stripContentLength() {
	if w.contentLengthStripped {
		return
	}
	w.Header().Del("Content-Length")
	w.contentLengthStripped = true
}

// maskBrand applies the replacements in order: the kiraai.vn domain must be replaced
// before the bare "Kira AI" token. Upstream error payloads pass through the response
// writer unchanged, so branding strings they carry (e.g. opencode2api) are rewritten here.
// Every spelling below contains "kira" or "opencode" case-insensitively, so a chunk
// carrying neither root needle cannot match and is returned unchanged.
func maskBrand(b []byte) []byte {
	if !containsFoldASCII(b, "kira") && !containsFoldASCII(b, "opencode") {
		return b
	}
	s := string(b)
	s = strings.ReplaceAll(s, "kiraai.vn", "llmgate.app")
	s = strings.ReplaceAll(s, "KIRAAI.VN", "LLMGATE.APP")
	s = strings.ReplaceAll(s, "opencode2api", "llmgate.app")
	s = strings.ReplaceAll(s, "OPENCODE2API", "LLMGATE.APP")
	s = strings.ReplaceAll(s, "Kira AI", "Model AI")
	s = strings.ReplaceAll(s, "kira ai", "model ai")
	s = strings.ReplaceAll(s, "KIRA AI", "MODEL AI")
	return []byte(s)
}
