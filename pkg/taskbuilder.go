// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"bytes"
	"context"
	"strings"
	"text/template"

	agentlib "github.com/bborbe/agent"
	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/errors"
)

// TaskConfig groups per-task envelope settings (stage routing + emitted-task
// frontmatter knobs).
type TaskConfig struct {
	Stage    string // "dev" or "prod" — frontmatter `stage`
	Assignee string // frontmatter `assignee` (default "human")
	Status   string // frontmatter `status` (default "in_progress")
	Phase    string // frontmatter `phase` (default "todo")
	Suffix   string // optional title/filename suffix appended as " - <suffix>"; empty = none

	// TitleTemplate overrides the emitted-task title; nil ⇒ defaultTitleTemplate.
	TitleTemplate *template.Template
	// BodyTemplate overrides the emitted-task body; nil ⇒ defaultBodyTemplate.
	BodyTemplate *template.Template
}

// taskTemplateData is the data passed to the title/body text/templates.
type taskTemplateData struct {
	Version         string // canonical go-version string, e.g. "go1.26.5"
	Number          string // version number without the "go" prefix, e.g. "1.26.5"
	ReleaseKind     string // "minor" or "patch"
	PreviousVersion string // previous canonical go-version string, e.g. "go1.26.4"
	ReleaseNotesURL string // full go.dev release-notes anchor URL
	DownloadsURL    string // go.dev downloads page URL
}

// releaseNotesBaseURL is the go.dev release-notes page; the version string is
// appended as an anchor (e.g. #go1.27.0).
const releaseNotesBaseURL = "https://go.dev/doc/devel/release#"

// downloadsURL is the go.dev downloads page.
const downloadsURL = "https://go.dev/dl/"

// defaultTitleTemplate renders the built-in emitted-task title when no
// TASK_TITLE_TEMPLATE override is configured.
var defaultTitleTemplate = template.Must(template.New("title").Parse("Update Go to {{.Number}}"))

// defaultBodyTemplate renders the built-in emitted-task body when no
// TASK_BODY_TEMPLATE override is configured.
var defaultBodyTemplate = template.Must(template.New("body").Parse(
	"# Update Go to {{.Number}}\n\n" +
		"Go {{.Number}} released ({{.ReleaseKind}}). Run [[Go - Update Version]] across bborbe Go repos.\n" +
		"- Release notes: {{.ReleaseNotesURL}}\n" +
		"- Downloads: {{.DownloadsURL}}\n",
))

// BuildCreateCommand assembles the CreateTaskCommand for a new Go version.
// newVersion and previousVersion are canonical go-version strings (e.g.
// "go1.27.0"); releaseKind is "minor" or "patch". The title and body are
// rendered from cfg.TitleTemplate / cfg.BodyTemplate (or the package defaults
// when nil); any template-execution failure is returned as a wrapped error.
func BuildCreateCommand(
	ctx context.Context,
	newVersion string,
	previousVersion string,
	releaseKind string,
	cfg TaskConfig,
) (task.CreateCommand, error) {
	taskIDStr := DeriveTaskID(newVersion).String()
	data := taskTemplateData{
		Version:         newVersion,
		Number:          strings.TrimPrefix(newVersion, "go"),
		ReleaseKind:     releaseKind,
		PreviousVersion: previousVersion,
		ReleaseNotesURL: releaseNotesBaseURL + newVersion,
		DownloadsURL:    downloadsURL,
	}

	renderedTitle, err := renderTemplate(ctx, cfg.TitleTemplate, defaultTitleTemplate, data)
	if err != nil {
		return task.CreateCommand{}, errors.Wrapf(ctx, err, "render task title")
	}
	title := applySuffix(renderedTitle, cfg.Suffix)

	body, err := renderTemplate(ctx, cfg.BodyTemplate, defaultBodyTemplate, data)
	if err != nil {
		return task.CreateCommand{}, errors.Wrapf(ctx, err, "render task body")
	}

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
		Body: body,
	}, nil
}

// renderTemplate executes tmpl (or fallback when tmpl is nil) against data and
// returns the rendered string, wrapping any execution error.
func renderTemplate(
	ctx context.Context,
	tmpl *template.Template,
	fallback *template.Template,
	data taskTemplateData,
) (string, error) {
	if tmpl == nil {
		tmpl = fallback
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", errors.Wrapf(ctx, err, "execute template %q", tmpl.Name())
	}
	return buf.String(), nil
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

// applySuffix appends " - <suffix>" to the rendered title when suffix is
// non-empty (feeding both the title frontmatter and the derived filename).
func applySuffix(title string, suffix string) string {
	if suffix != "" {
		return title + " - " + suffix
	}
	return title
}
