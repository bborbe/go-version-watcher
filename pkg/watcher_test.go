// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/go-version-watcher/mocks"
	"github.com/bborbe/go-version-watcher/pkg"
)

var _ = Describe("pkg.Watcher.Poll", func() {
	var (
		ctx                   context.Context
		client                *mocks.GoDevClient
		imageChecker          *mocks.ImageChecker
		publisher             *mocks.TaskPublisher
		notificationPublisher *mocks.NotificationPublisher
		metrics               *mocks.Metrics
		cursorPath            string
		tmpDir                string
		w                     pkg.Watcher
	)

	mustVersion := func(s string) pkg.Version {
		v, err := pkg.ParseVersion(ctx, s)
		Expect(err).NotTo(HaveOccurred())
		return v
	}

	filterSkipReasons := func() []string {
		reasons := make([]string, 0, metrics.IncFilterSkippedCallCount())
		for i := 0; i < metrics.IncFilterSkippedCallCount(); i++ {
			reasons = append(reasons, metrics.IncFilterSkippedArgsForCall(i))
		}
		return reasons
	}

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "watcher-poll-*")
		Expect(err).NotTo(HaveOccurred())
		cursorPath = filepath.Join(tmpDir, "cursor.json")

		client = &mocks.GoDevClient{}
		imageChecker = &mocks.ImageChecker{}
		publisher = &mocks.TaskPublisher{}
		notificationPublisher = &mocks.NotificationPublisher{}
		notificationPublisher.PublishNotificationReturns(true)
		metrics = &mocks.Metrics{}
		w = pkg.NewWatcher(
			client,
			imageChecker,
			publisher,
			notificationPublisher,
			metrics,
			cursorPath,
			pkg.TaskConfig{Stage: "prod"},
			"",
		)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir) // #nosec G104 -- best-effort temp dir cleanup
	})

	Context("stateless emission", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.26.5"), nil)
			client.SecondLatestStableReturns(mustVersion("go1.26.4"), nil)
			imageChecker.ImageExistsReturns(true, nil)
			publisher.PublishCreateReturns(true)
		})

		It("emits one task for latest with no cursor on disk", func() {
			Expect(w.Poll(ctx)).To(Succeed())

			Expect(publisher.PublishCreateCallCount()).To(Equal(1))
			_, cmd := publisher.PublishCreateArgsForCall(0)
			Expect(cmd.Frontmatter["new_version"]).To(Equal("go1.26.5"))
			Expect(cmd.Frontmatter["previous_version"]).To(Equal("go1.26.4"))
			Expect(cmd.Frontmatter["release_kind"]).To(Equal("patch"))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("success"))
		})

		It("stamps recurring: so the controller holds a terminal task in its slot", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			_, cmd := publisher.PublishCreateArgsForCall(0)
			Expect(cmd.Frontmatter["recurring"]).To(Equal("go-version:go1.26.5"))
		})

		It("re-emits on a second poll — dedup belongs to the controller, not a cursor", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(publisher.PublishCreateCallCount()).To(Equal(2))
		})

		It("publishes one go-release notification for the same event", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(notificationPublisher.PublishNotificationCallCount()).To(Equal(1))
			_, ncmd := notificationPublisher.PublishNotificationArgsForCall(0)
			Expect(ncmd.Type.String()).To(Equal("go-release"))
			Expect(ncmd.Metadata["version"]).To(Equal("go1.26.5"))
			Expect(ncmd.Metadata["release_kind"]).To(Equal("patch"))
		})
	})

	Context("new minor version", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.27.0"), nil)
			client.SecondLatestStableReturns(mustVersion("go1.26.5"), nil)
			imageChecker.ImageExistsReturns(true, nil)
			publisher.PublishCreateReturns(true)
		})

		It("classifies the bump as minor", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			_, cmd := publisher.PublishCreateArgsForCall(0)
			Expect(cmd.Frontmatter["release_kind"]).To(Equal("minor"))
		})
	})

	Context("image not ready", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.26.5"), nil)
			client.SecondLatestStableReturns(mustVersion("go1.26.4"), nil)
			imageChecker.ImageExistsReturns(false, nil)
		})

		It("emits no task and records image_not_ready", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(publisher.PublishCreateCallCount()).To(Equal(0))
			Expect(filterSkipReasons()).To(ContainElement("image_not_ready"))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("success"))
		})
	})

	Context("go.dev query error", func() {
		BeforeEach(func() {
			client.LatestStableReturns(pkg.Version{}, stderrors.New("network down"))
		})

		It("records go_dev_error and emits nothing", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(publisher.PublishCreateCallCount()).To(Equal(0))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("go_dev_error"))
		})
	})

	Context("second-latest query error", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.26.5"), nil)
			client.SecondLatestStableReturns(pkg.Version{}, stderrors.New("network down"))
		})

		It("records go_dev_error and emits nothing", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(publisher.PublishCreateCallCount()).To(Equal(0))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("go_dev_error"))
		})
	})

	Context("publish returns false", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.26.5"), nil)
			client.SecondLatestStableReturns(mustVersion("go1.26.4"), nil)
			imageChecker.ImageExistsReturns(true, nil)
			publisher.PublishCreateReturns(false)
		})

		It("records success and publishes no notification", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(notificationPublisher.PublishNotificationCallCount()).To(Equal(0))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("success"))
		})
	})

	Context("docker image check errors", func() {
		BeforeEach(func() {
			client.LatestStableReturns(mustVersion("go1.27.0"), nil)
			client.SecondLatestStableReturns(mustVersion("go1.26.5"), nil)
			imageChecker.ImageExistsReturns(false, stderrors.New("docker hub down"))
		})

		It("records image_check_error", func() {
			Expect(w.Poll(ctx)).To(Succeed())
			Expect(publisher.PublishCreateCallCount()).To(Equal(0))
			Expect(metrics.IncPollCycleArgsForCall(0)).To(Equal("image_check_error"))
		})
	})
})
