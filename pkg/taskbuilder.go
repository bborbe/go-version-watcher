// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"strings"

	agentlib "github.com/bborbe/agent"
	task "github.com/bborbe/agent/command/task"
)

// TaskConfig groups per-task envelope settings (stage routing + emitted-task
// frontmatter knobs).
type TaskConfig struct {
	Stage    string // "dev" or "prod" — frontmatter `stage`
	Assignee string // frontmatter `assignee` (default "human")
	Status   string // frontmatter `status` (default "in_progress")
	Phase    string // frontmatter `phase` (default "todo")
	Suffix   string // optional title/filename suffix appended as " - <suffix>"; empty = none
}

// releaseNotesBaseURL is the go.dev release-notes page; the version string is
// appended as an anchor (e.g. #go1.27.0).
const releaseNotesBaseURL = "https://go.dev/doc/devel/release#"

// downloadsURL is the go.dev downloads page.
const downloadsURL = "https://go.dev/dl/"

// BuildCreateCommand assembles the CreateTaskCommand for a new Go version.
// newVersion and previousVersion are canonical go-version strings (e.g.
// "go1.27.0"); releaseKind is "minor" or "patch".
func BuildCreateCommand(
	newVersion string,
	previousVersion string,
	releaseKind string,
	cfg TaskConfig,
) task.CreateCommand {
	taskIDStr := DeriveTaskID(newVersion).String()
	number := strings.TrimPrefix(newVersion, "go")
	title := computeTitle(number, cfg.Suffix)
	return task.CreateCommand{
		Title:          title,
		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
		Frontmatter: buildFrontmatter(
			newVersion,
			previousVersion,
			releaseKind,
			taskIDStr,
			title,
			cfg,
		),
		Body: buildTaskBody(newVersion, number, releaseKind),
	}
}

func buildFrontmatter(
	newVersion string,
	previousVersion string,
	releaseKind string,
	taskIDStr string,
	title string,
	cfg TaskConfig,
) agentlib.TaskFrontmatter {
	return agentlib.TaskFrontmatter{
		"task_type":         "go-version-update",
		"assignee":          cfg.Assignee,
		"phase":             cfg.Phase,
		"status":            cfg.Status,
		"stage":             cfg.Stage,
		"task_identifier":   taskIDStr,
		"title":             title,
		"new_version":       newVersion,
		"previous_version":  previousVersion,
		"release_kind":      releaseKind,
		"release_notes_url": releaseNotesBaseURL + newVersion,
	}
}

// computeTitle returns the human-readable task title. The base title is
// "Update Go to {number}"; when suffix is non-empty it is appended as
// " - <suffix>" (feeding both the title frontmatter and the derived filename).
func computeTitle(number string, suffix string) string {
	title := "Update Go to " + number
	if suffix != "" {
		title += " - " + suffix
	}
	return title
}

func buildTaskBody(newVersion string, number string, releaseKind string) string {
	return fmt.Sprintf(
		"# Update Go to %s\n\nGo %s released (%s). Run [[Go - Update Version]] across bborbe Go repos.\n- Release notes: %s%s\n- Downloads: %s\n",
		number,
		number,
		releaseKind,
		releaseNotesBaseURL,
		newVersion,
		downloadsURL,
	)
}
