// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"errors"

	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/notification/command/notification"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/go-version-watcher/mocks"
	"github.com/bborbe/go-version-watcher/pkg"
)

type fakeCreateCommandSender struct {
	sendErr      error
	capturedCmds []task.CreateCommand
}

func (f *fakeCreateCommandSender) SendCommand(_ context.Context, cmd task.CreateCommand) error {
	f.capturedCmds = append(f.capturedCmds, cmd)
	return f.sendErr
}

var _ = Describe("pkg.TaskPublisher", func() {
	var cmd task.CreateCommand

	BeforeEach(func() {
		var err error
		cmd, err = pkg.BuildCreateCommand(
			context.Background(), "go1.27.0", "go1.26.5", "minor", pkg.TaskConfig{Stage: "dev"},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns true and calls IncPublished(\"create\") on send success", func() {
		fakeSender := &fakeCreateCommandSender{}
		fakeMetrics := new(mocks.Metrics)
		publisher := pkg.NewTaskPublisher(fakeSender, fakeMetrics)

		result := publisher.PublishCreate(context.Background(), cmd)

		Expect(result).To(BeTrue())
		Expect(fakeMetrics.IncPublishedCallCount()).To(Equal(1))
		Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("create"))
		Expect(fakeSender.capturedCmds).To(HaveLen(1))
		Expect(fakeSender.capturedCmds[0].Frontmatter["task_type"]).To(Equal("go-version-update"))
	})

	It("returns false and calls IncPublished(\"error\") on send failure", func() {
		fakeSender := &fakeCreateCommandSender{sendErr: errors.New("kafka send failed")}
		fakeMetrics := new(mocks.Metrics)
		publisher := pkg.NewTaskPublisher(fakeSender, fakeMetrics)

		result := publisher.PublishCreate(context.Background(), cmd)

		Expect(result).To(BeFalse())
		Expect(fakeMetrics.IncPublishedCallCount()).To(Equal(1))
		Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("error"))
	})
})

type fakeNotificationPublishCommandSender struct {
	sendErr      error
	capturedCmds []notification.NotificationPublishCommand
}

func (f *fakeNotificationPublishCommandSender) SendPublishNotificationCommand(
	_ context.Context,
	cmd notification.NotificationPublishCommand,
) error {
	f.capturedCmds = append(f.capturedCmds, cmd)
	return f.sendErr
}

var _ = Describe("pkg.NotificationPublisher", func() {
	var cmd notification.NotificationPublishCommand

	BeforeEach(func() {
		var err error
		cmd, err = pkg.BuildNotificationCommand(
			context.Background(),
			"go1.27.0",
			"go1.26.5",
			"minor",
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns true and calls IncPublished(\"notification\") on send success", func() {
		fakeSender := &fakeNotificationPublishCommandSender{}
		fakeMetrics := new(mocks.Metrics)
		publisher := pkg.NewNotificationPublisher(fakeSender, fakeMetrics)

		result := publisher.PublishNotification(context.Background(), cmd)

		Expect(result).To(BeTrue())
		Expect(fakeMetrics.IncPublishedCallCount()).To(Equal(1))
		Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("notification"))
		Expect(fakeSender.capturedCmds).To(HaveLen(1))
		Expect(fakeSender.capturedCmds[0].Type).To(Equal(cmd.Type))
		Expect(fakeSender.capturedCmds[0].Message).To(Equal(cmd.Message))
	})

	It("returns false and calls IncPublished(\"notification_error\") on send failure", func() {
		fakeSender := &fakeNotificationPublishCommandSender{
			sendErr: errors.New("kafka send failed"),
		}
		fakeMetrics := new(mocks.Metrics)
		publisher := pkg.NewNotificationPublisher(fakeSender, fakeMetrics)

		result := publisher.PublishNotification(context.Background(), cmd)

		Expect(result).To(BeFalse())
		Expect(fakeMetrics.IncPublishedCallCount()).To(Equal(1))
		Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("notification_error"))
	})
})
