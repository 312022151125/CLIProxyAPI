package handlers

import (
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// shouldAttemptModelVersionFallback reports whether a handler should retry with a downgraded
// model after AuthManager execution failed.
func shouldAttemptModelVersionFallback(err error) bool {
	return coreauth.ModelSupportError(err)
}

// shouldAttemptRoutingModelVersionFallback reports whether a handler should retry with a
// downgraded model after provider/model routing failed.
func shouldAttemptRoutingModelVersionFallback(errMsg *interfaces.ErrorMessage) bool {
	if errMsg == nil {
		return false
	}
	// Image-only models reject non-image endpoints; downgrading the model id does not help.
	if errMsg.StatusCode == http.StatusServiceUnavailable {
		return false
	}
	return true
}
