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

func TestOpenAICompatEndpointPathMapsExtraEndpoints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "search", path: "/v1/search", want: "/search"},
		{name: "ppt", path: "/v1/ppt/generations", want: "/ppt/generations"},
		{name: "psd", path: "/v1/psd/generations", want: "/psd/generations"},
		{name: "editable", path: "/v1/editable-file-tasks", want: "/editable-file-tasks"},
		{name: "files", path: "/files/abc.png", want: "/files/abc.png"},
		{name: "embeddings", path: "/v1/embeddings", want: "/embeddings"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := openAICompatEndpointPath(cliproxyexecutor.Options{
				SourceFormat: sdktranslator.FromString("openai"),
				Metadata: map[string]any{
					cliproxyexecutor.RequestPathMetadataKey: tc.path,
				},
			})
			if got != tc.want {
				t.Fatalf("openAICompatEndpointPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestOpenAICompatExecutorForwardsMethodAndQuery(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model: "compat-model",
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Method:       http.MethodGet,
		Query:        url.Values{"ids": []string{"a", "b"}},
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/editable-file-tasks",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/v1/editable-file-tasks" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/editable-file-tasks")
	}
	if gotQuery.Get("ids") != "a" {
		t.Fatalf("query ids = %q, want a", gotQuery.Get("ids"))
	}
	if len(gotQuery["ids"]) != 2 || gotQuery["ids"][1] != "b" {
		t.Fatalf("query ids = %#v, want [a b]", gotQuery["ids"])
	}
}

func TestOpenAICompatExecutorPassthroughPOSTSearch(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	payload := []byte(`{"model":"compat-model","query":"hello"}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "compat-model",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FromString("openai"),
		OriginalRequest: payload,
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/search",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if string(gotBody) != string(payload) {
		t.Fatalf("body = %s, want %s", string(gotBody), string(payload))
	}
}
