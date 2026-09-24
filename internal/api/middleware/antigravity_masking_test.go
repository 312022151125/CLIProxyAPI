package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMaskAntigravity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "domain and bare token",
			in:   `{"text": "Visit antigravity.dev with antigravity and ANTIGRAVITY and Antigravity"}`,
			want: `{"text": "Visit gemini.dev with gemini and GEMINI and Gemini"}`,
		},
		{
			name: "mixed casing",
			in:   "antigravity Antigravity ANTIGRAVITY",
			want: "gemini Gemini GEMINI",
		},
		{
			name: "no antigravity leaves input untouched",
			in:   `{"ok": true}`,
			want: `{"ok": true}`,
		},
		{
			name: "antigravity as substring replaced",
			in:   "antigravity-antigravity",
			want: "gemini-gemini",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskAntigravity(tt.in); got != tt.want {
				t.Errorf("maskAntigravity(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsGeminiModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gemini-2.5-flash", true},
		{"gemini-2.5-pro", true},
		{"GEMINI-PRO", true},
		{"antigravity/gemini-2.5-pro", true},
		{"claude-sonnet-4-6", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isGeminiModel(tt.model); got != tt.want {
			t.Errorf("isGeminiModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestAntigravityMaskingGeminiModelNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AntigravityMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "1000")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by antigravity and ANTIGRAVITY"}`)
	})

	reqBody := `{"model": "gemini-2.5-pro", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"content": "powered by gemini and GEMINI"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want stripped", got)
	}
}

func TestAntigravityMaskingNonGeminiModelUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AntigravityMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "44")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by antigravity"}`)
	})

	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"content": "powered by antigravity"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestAntigravityMaskingStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AntigravityMaskingMiddleware())
	r.POST("/v1/messages", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Writer.WriteHeader(http.StatusOK)
		for _, chunk := range []string{
			`data: {"delta": "antigravity is real"}` + "\n\n",
			`data: [DONE]` + "\n\n",
		} {
			if _, err := c.Writer.WriteString(chunk); err != nil {
				t.Errorf("write chunk: %v", err)
			}
			c.Writer.Flush()
		}
	})

	reqBody := `{"model": "gemini-2.5-flash", "stream": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	want := `data: {"delta": "gemini is real"}` + "\n\n" + `data: [DONE]` + "\n\n"
	if got := string(body); got != want {
		t.Errorf("stream body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
}
