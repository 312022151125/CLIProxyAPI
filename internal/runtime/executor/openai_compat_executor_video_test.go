package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// videoOpts returns an Options with SourceFormat=openai-video and an optional alt tag.
func videoOpts(alt string) cliproxyexecutor.Options {
	return cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
		Alt:          alt,
	}
}

// TestOpenAICompatIsVideoRequest checks that only openai-video source format is detected.
func TestOpenAICompatIsVideoRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		format string
		want   bool
	}{
		{openAICompatVideoHandlerType, true},
		{"openai", false},
		{"openai-image", false},
		{"", false},
	}
	for _, tc := range cases {
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString(tc.format)}
		if got := openAICompatIsVideoRequest(opts); got != tc.want {
			t.Errorf("openAICompatIsVideoRequest(%q) = %v, want %v", tc.format, got, tc.want)
		}
	}
}

// TestOpenAICompatVideoMethodAndEndpoint checks every alt mapping.
func TestOpenAICompatVideoMethodAndEndpoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		alt        string
		payload    []byte
		wantMethod string
		wantEp     string
	}{
		{
			name:       "list",
			alt:        "videos/list",
			wantMethod: http.MethodGet,
			wantEp:     "/videos",
		},
		{
			name:       "delete with id",
			alt:        "videos/delete",
			payload:    []byte(`{"request_id":"vid-abc"}`),
			wantMethod: http.MethodDelete,
			wantEp:     "/videos/vid-abc",
		},
		{
			name:       "delete without id",
			alt:        "videos/delete",
			wantMethod: http.MethodDelete,
			wantEp:     "/videos",
		},
		{
			name:       "remix with id",
			alt:        "videos/remix",
			payload:    []byte(`{"request_id":"vid-xyz"}`),
			wantMethod: http.MethodPost,
			wantEp:     "/videos/vid-xyz/remix",
		},
		{
			name:       "remix without id",
			alt:        "videos/remix",
			wantMethod: http.MethodPost,
			wantEp:     "/videos/generations",
		},
		{
			name:       "characters get with id",
			alt:        "videos/characters/get",
			payload:    []byte(`{"character_id":"char-1"}`),
			wantMethod: http.MethodGet,
			wantEp:     "/videos/characters/char-1",
		},
		{
			name:       "characters get without id",
			alt:        "videos/characters/get",
			wantMethod: http.MethodGet,
			wantEp:     "/videos/characters",
		},
		{
			name:       "characters create",
			alt:        "videos/characters/create",
			wantMethod: http.MethodPost,
			wantEp:     "/videos/characters",
		},
		{
			name:       "default create (empty alt)",
			alt:        "",
			wantMethod: http.MethodPost,
			wantEp:     "/videos/generations",
		},
		{
			name:       "path suffix generations",
			alt:        "",
			wantMethod: http.MethodPost,
			wantEp:     "/videos/generations",
		},
		{
			name:       "path suffix edits",
			alt:        "other",
			wantMethod: http.MethodPost,
			wantEp:     "/videos/generations", // no matching suffix → default
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := videoOpts(tc.alt)
			method, ep := openAICompatVideoMethodAndEndpoint(opts, tc.payload)
			if method != tc.wantMethod {
				t.Errorf("method = %q, want %q", method, tc.wantMethod)
			}
			if ep != tc.wantEp {
				t.Errorf("endpoint = %q, want %q", ep, tc.wantEp)
			}
		})
	}
}

// TestOpenAICompatVideoMethodAndEndpointPathSuffix verifies that the request-path metadata
// is used to derive the endpoint when no recognized alt is set.
func TestOpenAICompatVideoMethodAndEndpointPathSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		requestPath string
		wantEp      string
	}{
		{"/v1/videos/generations", "/videos/generations"},
		{"/v1/videos/edits", "/videos/edits"},
		{"/v1/videos/extensions", "/videos/extensions"},
		{"/v1/videos", "/videos"},
		{"/something/else", "/videos/generations"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.requestPath, func(t *testing.T) {
			t.Parallel()
			opts := cliproxyexecutor.Options{
				SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
				Alt:          "",
				Metadata: map[string]any{
					cliproxyexecutor.RequestPathMetadataKey: tc.requestPath,
				},
			}
			_, ep := openAICompatVideoMethodAndEndpoint(opts, nil)
			if ep != tc.wantEp {
				t.Errorf("path=%q: endpoint = %q, want %q", tc.requestPath, ep, tc.wantEp)
			}
		})
	}
}

// TestOpenAICompatExecutorVideoList verifies GET /videos is forwarded correctly.
func TestOpenAICompatExecutorVideoList(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"object":"list"}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "videos/list",
			Query:        url.Values{"limit": []string{"5"}, "order": []string{"desc"}},
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/videos" {
		t.Errorf("path = %q, want /v1/videos", gotPath)
	}
	if gotQuery.Get("limit") != "5" {
		t.Errorf("query limit = %q, want 5", gotQuery.Get("limit"))
	}
}

// TestOpenAICompatExecutorVideoDelete verifies DELETE /videos/:id is forwarded correctly.
func TestOpenAICompatExecutorVideoDelete(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deleted":true}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"request_id":"vid-123"}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: payload},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "videos/delete",
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v1/videos/vid-123" {
		t.Errorf("path = %q, want /v1/videos/vid-123", gotPath)
	}
	// DELETE should send no body
	if len(gotBody) != 0 {
		t.Errorf("body = %q, want empty", string(gotBody))
	}
}

// TestOpenAICompatExecutorVideoRemix verifies POST /videos/:id/remix is forwarded with body.
func TestOpenAICompatExecutorVideoRemix(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"remix-1","status":"queued"}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"request_id":"vid-456","prompt":"loop it"}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: payload},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "videos/remix",
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/videos/vid-456/remix" {
		t.Errorf("path = %q, want /v1/videos/vid-456/remix", gotPath)
	}
	if string(gotBody) != string(payload) {
		t.Errorf("body = %s, want %s", string(gotBody), string(payload))
	}
}

// TestOpenAICompatExecutorVideoCreateCharacter verifies POST /videos/characters.
func TestOpenAICompatExecutorVideoCreateCharacter(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"char-new"}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"video_id":"vid-789","name":"hero"}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: payload},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "videos/characters/create",
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/videos/characters" {
		t.Errorf("path = %q, want /v1/videos/characters", gotPath)
	}
}

// TestOpenAICompatExecutorVideoGetCharacter verifies GET /videos/characters/:id.
func TestOpenAICompatExecutorVideoGetCharacter(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"char-42","name":"hero"}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"character_id":"char-42"}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: payload},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "videos/characters/get",
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/videos/characters/char-42" {
		t.Errorf("path = %q, want /v1/videos/characters/char-42", gotPath)
	}
}

// TestOpenAICompatExecutorVideoCreate verifies the default POST /videos/generations create path.
func TestOpenAICompatExecutorVideoCreate(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"req-gen-1","status":"queued"}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"model":"sora-2","prompt":"a sunset"}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: payload},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "",
		})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/videos/generations" {
		t.Errorf("path = %q, want /v1/videos/generations", gotPath)
	}
	if string(gotBody) != string(payload) {
		t.Errorf("body = %s, want %s", string(gotBody), string(payload))
	}
}

// TestOpenAICompatExecutorVideoUpstreamError verifies that a 4xx from the upstream is
// propagated as an error rather than silently swallowed.
func TestOpenAICompatExecutorVideoUpstreamError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"message":"prompt is required"}}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{Payload: []byte(`{}`)},
		cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString(openAICompatVideoHandlerType),
			Alt:          "",
		})
	if err == nil {
		t.Fatal("expected error for 422 upstream, got nil")
	}
}

// TestOpenAICompatImageEndpointPathVariations verifies that /images/variations
// is correctly mapped to the upstream endpoint path.
func TestOpenAICompatImageEndpointPathVariations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		requestPath string
		wantEp      string
	}{
		{"/v1/images/generations", openAICompatImagesGenerationsPath},
		{"/v1/images/edits", openAICompatImagesEditsPath},
		{"/v1/images/variations", openAICompatImagesVariationsPath},
		// unknown path falls back to default (generations)
		{"/v1/images/unknown", openAICompatDefaultImageEndpoint},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.requestPath, func(t *testing.T) {
			t.Parallel()
			opts := cliproxyexecutor.Options{
				SourceFormat: sdktranslator.FromString(openAICompatImageHandlerType),
				Metadata: map[string]any{
					cliproxyexecutor.RequestPathMetadataKey: tc.requestPath,
				},
			}
			got := openAICompatImageEndpointPath(opts)
			if got != tc.wantEp {
				t.Errorf("path=%q: endpoint = %q, want %q", tc.requestPath, got, tc.wantEp)
			}
		})
	}
}

// TestOpenAICompatExecutorImageVariations verifies that a POST /images/variations
// request is forwarded as POST {base}/images/variations by the executor.
func TestOpenAICompatExecutorImageVariations(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"abc"}]}`))
	}))
	defer server.Close()

	ex := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	payload := []byte(`{"model":"dall-e-2","n":1}`)
	_, err := ex.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "dall-e-2",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString(openAICompatImageHandlerType),
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/variations",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/images/variations" {
		t.Errorf("path = %q, want /v1/images/variations", gotPath)
	}
	if string(gotBody) != string(payload) {
		t.Errorf("body = %s, want %s", string(gotBody), string(payload))
	}
}
