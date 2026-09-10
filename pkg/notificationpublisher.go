// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"

	"github.com/bborbe/notification/command/notification"
	"github.com/golang/glog"
)

//counterfeiter:generate -o ../mocks/notification_publisher.go --fake-name NotificationPublisher . NotificationPublisher

// NotificationPublisher sends a pre-built NotificationPublishCommand via the
// supplied NotificationPublishCommandSender. Returns true on successful send,
// false on error.
type NotificationPublisher interface {
	PublishNotification(ctx context.Context, cmd notification.NotificationPublishCommand) bool
}

// NewNotificationPublisher returns a NotificationPublisher that wraps the given
// sender + metrics.
func NewNotificationPublisher(
	sender notification.NotificationPublishCommandSender,
	metrics Metrics,
) NotificationPublisher {
	return &notificationPublisher{sender: sender, metrics: metrics}
}

type notificationPublisher struct {
	sender  notification.NotificationPublishCommandSender
	metrics Metrics
}

func (p *notificationPublisher) PublishNotification(
	ctx context.Context,
	cmd notification.NotificationPublishCommand,
) bool {
	if err := p.sender.SendPublishNotificationCommand(ctx, cmd); err != nil {
		glog.Errorf(
			"publish go-release notification failed type=%s err=%v",
			cmd.Type,
			err,
		)
		p.metrics.IncPublished("notification_error")
		return false
	}
	glog.V(2).Infof(
		"published notification %s %s",
		cmd.Type,
		cmd.Metadata["version"],
	)
	p.metrics.IncPublished("notification")
	return true
}
