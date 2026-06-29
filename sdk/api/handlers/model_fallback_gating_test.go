package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestShouldAttemptModelVersionFallback(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "model support 400",
			err:  &coreauth.Error{HTTPStatus: http.StatusBadRequest, Message: "unsupported model"},
			want: true,
		},
		{
			name: "model support 422",
			err:  &coreauth.Error{HTTPStatus: http.StatusUnprocessableEntity, Message: "model is not supported"},
			want: true,
		},
		{
			name: "auth not found",
			err:  &coreauth.Error{Code: "auth_not_found", Message: "no auth available", HTTPStatus: http.StatusUnauthorized},
			want: false,
		},
		{
			name: "rate limit",
			err:  &coreauth.Error{Code: "auth_unavailable", Message: "rate limited", HTTPStatus: http.StatusTooManyRequests},
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("boom"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAttemptModelVersionFallback(tt.err); got != tt.want {
				t.Fatalf("shouldAttemptModelVersionFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldAttemptRoutingModelVersionFallback(t *testing.T) {
	tests := []struct {
		name   string
		errMsg *interfaces.ErrorMessage
		want   bool
	}{
		{
			name:   "nil",
			errMsg: nil,
			want:   false,
		},
		{
			name: "unknown provider",
			errMsg: &interfaces.ErrorMessage{
				StatusCode: http.StatusBadGateway,
				Error:      fmt.Errorf("unknown provider for model kimi-k2.6"),
			},
			want: true,
		},
		{
			name: "image only endpoint",
			errMsg: &interfaces.ErrorMessage{
				StatusCode: http.StatusServiceUnavailable,
				Error:      fmt.Errorf("model gpt-image-1 is only supported on /v1/images/generations"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAttemptRoutingModelVersionFallback(tt.errMsg); got != tt.want {
				t.Fatalf("shouldAttemptRoutingModelVersionFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}
