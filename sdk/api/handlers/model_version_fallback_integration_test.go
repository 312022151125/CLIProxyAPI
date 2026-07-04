package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/tidwall/gjson"
)

func TestExecuteWithAuthManagerFormats_VersionFallbackMultiHopRestoresModel(t *testing.T) {
	const (
		highModel  = "glm-5.2"
		lowModel   = "glm-5.1"
		clientID   = "version-fallback-integration-client"
		providerID = "openai-compat-test"
	)
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(clientID, providerID, []*registry.ModelInfo{
		{ID: highModel},
		{ID: lowModel},
	})
	t.Cleanup(func() { reg.UnregisterClient(clientID) })

	executor := &interceptorCaptureExecutor{provider: providerID}
	executor.execute = func(ctx context.Context, auth *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
		base := strings.TrimSpace(req.Model)
		if base == highModel {
			return coreexecutor.Response{}, &coreauth.Error{
				HTTPStatus: http.StatusBadRequest,
				Message:    "unsupported model",
			}
		}
		if base != lowModel {
			return coreexecutor.Response{}, fmt.Errorf("unexpected execute model %q", req.Model)
		}
		return coreexecutor.Response{
			Payload: []byte(fmt.Sprintf(`{"model":%q,"id":"ok"}`, lowModel)),
		}, nil
	}

	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)
	auth := &coreauth.Auth{
		ID:       clientID,
		Provider: providerID,
		Status:   coreauth.StatusActive,
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register(): %v", err)
	}

	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	handler.SetQuotaExceededBehavior(internalconfig.QuotaExceeded{SwitchPreviewModel: true})

	raw := []byte(fmt.Sprintf(`{"model":%q}`, highModel))
	body, _, errMsg := handler.executeWithAuthManagerFormats(
		context.Background(), "openai", "openai", highModel, raw, "", false, modelExecutionOptions{},
	)
	if errMsg != nil {
		t.Fatalf("executeWithAuthManagerFormats() err = %+v", errMsg)
	}
	gotModel := gjson.GetBytes(body, "model").String()
	if gotModel != highModel {
		t.Fatalf("response model = %q, want %q (user-facing name restored)", gotModel, highModel)
	}
}
