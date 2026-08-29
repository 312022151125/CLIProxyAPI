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

func TestMaskKiro(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "domain and bare token",
			in:   `{"text": "Visit kiro.dev with kiro and KIRO and Kiro"}`,
			want: `{"text": "Visit claude.ai with claude and CLAUDE and Claude"}`,
		},
		{
			name: "domain replaced before bare token",
			in:   "kiro.dev",
			want: "claude.ai",
		},
		{
			name: "kiro.dev must not become claude.dev",
			in:   "see kiro.dev now",
			want: "see claude.ai now",
		},
		{
			name: "mixed domain casing",
			in:   "Kiro.dev KIRO.DEV kiro.dev",
			want: "Claude.ai CLAUDE.AI claude.ai",
		},
		{
			name: "no kiro leaves input untouched",
			in:   `{"ok": true}`,
			want: `{"ok": true}`,
		},
		{
			name: "kiro as substring of other word replaced",
			in:   "kiro-kiro",
			want: "claude-claude",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskKiro(tt.in); got != tt.want {
				t.Errorf("maskKiro(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsClaudeModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-", true},
		{"claude-haiku-4-5", true},
		{"claude-sonnet-4-6", true},
		{"claude-opus-4-7", true},
		{"claude-3-5-sonnet-20241022", true},
		{"CLAUDE-3-7-SONNET", true},
		{"anthropic/claude-3.5-sonnet", true},
		{"gpt-4o", false},
		{"gpt-5.4", false},
		{"gemini-2.5-flash", false},
		{"deepseek-chat", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isClaudeModel(tt.model); got != tt.want {
			t.Errorf("isClaudeModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestKiroMaskingClaudeModelNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "1000")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by kiro.dev and kiro"}`)
	})

	reqBody := `{"model": "claude-haiku-4-5", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"content": "powered by claude.ai and claude"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	// Content-Length must be stripped for masked responses
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want stripped", got)
	}
}

func TestKiroMaskingNonClaudeModelUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "44")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by kiro.dev and kiro"}`)
	})

	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Non-Claude models must NOT be masked
	if got, want := rec.Body.String(), `{"content": "powered by kiro.dev and kiro"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestKiroMaskingStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.POST("/v1/messages", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Writer.WriteHeader(http.StatusOK)
		for _, chunk := range []string{
			`data: {"delta": "kiro.dev is kiro"}` + "\n\n",
			`data: [DONE]` + "\n\n",
		} {
			if _, err := c.Writer.WriteString(chunk); err != nil {
				t.Errorf("write chunk: %v", err)
			}
			c.Writer.Flush()
		}
	})

	reqBody := `{"model": "claude-sonnet-4-6", "stream": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	want := `data: {"delta": "claude.ai is claude"}` + "\n\n" + `data: [DONE]` + "\n\n"
	if got := string(body); got != want {
		t.Errorf("stream body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
}

func TestKiroMaskingPathOrQueryClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.GET("/v1/models/claude-3-5-sonnet", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.WriteString("kiro.dev host for kiro")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models/claude-3-5-sonnet", nil))

	if got, want := rec.Body.String(), "claude.ai host for claude"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
