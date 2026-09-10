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

func TestMaskKira(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "domain and Kira AI token",
			in:   `{"text": "Visit kiraai.vn with Kira AI and KIRA AI and kira ai"}`,
			want: `{"text": "Visit llmgate.app with Model AI and MODEL AI and model ai"}`,
		},
		{
			name: "domain replaced",
			in:   "kiraai.vn",
			want: "llmgate.app",
		},
		{
			name: "uppercase domain replaced",
			in:   "KIRAAI.VN",
			want: "LLMGATE.APP",
		},
		{
			name: "Kira AI replaced",
			in:   "Kira AI is here",
			want: "Model AI is here",
		},
		{
			name: "no kira leaves input untouched",
			in:   `{"ok": true}`,
			want: `{"ok": true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskKira(tt.in); got != tt.want {
				t.Errorf("maskKira(%q) = %q, want %q", tt.in, got, tt.want)
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
		{"gemini-pro", true},
		{"GEMINI-1.5-PRO", true},
		{"google/gemini-2.0", true},
		{"gpt-4o", false},
		{"claude-3-5-sonnet", false},
		{"deepseek-chat", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isGeminiModel(tt.model); got != tt.want {
			t.Errorf("isGeminiModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestKiraMaskingGeminiModelNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiraMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "1000")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by kiraai.vn and Kira AI"}`)
	})

	reqBody := `{"model": "gemini-2.5-flash", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"content": "powered by llmgate.app and Model AI"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	// Content-Length must be stripped for masked responses
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want stripped", got)
	}
}

func TestKiraMaskingNonGeminiModelUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiraMaskingMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", "50")
		c.Status(http.StatusOK)
		c.Writer.WriteString(`{"content": "powered by kiraai.vn and Kira AI"}`)
	})

	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Non-Gemini models must NOT be masked
	if got, want := rec.Body.String(), `{"content": "powered by kiraai.vn and Kira AI"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestKiraMaskingStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiraMaskingMiddleware())
	r.POST("/v1/messages", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Writer.WriteHeader(http.StatusOK)
		for _, chunk := range []string{
			`data: {"delta": "kiraai.vn is Kira AI"}` + "\n\n",
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
	want := `data: {"delta": "llmgate.app is Model AI"}` + "\n\n" + `data: [DONE]` + "\n\n"
	if got := string(body); got != want {
		t.Errorf("stream body = %q, want %q", got, want)
	}
}

func TestKiraMaskingPathGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KiraMaskingMiddleware())
	r.GET("/v1/models/gemini-2.5-flash", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.WriteString("kiraai.vn host for Kira AI")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models/gemini-2.5-flash", nil))

	if got, want := rec.Body.String(), "llmgate.app host for Model AI"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
