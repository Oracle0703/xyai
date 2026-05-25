# Image Generation Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a public `/image-gen` operations page that calls the existing image generation gateway with a user-provided API key and stores local generation history.

**Architecture:** Add one focused Vue view and one public router entry. The view owns form state, gateway fetch, response normalization, and local history persistence.

**Tech Stack:** Vue 3, Vue Router, Tailwind CSS, Vitest, Vue Test Utils.

---

### Task 1: Add Page Behavior Tests

**Files:**
- Create: `frontend/src/views/__tests__/ImageGenView.spec.ts`
- Modify: none

- [ ] **Step 1: Write failing tests**

Create tests that mount `ImageGenView`, submit a prompt with an API key, assert the `POST /v1/images/generations` request shape, and assert a local history entry is written.

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm --dir frontend test:run src/views/__tests__/ImageGenView.spec.ts`

Expected: fail because `ImageGenView.vue` does not exist yet.

### Task 2: Implement ImageGenView

**Files:**
- Create: `frontend/src/views/ImageGenView.vue`
- Test: `frontend/src/views/__tests__/ImageGenView.spec.ts`

- [ ] **Step 1: Build the page**

Implement the public operations tool page with API key input, prompt textarea, size and count controls, generate button, result gallery, and local history list.

- [ ] **Step 2: Run targeted tests**

Run: `pnpm --dir frontend test:run src/views/__tests__/ImageGenView.spec.ts`

Expected: pass.

### Task 3: Register Public Route

**Files:**
- Modify: `frontend/src/router/index.ts`

- [ ] **Step 1: Add route**

Register `/image-gen` with `requiresAuth: false`.

- [ ] **Step 2: Run route-adjacent checks**

Run: `pnpm --dir frontend typecheck`

Expected: exit 0.

### Task 4: Final Verification

**Files:**
- Verify changed frontend files.

- [ ] **Step 1: Run targeted test**

Run: `pnpm --dir frontend test:run src/views/__tests__/ImageGenView.spec.ts`

Expected: exit 0.

- [ ] **Step 2: Run typecheck**

Run: `pnpm --dir frontend typecheck`

Expected: exit 0.
