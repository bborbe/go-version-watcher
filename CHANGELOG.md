# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## v0.1.0

- Initial release. Polls `https://go.dev/dl/?mode=json` on an interval, computes the
  max `stable:true` Go version, and emits one deduplicated `CreateTaskCommand`
  (`task_type: go-version-update`, `assignee: human`) per new version pointing at
  the Go update runbook. Persists a single-value cursor (`LastSeenVersion`) at
  `/data/cursor.json`. Builds and publishes
  `docker.io/bborbe/go-version-watcher:<version>` via `make buca`.
