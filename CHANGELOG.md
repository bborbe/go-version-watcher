# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## v0.4.0

- Add `TARGET_VAULT` env (standard task-routing param alongside `TASK_ASSIGNEE`/`TASK_STATUS`/`TASK_PHASE`/`TASK_SUFFIX`): sets the emitted `CreateCommand.TargetVault` (via the sender's default-vault) so the task-controller materializes the task into the named Obsidian vault (e.g. `personal` → `24 Tasks/`). Empty = controller default (openclaw).

## v0.3.0

- Add `SEED_VERSION` env (cold-start seeds a lower version so the first poll
  emits a task for the current latest) and cursor admin endpoints
  `/resetcursor`, `/setcursor/{version}`, `/trigger` (force an immediate poll)
  for operational reset / live end-to-end testing.

## v0.2.0

- Emitted-task assignee/status/phase and an optional title suffix are now
  env-configurable (`TASK_ASSIGNEE`/`TASK_STATUS`/`TASK_PHASE`/`TASK_SUFFIX`),
  defaulting to human/in_progress/todo/none. Enables distinct routing per
  deployment (quant vs octopus).

## v0.1.0

- Initial release. Polls `https://go.dev/dl/?mode=json` on an interval, computes the
  max `stable:true` Go version, and emits one deduplicated `CreateTaskCommand`
  (`task_type: go-version-update`, `assignee: human`) per new version pointing at
  the Go update runbook. Persists a single-value cursor (`LastSeenVersion`) at
  `/data/cursor.json`. Builds and publishes
  `docker.io/bborbe/go-version-watcher:<version>` via `make buca`.
