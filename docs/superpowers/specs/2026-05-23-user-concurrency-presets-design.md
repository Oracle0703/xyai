# User Concurrency Presets Design

## Goal

Add an admin-only feature for saving and applying user concurrency presets. A preset changes only selected users' `concurrency` values. It can be applied manually or automatically once per day at a configured time.

## Scope

Included:

- Preset CRUD for admins.
- Preset target users stored as explicit user IDs.
- One target concurrency value per preset.
- Manual "apply preset" action.
- Optional daily schedule using `HH:mm`.
- Execution logs for manual and scheduled runs.
- Auth cache invalidation for affected users after a successful apply.

Excluded from the first version:

- Account concurrency changes.
- User RPM changes.
- Cron expression editing.
- Weekly schedules.
- Automatic selection by tags, notes, API keys, or usage rules.

## Data Model

### `user_concurrency_presets`

Stores the editable preset definition.

- `id`
- `name`
- `description`
- `target_concurrency`
- `user_ids` as JSON array
- `schedule_enabled`
- `schedule_time` in `HH:mm`
- `last_scheduled_run_date`
- `created_at`
- `updated_at`

### `user_concurrency_preset_runs`

Stores each apply attempt.

- `id`
- `preset_id`
- `trigger` as `manual` or `scheduled`
- `target_concurrency`
- `user_ids` as JSON array
- `affected_count`
- `status` as `success` or `failed`
- `error_message`
- `started_at`
- `finished_at`

## Backend Design

Add a dedicated service for preset management instead of expanding the general admin user service too much.

Main methods:

- `ListPresets`
- `CreatePreset`
- `UpdatePreset`
- `DeletePreset`
- `ApplyPreset`
- `ListPresetRuns`
- `RunDueSchedules`

Validation:

- `name` is required.
- `target_concurrency >= 1`.
- `user_ids` must be non-empty.
- Admin users are rejected as preset targets.
- Deleted or missing users are ignored during apply and reported through the execution log.
- `schedule_time` must match `HH:mm` when scheduling is enabled.

Apply behavior:

1. Load the preset.
2. Validate and normalize target users.
3. Set selected users to `target_concurrency`.
4. Invalidate auth cache for affected users.
5. Write a run log.

Scheduled behavior:

- A background runner checks once per minute.
- It uses configured application timezone when available, otherwise server local time.
- A scheduled preset runs at most once per local calendar date.
- The runner updates `last_scheduled_run_date` only after a successful scheduled apply.

## API Design

Base path: `/api/v1/admin/users/concurrency-presets`.

- `GET /` list presets.
- `POST /` create preset.
- `PUT /:id` update preset.
- `DELETE /:id` delete preset.
- `POST /:id/apply` manually apply preset.
- `GET /:id/runs` list recent run logs.

## Frontend Design

Add a "Concurrency Presets" action to the admin user management page.

UI elements:

- Preset list dialog.
- Create/edit preset dialog.
- User picker with search and selected user summary.
- Target concurrency numeric input.
- Daily schedule toggle.
- `HH:mm` schedule time input.
- Manual apply button with confirmation.
- Recent run status for each preset.

The first version keeps the UI compact and uses the existing admin users API for searching users.

## Error Handling

- Invalid input returns 400 with a clear message.
- Missing preset returns 404.
- Partial target mismatch is not fatal if at least one valid non-admin user remains.
- If no valid users remain, apply fails and logs a failed run.
- Scheduled runner logs errors but does not crash the service.

## Tests

Backend:

- Create/update validation.
- Reject admin users as targets.
- Manual apply updates selected users only.
- Auth cache invalidation is called for affected users.
- Scheduled apply runs once per day.
- Run logs record success and failure.

Frontend:

- Preset dialog renders.
- Create/edit request payloads are correct.
- Manual apply confirmation calls the API.
- Schedule toggle validates `HH:mm`.

