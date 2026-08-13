# Sub2API v0.1.176 Upstream Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge `Wei-Shaw/sub2api main@0e82efe48951cb7da1f8554639afdeab05bf16b8` into a new branch based on local `main`, preserve unique local features, adopt upstream for fully overlapping features, and stop before commit for user review.

**Architecture:** Treat the requested upstream SHA as an immutable second-parent boundary. Use Git three-way evidence to distinguish local-only, upstream-only, and overlapping paths; resolve only textual or necessary semantic merge incompatibilities, regenerate owned Ent/Wire outputs from merged sources, and do not repair bugs already present in the upstream target.

**Tech Stack:** Git, Go 1.26.5, Gin, Ent, Wire, Vue 3, Vite, Pinia, pnpm 9.15.9, Vitest, llm-wiki, Understand Anything.

## Global Constraints

- Base the branch on local `main`, without pulling or rebasing it.
- Create `feature/hy/10176_merge_sub2api_176`.
- Merge only `0e82efe48951cb7da1f8554639afdeab05bf16b8` from `Wei-Shaw/sub2api` `main`.
- Preserve unique local functionality and every tracked local file under `docs/features/`.
- Where local and upstream functionality fully overlap, use the upstream implementation.
- Resolve merge conflicts and merge-required compatibility only; do not fix upstream bugs.
- Update the append-only merge ledger and all affected `llm-wiki/wiki/*.md` pages.
- Run the repository-matched backend, frontend, generation, build, lint, and wiki checks.
- Keep `MERGE_HEAD` and the complete staged candidate; do not commit, push, create a PR, or deploy before user approval.

---

### Task 1: Lock The Merge Boundary And Create The Branch

**Files:**
- Inspect: `backend/cmd/server/VERSION`
- Inspect: Git refs and merge history

**Interfaces:**
- Consumes: local `main@cf5b7ee5a15cd04bb6134a372d20e86534660b93`
- Produces: branch `feature/hy/10176_merge_sub2api_176` and verified upstream object/version evidence

- [x] **Step 1:** Fetch `upstream/main` and the exact requested object without changing local `main`.
- [x] **Step 2:** Verify the target object, `VERSION=0.1.176`, ancestry, merge base, and first-parent/upstream deltas.
- [x] **Step 3:** Run `git merge-tree --write-tree --messages main 0e82efe48951cb7da1f8554639afdeab05bf16b8` and record textual conflicts plus overlap paths.
- [x] **Step 4:** Create `feature/hy/10176_merge_sub2api_176` directly from local `main`.

### Task 2: Merge And Resolve Only Merge Incompatibilities

**Files:**
- Modify: only files selected by the fixed upstream merge, explicit conflicts, required generated outputs, and merge documentation
- Preserve: `docs/features/**` except the required append to `docs/features/sub2api -merage-list.md`

**Interfaces:**
- Consumes: Git stages 1/2/3 for each conflict and the local feature inventory
- Produces: zero unmerged paths with upstream behavior plus non-overlapping local behavior

- [x] **Step 1:** Run `git merge --no-commit --no-ff 0e82efe48951cb7da1f8554639afdeab05bf16b8`.
- [x] **Step 2:** Resolve every textual conflict by comparing merge base, local first parent, and fixed upstream target.
- [x] **Step 3:** Review all both-modified auto-merged paths for semantic loss, especially routes, gateway adapters, settings, repositories, Ent schemas, Wire providers, frontend API/store contracts, and tests.
- [x] **Step 4:** Inventory local feature paths and prove no tracked `docs/features/` file was deleted; use upstream implementations only for fully overlapping behavior.
- [x] **Step 5:** Record upstream or baseline defects without changing their production implementation or tests unless a test cannot compile solely because the merge changed an interface.

### Task 3: Regenerate Owned Outputs And Update Knowledge

**Files:**
- Regenerate when source inputs changed: `backend/ent/**`, `backend/cmd/server/wire_gen.go`
- Modify: `docs/features/sub2api -merage-list.md`
- Modify: affected `llm-wiki/wiki/*.md`
- Regenerate: `llm-wiki/.understand-anything/knowledge-graph.json`

**Interfaces:**
- Consumes: resolved source graph and audited upstream delta
- Produces: source-consistent generated code, append-only merge record, current project knowledge

- [x] **Step 1:** Run Ent and Wire generation when their source definitions changed, then prove a second generation has no drift.
- [x] **Step 2:** Append the v0.1.176 merge record with branch, base, merge base, exact upstream SHA, conflict decisions, local feature preservation, known upstream issue boundary, and verification status.
- [x] **Step 3:** Update only wiki pages whose architecture, configuration, schema, security, gateway, frontend, or operational facts changed.
- [x] **Step 4:** Refresh the wiki knowledge graph and validate its JSON, source hash, links, and expected dirty-status interpretation.

### Task 4: Verify And Prepare The Commit Review Gate

**Files:**
- Verify: complete staged merge candidate

**Interfaces:**
- Consumes: resolved merge candidate
- Produces: review-ready `MERGE_HEAD` state with exact pass/fail and baseline/upstream-failure evidence

- [x] **Step 1:** Run focused tests for conflict and high-risk changed packages using repository-local Go caches and a fresh `GOTMPDIR`.
- [x] **Step 2:** Run backend default, unit, and integration suites; Go builds; lint; and generated-code drift checks according to `llm-wiki/wiki/ops.md`.
- [x] **Step 3:** Run frontend lint, typecheck, focused tests, full Vitest, and production build with pnpm 9.15.9.
- [x] **Step 4:** Check conflict markers, unmerged entries, whitespace, changed-file counts, upstream-only blob fidelity, local-only blob preservation, and `docs/features/` deletions.
- [x] **Step 5:** Stage the complete candidate while retaining `MERGE_HEAD`; verify `HEAD` is still the local-main first parent and no commit/push/PR/deploy occurred.
- [x] **Step 6:** Notify the user that the candidate is ready for commit review, including exact SHA, conflict list, change counts, tests, known unfixed upstream/baseline issues, and Git state.
