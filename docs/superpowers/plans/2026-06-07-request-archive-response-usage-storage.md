# Request Archive Response Usage Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop storing archived response bodies while preserving token usage metadata in request archive response records.

**Architecture:** Keep the existing `RequestArchive` middleware and JSONL writer. Change the response writer wrapper so it tracks response size/hash/stream state and extracts lightweight `usage` data from JSON/SSE chunks without persisting the response body.

**Tech Stack:** Go, Gin middleware, JSONL request archive, Vue 3 admin settings page.

---

### Task 1: Backend Red Tests

**Files:**
- Modify: `backend/internal/server/middleware/request_archive_test.go`

- [x] Add a test proving `capture_response=true` no longer writes response `body`, but writes `usage` for a non-stream JSON response.
- [x] Add a test proving SSE `data:` chunks produce a response `usage` record without writing response `body`.
- [x] Run `cd backend && go test ./internal/server/middleware/ -run "RequestArchive.*Usage|RequestArchive.*ResponseBody"` and confirm the new tests fail before implementation.

### Task 2: Backend Implementation

**Files:**
- Modify: `backend/internal/server/middleware/request_archive.go`

- [x] Add `Usage map[string]any json:"usage,omitempty"` to `requestArchiveRecord`.
- [x] Replace response body buffering with metadata tracking only.
- [x] Extract usage from non-stream JSON responses at `usage` and `response.usage`.
- [x] Extract latest usage from SSE `data:` JSON chunks.
- [x] Keep `body_size`, `body_sha256`, `body_truncated`, and `stream` semantics.
- [x] Run middleware tests and fix regressions.

### Task 3: Frontend Text

**Files:**
- Modify: `frontend/src/views/admin/SettingsView.vue` or i18n files if the text is keyed there.

- [x] Update the request archive response toggle copy so admins understand it captures response metadata/token usage, not response content.
- [ ] Run the narrow frontend validation if TypeScript/i18n changes require it.

### Task 4: Project Knowledge

**Files:**
- Modify: `docs/features/request-archive-response-usage-storage-design-cn.md`
- Modify: `docs/features/request-archive-async-writer-technical-notes-cn.md`
- Modify: `llm-wiki/wiki/backend.md`
- Modify: `deploy/config.example.yaml`

- [x] Record the final behavior and storage reason.
- [x] Update `llm-wiki` so future agents do not assume `capture_response` stores response body.

### Task 5: Verification

**Files:**
- No new files expected.

- [x] Run `cd backend && go test ./internal/server/middleware/`.
- [x] Run `cd backend && go test ./internal/config/ ./internal/service/ -run RequestArchive`.
- [x] Run frontend validation if frontend source changes require it.
- [x] Check `git diff --stat` and summarize behavior changes.
