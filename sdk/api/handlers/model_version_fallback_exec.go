package handlers

import (
	"context"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/modelversion"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const maxVersionFallbackHops = 16

// QuotaExceededBehavior mirrors config.QuotaExceeded fields used by handler-level model failover.
type QuotaExceededBehavior struct {
	SwitchPreviewModel bool
}

func (h *BaseAPIHandler) modelVersionFallbackEnabled() bool {
	return h != nil && h.quotaExceeded.SwitchPreviewModel
}

func (h *BaseAPIHandler) shouldAttemptModelVersionFallback(err error) bool {
	if h == nil || !h.modelVersionFallbackEnabled() {
		return false
	}
	return coreauth.ModelSupportError(err) || coreauth.ModelQuotaOrCapacityError(err)
}

func (h *BaseAPIHandler) shouldAttemptRoutingModelVersionFallback(errMsg *interfaces.ErrorMessage) bool {
	if h == nil || !h.modelVersionFallbackEnabled() || errMsg == nil {
		return false
	}
	// Image-only models reject non-image endpoints; downgrading the model id does not help.
	if errMsg.StatusCode == http.StatusServiceUnavailable {
		return false
	}
	return true
}

func nextFallbackModel(requested string, providers []string) string {
	return modelversion.Next(requested, providers)
}

func (h *BaseAPIHandler) executeWithAuthManagerFormatsWithVersionFallback(
	ctx context.Context, entryProtocol, exitProtocol, modelName string,
	rawJSON []byte, alt string, allowImageModel bool, execOptions modelExecutionOptions,
	providers []string,
) ([]byte, http.Header, *interfaces.ErrorMessage) {
	originalRequestedModel := modelName
	currentModel := modelName
	currentPayload := rawJSON
	var lastErr *interfaces.ErrorMessage

	for range maxVersionFallbackHops {
		body, headers, errMsg := h.executeWithAuthManagerFormatsOnce(
			ctx, entryProtocol, exitProtocol, currentModel, currentPayload, alt, allowImageModel, execOptions,
		)
		if errMsg == nil {
			return restoreOriginalModelInBody(body, originalRequestedModel), headers, nil
		}
		lastErr = errMsg
		if !h.shouldAttemptRoutingModelVersionFallback(errMsg) && !h.shouldAttemptModelVersionFallback(errMsg.Error) {
			return nil, headers, errMsg
		}
		next := nextFallbackModel(currentModel, providers)
		if next == "" {
			return nil, headers, errMsg
		}
		currentModel = next
		currentPayload = withFallbackModelInPayload(rawJSON, next)
	}
	return nil, nil, lastErr
}

func (h *BaseAPIHandler) executeCountWithAuthManagerWithVersionFallback(
	ctx context.Context, handlerType, modelName string,
	rawJSON []byte, alt string, execOptions modelExecutionOptions,
	providers []string,
) ([]byte, http.Header, *interfaces.ErrorMessage) {
	originalRequestedModel := modelName
	currentModel := modelName
	currentPayload := rawJSON
	var lastErr *interfaces.ErrorMessage

	for range maxVersionFallbackHops {
		body, headers, errMsg := h.executeCountWithAuthManagerOnce(
			ctx, handlerType, currentModel, currentPayload, alt, execOptions,
		)
		if errMsg == nil {
			return restoreOriginalModelInBody(body, originalRequestedModel), headers, nil
		}
		lastErr = errMsg
		if !h.shouldAttemptRoutingModelVersionFallback(errMsg) && !h.shouldAttemptModelVersionFallback(errMsg.Error) {
			return nil, headers, errMsg
		}
		next := nextFallbackModel(currentModel, providers)
		if next == "" {
			return nil, headers, errMsg
		}
		currentModel = next
		currentPayload = withFallbackModelInPayload(rawJSON, next)
	}
	return nil, nil, lastErr
}

func (h *BaseAPIHandler) executeStreamWithAuthManagerFormatsWithVersionFallback(
	ctx context.Context, entryProtocol, exitProtocol, modelName string,
	rawJSON []byte, alt string, allowImageModel bool, execOptions modelExecutionOptions,
	providers []string,
) (<-chan []byte, http.Header, <-chan *interfaces.ErrorMessage) {
	originalRequestedModel := modelName
	currentModel := modelName
	currentPayload := rawJSON

	for range maxVersionFallbackHops {
		dataChan, headers, errChan := h.executeStreamWithAuthManagerFormatsOnce(
			ctx, entryProtocol, exitProtocol, currentModel, currentPayload, alt, allowImageModel, execOptions,
		)
		if dataChan != nil {
			wrapped := make(chan []byte)
			go func(displayModel string, in <-chan []byte) {
				defer close(wrapped)
				for chunk := range in {
					if len(chunk) > 0 {
						chunk = restoreOriginalModelInChunk(chunk, displayModel)
					}
					if ctx == nil {
						wrapped <- chunk
						continue
					}
					select {
					case <-ctx.Done():
						return
					case wrapped <- chunk:
					}
				}
			}(originalRequestedModel, dataChan)
			return wrapped, headers, errChan
		}
		var errMsg *interfaces.ErrorMessage
		select {
		case errMsg = <-errChan:
		default:
		}
		if errMsg == nil {
			return nil, headers, errChan
		}
		if !h.shouldAttemptRoutingModelVersionFallback(errMsg) && !h.shouldAttemptModelVersionFallback(errMsg.Error) {
			out := make(chan *interfaces.ErrorMessage, 1)
			out <- errMsg
			close(out)
			return nil, headers, out
		}
		next := nextFallbackModel(currentModel, providers)
		if next == "" {
			out := make(chan *interfaces.ErrorMessage, 1)
			out <- errMsg
			close(out)
			return nil, headers, out
		}
		currentModel = next
		currentPayload = withFallbackModelInPayload(rawJSON, next)
	}
	errChan := make(chan *interfaces.ErrorMessage, 1)
	errChan <- &interfaces.ErrorMessage{StatusCode: http.StatusBadGateway, Error: http.ErrHandlerTimeout}
	close(errChan)
	return nil, nil, errChan
}
