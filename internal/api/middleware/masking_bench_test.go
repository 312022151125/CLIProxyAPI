package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// benchChunk is a clean SSE-sized chunk: it carries no masking token, so the
// masking writers must forward it without rewriting or allocating.
func benchChunk() []byte {
	chunk := make([]byte, 1024)
	for i := range chunk {
		chunk[i] = byte('a' + i%26)
	}
	return chunk
}

// benchWriteChunks writes 2000 chunks through write and reports allocations.
func benchWriteChunks(b *testing.B, write func([]byte) (int, error)) {
	b.Helper()
	chunk := benchChunk()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for j := range 2000 {
			if _, err := write(chunk); err != nil {
				b.Fatalf("write chunk %d: %v", j, err)
			}
		}
	}
}

func newBenchGinWriter() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	return c, rec
}

func BenchmarkBrandMaskingResponseWriterStreamChunk(b *testing.B) {
	c, _ := newBenchGinWriter()
	w := &brandMaskingResponseWriter{ResponseWriter: c.Writer}
	benchWriteChunks(b, w.Write)
}

func BenchmarkKiroMaskingResponseWriterStreamChunk(b *testing.B) {
	c, _ := newBenchGinWriter()
	w := &kiroMaskingResponseWriter{ResponseWriter: c.Writer, profile: claudeMaskProfile}
	benchWriteChunks(b, w.Write)
}

func BenchmarkAntigravityMaskingResponseWriterStreamChunk(b *testing.B) {
	c, _ := newBenchGinWriter()
	w := &antigravityMaskingResponseWriter{ResponseWriter: c.Writer}
	benchWriteChunks(b, w.Write)
}

// BenchmarkResponseWriterWrapperHeaderCapture measures the per-chunk header
// capture cost of the logging wrapper on a streaming response.
func BenchmarkResponseWriterWrapperHeaderCapture(b *testing.B) {
	c, _ := newBenchGinWriter()
	wrapper := NewResponseWriterWrapper(c.Writer, &testRequestLogger{enabled: true}, &RequestInfo{
		URL:     "/v1/chat/completions",
		Method:  http.MethodPost,
		Headers: map[string][]string{"Content-Type": {"application/json"}},
	})
	c.Writer = wrapper
	wrapper.WriteHeader(http.StatusOK)
	benchWriteChunks(b, wrapper.Write)
}
