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
		name    string
		in      string
		profile maskProfile
		want    string
	}{
		{
			name:    "claude domain and bare token",
			in:      `{"text": "Visit kiro.dev with kiro and KIRO and Kiro"}`,
			profile: claudeMaskProfile,
			want:    `{"text": "Visit claude.ai with claude and CLAUDE and Claude"}`,
		},
		{
			name:    "openai domain and gpt token",
			in:      `{"text": "Visit kiro.dev with kiro and KIRO and Kiro"}`,
			profile: openAIMaskProfile,
			want:    `{"text": "Visit openai.com with gpt and GPT and Gpt"}`,
		},
		{
			name:    "claude domain replaced before bare token",
			in:      "kiro.dev",
			profile: claudeMaskProfile,
			want:    "claude.ai",
		},
		{
			name:    "kiro.dev must not become claude.dev",
			in:      "see kiro.dev now",
			profile: claudeMaskProfile,
			want:    "see claude.ai now",
		},
		{
			name:    "kiro.dev must not become gpt.dev",
			in:      "see kiro.dev now",
			profile: openAIMaskProfile,
			want:    "see openai.com now",
		},
		{
			name:    "mixed domain casing",
			in:      "Kiro.dev KIRO.DEV kiro.dev",
			profile: claudeMaskProfile,
			want:    "Claude.ai CLAUDE.AI claude.ai",
		},
		{
			name:    "mixed domain casing openai",
			in:      "Kiro.dev KIRO.DEV kiro.dev",
			profile: openAIMaskProfile,
			want:    "Openai.com OPENAI.COM openai.com",
		},
		{
			name:    "no kiro leaves input untouched",
			in:      `{"ok": true}`,
			profile: claudeMaskProfile,
			want:    `{"ok": true}`,
		},
		{
			name:    "kiro as substring of other word replaced",
			in:      "kiro-kiro",
			profile: claudeMaskProfile,
			want:    "claude-claude",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskKiro(tt.in, tt.profile); got != tt.want {
				t.Errorf("maskKiro(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestProfileForModel(t *testing.T) {
	tests := []struct {
		model      string
		wantMasked bool
		wantDomain string
		wantToken  string
	}{
		{"claude-", true, "claude.ai", "claude"},
		{"claude-haiku-4-5", true, "claude.ai", "claude"},
		{"claude-sonnet-4-6", true, "claude.ai", "claude"},
		{"claude-opus-4-7", true, "claude.ai", "claude"},
		{"claude-3-5-sonnet-20241022", true, "claude.ai", "claude"},
		{"CLAUDE-3-7-SONNET", true, "claude.ai", "claude"},
		{"anthropic/claude-3.5-sonnet", true, "claude.ai", "claude"},
		{"gpt-5.6", true, "openai.com", "gpt"},
		{"gpt-5.6-sol", true, "openai.com", "gpt"},
		{"gpt-5.6-luna", true, "openai.com", "gpt"},
		{"vendor/gpt-5.6-sol", true, "openai.com", "gpt"},
		{" GPT-5.6-SOL ", true, "openai.com", "gpt"},
		{"gpt-5.4", false, "", ""},
		{"gpt-5.60", false, "", ""},
		{"gpt-4o", false, "", ""},
		{"gemini-2.5-flash", false, "", ""},
		{"deepseek-chat", false, "", ""},
		{"", false, "", ""},
	}
	for _, tt := range tests {
		got, ok := profileForModel(tt.model)
		if ok != tt.wantMasked {
			t.Errorf("profileForModel(%q) masked = %v, want %v", tt.model, ok, tt.wantMasked)
			continue
		}
		if !ok {
			continue
		}
		if got.domain != tt.wantDomain || got.token != tt.wantToken {
			t.Errorf("profileForModel(%q) = %+v, want {%q %q}", tt.model, got, tt.wantDomain, tt.wantToken)
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
	// Non-masked models must NOT be masked
	if got, want := rec.Body.String(), `{"content": "powered by kiro.dev and kiro"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestKiroMaskingGPT56Model(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "1000")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by kiro.dev and kiro"}`)
	})

	reqBody := `{"model": "gpt-5.6-sol", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"content": "powered by openai.com and gpt"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want stripped", got)
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

func TestKiroMaskingStreamingGPT56(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.POST("/v1/responses", func(c *gin.Context) {
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

	reqBody := `{"model": "gpt-5.6-sol", "stream": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	want := `data: {"delta": "openai.com is gpt"}` + "\n\n" + `data: [DONE]` + "\n\n"
	if got := string(body); got != want {
		t.Errorf("stream body = %q, want %q", got, want)
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

func TestKiroMaskingPathGPT56(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiroMaskingMiddleware())
	r.GET("/v1/models/gpt-5.6-sol", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.WriteString("kiro.dev host for kiro")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models/gpt-5.6-sol", nil))

	if got, want := rec.Body.String(), "openai.com host for gpt"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
