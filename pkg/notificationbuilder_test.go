// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"

	core "github.com/bborbe/notification"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/go-version-watcher/pkg"
)

var _ = Describe("pkg.BuildNotificationCommand", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("builds a go-release command with version + release-notes URL in the message", func() {
		cmd, err := pkg.BuildNotificationCommand(ctx, "go1.27.0", "go1.26.5", "minor")
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Type).To(Equal(core.GoReleaseNotificationType))
		Expect(cmd.Message.String()).
			To(Equal("Go 1.27.0 released (minor) - https://go.dev/doc/devel/release#go1.27.0"))
		Expect(cmd.Metadata["source"]).To(Equal("go-version-watcher"))
		Expect(cmd.Metadata["version"]).To(Equal("go1.27.0"))
		Expect(cmd.Metadata["previous_version"]).To(Equal("go1.26.5"))
		Expect(cmd.Metadata["release_kind"]).To(Equal("minor"))
		Expect(cmd.Metadata["release_notes_url"]).
			To(Equal("https://go.dev/doc/devel/release#go1.27.0"))
	})

	It("passes validation (go-release type is registered in the core)", func() {
		cmd, err := pkg.BuildNotificationCommand(ctx, "go1.27.0", "go1.26.5", "minor")
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Validate(ctx)).To(Succeed())
	})

	It("classifies a patch bump in the message", func() {
		cmd, err := pkg.BuildNotificationCommand(ctx, "go1.26.5", "go1.26.4", "patch")
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Message.String()).
			To(Equal("Go 1.26.5 released (patch) - https://go.dev/doc/devel/release#go1.26.5"))
	})
})
