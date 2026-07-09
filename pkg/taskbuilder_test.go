// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/go-version-watcher/pkg"
)

var _ = Describe("pkg.BuildCreateCommand", func() {
	It("produces the exact go-version-update frontmatter", func() {
		cmd := pkg.BuildCreateCommand(
			"go1.27.0",
			"go1.26.5",
			"minor",
			pkg.TaskConfig{Stage: "prod"},
		)

		Expect(cmd.Frontmatter["task_type"]).To(Equal("go-version-update"))
		Expect(cmd.Frontmatter["assignee"]).To(Equal("human"))
		Expect(cmd.Frontmatter["phase"]).To(Equal("todo"))
		Expect(cmd.Frontmatter["status"]).To(Equal("in_progress"))
		Expect(cmd.Frontmatter["stage"]).To(Equal("prod"))
		Expect(cmd.Frontmatter["title"]).To(Equal("Update Go to 1.27.0"))
		Expect(cmd.Frontmatter["new_version"]).To(Equal("go1.27.0"))
		Expect(cmd.Frontmatter["previous_version"]).To(Equal("go1.26.5"))
		Expect(cmd.Frontmatter["release_kind"]).To(Equal("minor"))
		Expect(cmd.Frontmatter["release_notes_url"]).
			To(Equal("https://go.dev/doc/devel/release#go1.27.0"))
		Expect(cmd.Frontmatter["task_identifier"]).
			To(Equal(pkg.DeriveTaskID("go1.27.0").String()))
		Expect(string(cmd.TaskIdentifier)).To(Equal(cmd.Frontmatter["task_identifier"]))
		Expect(cmd.Title).To(Equal("Update Go to 1.27.0"))
	})

	It("body is an operator-readable header referencing the runbook and URLs", func() {
		cmd := pkg.BuildCreateCommand("go1.27.0", "go1.26.5", "minor", pkg.TaskConfig{Stage: "dev"})

		Expect(cmd.Body).To(ContainSubstring("Go 1.27.0 released (minor)"))
		Expect(cmd.Body).To(ContainSubstring("[[Go - Update Version]]"))
		Expect(cmd.Body).To(ContainSubstring("https://go.dev/doc/devel/release#go1.27.0"))
		Expect(cmd.Body).To(ContainSubstring("https://go.dev/dl/"))
	})

	It("classifies a patch bump title correctly", func() {
		cmd := pkg.BuildCreateCommand("go1.26.5", "go1.26.4", "patch", pkg.TaskConfig{Stage: "dev"})
		Expect(cmd.Frontmatter["title"]).To(Equal("Update Go to 1.26.5"))
		Expect(cmd.Frontmatter["release_kind"]).To(Equal("patch"))
	})

	It("same inputs produce identical commands", func() {
		cfg := pkg.TaskConfig{Stage: "dev"}
		cmd1 := pkg.BuildCreateCommand("go1.27.0", "go1.26.5", "minor", cfg)
		cmd2 := pkg.BuildCreateCommand("go1.27.0", "go1.26.5", "minor", cfg)
		Expect(cmd1.Frontmatter).To(Equal(cmd2.Frontmatter))
		Expect(cmd1.TaskIdentifier).To(Equal(cmd2.TaskIdentifier))
		Expect(strings.Contains(cmd1.Body, "\n- ")).To(BeTrue()) // header bullets only, not data
	})
})
