# Local Request Archive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in local JSONL archive for AI gateway requests and responses, including request body, client device information, and response body correlated by `archive_id`.

**Architecture:** Implement a focused Gin middleware in `backend/internal/server/middleware` that reads and restores request bodies, wraps the response writer for bounded capture, and writes JSONL events through a small file-backed archiver. Register the middleware only on AI gateway routes.

**Tech Stack:** Go 1.26, Gin, existing `config.GatewayConfig`, existing gateway routes and middleware stack.

---

### Task 1: Config and Archive Package

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/server/middleware/request_archive.go`
- Test: `backend/internal/server/middleware/request_archive_test.go`

- [ ] **Step 1: Write failing config and writer tests**

Create tests that instantiate the middleware with `Enabled: true`, send a JSON request through a tiny Gin router, and assert two JSONL records are written with the same `archive_id`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/middleware -run RequestArchive -v`

Expected: FAIL because `RequestArchiveConfig` and middleware do not exist yet.

- [ ] **Step 3: Add config type and middleware implementation**

Add `GatewayRequestArchiveConfig` under `GatewayConfig` with fields `Enabled`, `Dir`, `MaxRequestBodyBytes`, `MaxResponseBodyBytes`, and `CaptureResponse`. Implement middleware that writes request and response events.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/middleware -run RequestArchive -v`

Expected: PASS.

### Task 2: Route Integration

**Files:**
- Modify: `backend/internal/server/routes/gateway.go`
- Test: `backend/internal/server/routes/gateway_test.go`

- [ ] **Step 1: Write failing route test**

Add a route-level test that enables request archive config, hits a gateway POST path, and asserts the route stack runs the archive middleware before the handler.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/routes -run RequestArchive -v`

Expected: FAIL because gateway routes do not register the middleware.

- [ ] **Step 3: Register middleware on AI gateway groups and alias routes**

Create `requestArchive := middleware.RequestArchive(cfg.Gateway.RequestArchive)` and add it after `bodyLimit` and before handlers on `/v1`, `/v1beta`, `/responses`, `/backend-api/codex`, `/chat/completions`, `/images/*`, `/antigravity/v1`, and `/antigravity/v1beta`.

- [ ] **Step 4: Run route test to verify it passes**

Run: `go test ./internal/server/routes -run RequestArchive -v`

Expected: PASS.

### Task 3: Defaults, Validation, and Safety

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `.gitignore`
- Test: `backend/internal/config/config_test.go`

- [ ] **Step 1: Write failing config default test**

Assert defaults: `gateway.request_archive.enabled=true`, `dir=data/request-archive`, `max_request_body_bytes=8388608`, `max_response_body_bytes=2097152`, `capture_response=true`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run RequestArchive -v`

Expected: FAIL because defaults are not registered.

- [ ] **Step 3: Add defaults and ignore archive directory**

Set Viper defaults and add `backend/data/request-archive/` plus `data/request-archive/` to `.gitignore`.

- [ ] **Step 4: Run config test to verify it passes**

Run: `go test ./internal/config -run RequestArchive -v`

Expected: PASS.

### Task 4: Verification

**Files:**
- No new files unless tests expose issues.

- [ ] **Step 1: Run focused backend tests**

Run: `go test ./internal/server/middleware ./internal/server/routes ./internal/config`

Expected: PASS.

- [ ] **Step 2: Run handler smoke tests for touched behavior**

Run: `go test ./internal/handler -run 'OpenAI|Gateway|Gemini|Image|RequestBody'`

Expected: PASS.

- [ ] **Step 3: Review git diff**

Run: `git diff --check` and `git status --short`

Expected: no whitespace errors; changed files limited to config, middleware, routes, docs, tests, and `.gitignore`.
