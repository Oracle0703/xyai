# Token Analysis Production Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Token Analysis page usable with multi-GB production request archives by adding resumable indexing, safer archive-line handling, and clearer analysis indicators.

**Architecture:** Keep the existing asynchronous request-archive indexer and admin API. Add repository access to index state so the service can resume from the last processed byte offset, skip incomplete trailing lines without poisoning the index state, and expose enough aggregate fields for the frontend to show matched/unmatched and risk distribution signals.

**Tech Stack:** Go 1.26, Gin, PostgreSQL repository layer, Vue 3, TypeScript, Vitest.

---

### Task 1: Resumable Indexer

**Files:**
- Modify: `backend/internal/service/token_analysis_types.go`
- Modify: `backend/internal/service/token_analysis_indexer.go`
- Modify: `backend/internal/service/token_analysis_indexer_test.go`
- Modify: `backend/internal/repository/token_analysis_repo.go`

- [ ] Write failing tests for resuming from a stored offset and for ignoring an incomplete trailing JSON line.
- [ ] Add `GetIndexState(ctx, sourceFile)` to `TokenAnalysisRepository`.
- [ ] Implement repository lookup for `token_analysis_index_state`.
- [ ] Seek to `last_offset` before scanning a file, and only process bytes after that offset.
- [ ] Treat EOF bad JSON without a trailing newline as skipped, because the writer may still be appending the current line.
- [ ] Run `go test ./internal/service -run TokenAnalysisIndexer` and `go test ./internal/repository -run TokenAnalysisRepository`.

### Task 2: Analysis Summary Fields

**Files:**
- Modify: `backend/internal/service/token_analysis_types.go`
- Modify: `backend/internal/repository/token_analysis_repo.go`
- Modify: `frontend/src/api/admin/tokenAnalysis.ts`

- [ ] Add aggregate summary fields for unmatched requests and risk reason distribution.
- [ ] Populate the fields in `GetSummary` without returning request or response bodies.
- [ ] Update frontend TypeScript types.
- [ ] Run backend repository tests and frontend token analysis API tests.

### Task 3: Page Enhancements

**Files:**
- Modify: `frontend/src/views/admin/TokenAnalysisView.vue`
- Modify: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] Add a failing view test that expects unmatched ratio and risk reason distribution to render.
- [ ] Add compact summary cards for matched/unmatched and risk ratio.
- [ ] Add risk reason filter chips sourced from summary distribution.
- [ ] Improve index status display with per-file rows and failed/processed counters.
- [ ] Run `npm run test:run -- src/views/admin/__tests__/TokenAnalysisView.spec.ts` and `npm run typecheck`.

### Task 4: Verification

**Files:**
- Review changed files only.

- [ ] Run `go test ./internal/service ./internal/repository ./internal/handler/admin -run TokenAnalysis`.
- [ ] Run `npm run test:run -- src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts`.
- [ ] Run `npm run typecheck`.
- [ ] Run `git diff --check` and inspect `git status --short`.
