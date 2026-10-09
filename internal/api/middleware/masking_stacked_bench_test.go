package middleware

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// benchmarkStacked wraps the writer chain a request actually gets today:
// brand always applies, plus exactly one profile writer.
func benchmarkStacked(b *testing.B, build func(gin.ResponseWriter) gin.ResponseWriter) {
	b.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := newBenchGinWriter()
	w := build(c.Writer)
	c.Writer = w
	benchWriteChunks(b, w.Write)
}

// BenchmarkStackedBrandKiro covers a claude-* request: brand + kiro.
func BenchmarkStackedBrandKiro(b *testing.B) {
	benchmarkStacked(b, func(inner gin.ResponseWriter) gin.ResponseWriter {
		return &brandMaskingResponseWriter{ResponseWriter: &kiroMaskingResponseWriter{ResponseWriter: inner, profile: claudeMaskProfile}}
	})
}

// BenchmarkStackedBrandAntigravity covers a gemini-* request: brand + antigravity.
func BenchmarkStackedBrandAntigravity(b *testing.B) {
	benchmarkStacked(b, func(inner gin.ResponseWriter) gin.ResponseWriter {
		return &brandMaskingResponseWriter{ResponseWriter: &antigravityMaskingResponseWriter{ResponseWriter: inner}}
	})
}

// BenchmarkStackedBrandOnly covers a request with no profile writer at all.
func BenchmarkStackedBrandOnly(b *testing.B) {
	benchmarkStacked(b, func(inner gin.ResponseWriter) gin.ResponseWriter {
		return &brandMaskingResponseWriter{ResponseWriter: inner}
	})
}
