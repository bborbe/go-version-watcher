// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"strings"

	"github.com/bborbe/errors"
	core "github.com/bborbe/notification"
	"github.com/bborbe/notification/command/notification"
)

// BuildNotificationCommand assembles the NotificationPublishCommand emitted
// alongside the CreateTaskCommand for a new Go version. The message carries the
// version + release-notes URL; per [[Notification Targets]], the message never
// knows where it ends — the controller routes type go-release to Discord.
func BuildNotificationCommand(
	ctx context.Context,
	newVersion string,
	previousVersion string,
	releaseKind string,
) (notification.NotificationPublishCommand, error) {
	cmd := notification.NotificationPublishCommand{
		Type: core.GoReleaseNotificationType,
		Message: core.NotificationMessagef(
			"Go %s released (%s) - %s",
			strings.TrimPrefix(newVersion, "go"),
			releaseKind,
			releaseNotesBaseURL+newVersion,
		),
		Metadata: map[string]string{
			"source":            "go-version-watcher",
			"version":           newVersion,
			"previous_version":  previousVersion,
			"release_kind":      releaseKind,
			"release_notes_url": releaseNotesBaseURL + newVersion,
		},
	}
	if err := cmd.Validate(ctx); err != nil {
		return notification.NotificationPublishCommand{}, errors.Wrapf(
			ctx,
			err,
			"validate notification command",
		)
	}
	return cmd, nil
}
