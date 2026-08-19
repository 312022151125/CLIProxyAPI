# Plan: Add `GET /v1/videos/:request_id/content` Route with Relative URL Support

## Overview

The `banliapi.top` API (and other OpenAI-compatible video providers) returns video job metadata
from `GET /v1/videos/:id` with a `"video.url"` field pointing to `/v1/videos/:id/content`.
Clients follow that URL to download the actual video bytes.

Two problems exist today:

1. **Missing route:** `GET /v1/videos/:request_id/content` does not exist on the `/v1` prefix.
   Only `GET /openai/v1/videos/:video_id/content` exists.

2. **Relative URL rejected:** OpenAI-compat providers return a relative URL like
   `/v1/videos/:id/content` (no scheme/host). The current `xaiVideoContentURLFromPayload`
   validation explicitly rejects any URL without `http`/`https` scheme + host, causing a
   `502 BadGateway` on content download.

**Fix:** Store the provider `baseURL` alongside `authID + model` in `videoAuthBinding`, then
resolve relative content URLs against that base URL before proxying the bytes.

**Scope:** Three files touched. No new dependencies. No schema changes to config.

---

## Sub-Tasks

### Sub-Task 1 — Store `baseURL` in `videoAuthBinding`

**Status**: [ ] pending

**Intent**
Extend the in-memory `videoAuthBinding` struct to hold the provider `baseURL` (from
`auth.Attributes["base_url"]`). Update all write-paths to populate it when an auth is
selected after video creation or retrieval. This is the foundation that Sub-Task 2 depends on.

**Expected Outcomes**
- `videoAuthBinding` has a new `baseURL string` field.
- `videoAuthBindingStore.setWithModel` (or a new `setFull` helper) accepts and stores `baseURL`.
- All callers that already call `bindVideoAuthID` / `bindVideoAuthIDAndModelFromPayload` are
  updated to also pass the `baseURL` — extracted from the full `*coreauth.Auth` object via
  `auth.Attributes["base_url"]` after execution completes.
- A new helper `(h *OpenAIAPIHandler) baseURLForVideoAuth(videoID string) string` reads the
  stored binding and returns the `baseURL`, empty string if not found.
- No behavior change for xAI providers (their `baseURL` will be populated but ignored during
  content download since they always return absolute URLs).

**Todo List**
1. In `sdk/api/handlers/openai/openai_videos_handlers.go`:
   a. Add `baseURL string` field to `videoAuthBinding` struct (line ~59).
   b. Update `setWithModel` signature to accept `baseURL string`, store it in the entry.
   c. Update `bindVideoAuthID(videoID, authID, model string)` to accept `baseURL string` and
      pass through to `setWithModel`. Update all 3 call-sites.
   d. Update `bindVideoAuthIDAndModelFromPayload` and `bindVideoAuthIDAndModelFromPayload`
      to also accept `baseURL string`. Update all call-sites.
   e. Add helper: `func (h *OpenAIAPIHandler) baseURLForVideoAuth(videoID string) string`
      that calls `videoAuthBindings.getBinding(videoID)` and returns `binding.baseURL`.
2. In each handler that calls `bindVideoAuthID*`, obtain the `baseURL` by calling
   `h.AuthManager.GetByID(selectedAuthID)` after execution — `auth.Attributes["base_url"]` —
   then pass it to the bind call. This follows the exact same pattern as
   `videoContentDownloadAuth` (line 970-987).

**Relevant Context**
- File: `sdk/api/handlers/openai/openai_videos_handlers.go`
- `videoAuthBinding` struct: line 59–63
- `setWithModel`: line 80–101
- `bindVideoAuthID`: line 338–340
- `bindVideoAuthIDAndModelFromPayload`: line 330–336
- `videoContentDownloadAuth` (pattern to follow): line 970–987 — uses `h.AuthManager.GetByID(authID)` then `auth.Attributes["base_url"]`
- `coreauth.Auth.Attributes["base_url"]` is set for all openai-compat providers by the executor

---

### Sub-Task 2 — Resolve relative content URLs using stored `baseURL`

**Status**: [ ] pending

**Intent**
Update `xaiVideoContentURLFromPayload` (and its call-site in `VideosContent` /
`videosContentByID`) to accept a fallback `baseURL`. When the upstream returns a relative URL
(`/v1/videos/:id/content`), resolve it against the stored provider `baseURL` to form a valid
absolute URL before proxying.

**Expected Outcomes**
- `xaiVideoContentURLFromPayload(payload []byte, baseURL string) (string, error)` — new
  signature accepts a `baseURL` fallback.
- If `video.url` is already an absolute URL (`http`/`https` + host) → behaviour unchanged.
- If `video.url` is a relative path (no scheme/host) AND `baseURL` is non-empty → resolve:
  `strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(relPath, "/")`.
- If relative AND `baseURL` is empty → return error (same as today).
- All call-sites of `xaiVideoContentURLFromPayload` updated to pass `baseURL`.

**Todo List**
1. Update `xaiVideoContentURLFromPayload` signature and logic in
   `sdk/api/handlers/openai/openai_videos_handlers.go` (line 686–696).
2. In `VideosContent` handler (line 846), pass `h.baseURLForVideoAuth(videoID)` as the
   `baseURL` argument to `xaiVideoContentURLFromPayload`.
3. In the new `XAIVideosContent` handler (Sub-Task 3), similarly pass
   `h.baseURLForVideoAuth(requestID)`.

**Relevant Context**
- File: `sdk/api/handlers/openai/openai_videos_handlers.go`
- `xaiVideoContentURLFromPayload`: line 686–696
- `VideosContent` calls it at line 896
- The resolved absolute URL is then passed to `writeVideoContentFromURL` which proxies the
  bytes — no changes needed there

---

### Sub-Task 3 — Add `XAIVideosContent` handler and register the `/v1` route

**Status**: [ ] pending

**Intent**
Add the missing `GET /v1/videos/:request_id/content` endpoint. Extract the shared logic from
`VideosContent` into a private helper `videosContentByID(c, videoID)` so both the existing
`/openai/v1` handler and the new `/v1` handler share code without duplication.

**Expected Outcomes**
- `videosContentByID(c *gin.Context, videoID string)` private helper contains the full
  download logic (currently in `VideosContent` lines 846–909).
- `VideosContent` becomes a 3-line wrapper: reads `c.Param("video_id")`, validates, calls helper.
- `XAIVideosContent` is a 3-line wrapper: reads `c.Param("request_id")`, validates, calls helper.
- `videoContentDownloadAuth` is updated to accept `videoID string` directly (instead of reading
  from `c.Param("video_id")`) so both wrappers can use it.
- Route `v1.GET("/videos/:request_id/content", openaiHandlers.XAIVideosContent)` is added
  in `server_routes.go` immediately after the existing `GET /videos/:request_id` line (line 79).

**Todo List**
1. In `sdk/api/handlers/openai/openai_videos_handlers.go`:
   a. Refactor `videoContentDownloadAuth(c *gin.Context)` → `videoContentDownloadAuth(videoID string)`
      (remove the `c.Param("video_id")` read; accept videoID directly). Update its one call-site
      in `writeVideoContentFromURL` — or pass videoID through the helper chain.
   b. Extract body of `VideosContent` into
      `func (h *OpenAIAPIHandler) videosContentByID(c *gin.Context, videoID string)`.
   c. Replace `VideosContent` body with: validate `c.Param("video_id")` → call
      `h.videosContentByID(c, videoID)`.
   d. Add `XAIVideosContent`: validate `c.Param("request_id")` → call
      `h.videosContentByID(c, requestID)`.
2. In `internal/api/server_routes.go`:
   Add `v1.GET("/videos/:request_id/content", openaiHandlers.XAIVideosContent)` after line 79.

**Relevant Context**
- File: `sdk/api/handlers/openai/openai_videos_handlers.go`
- `VideosContent`: line 846–909 (to be refactored into helper)
- `videoContentDownloadAuth`: line 970–987 (reads `c.Param("video_id")` — must be decoupled)
- File: `internal/api/server_routes.go`
- Insert after line 79: `v1.GET("/videos/:request_id", openaiHandlers.XAIVideosRetrieve)`
- Gin wildcard name constraint: all routes under `/videos/:X` in the same group must use the
  same wildcard name (`:request_id` is already established by DELETE and remix routes)

---

### Sub-Task 4 — Build and test verification

**Status**: [ ] pending

**Intent**
Confirm all changes compile and existing tests pass.

**Expected Outcomes**
- `go build` exits 0 with no new errors.
- Package tests for video handlers and executor pass.

**Todo List**
1. Run `go build -o test-output ./cmd/server && rm -f test-output`
2. Run `go test ./sdk/api/handlers/openai/... ./internal/runtime/executor/...`
3. Fix any compilation errors from signature changes (setWithModel, bindVideoAuthID, etc.)

**Relevant Context**
- No new dependencies or external packages needed
- All bind call-sites are in `openai_videos_handlers.go` — no other files use `bindVideoAuthID*` directly
