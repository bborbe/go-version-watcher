// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

//counterfeiter:generate -o ../mocks/watcher.go --fake-name Watcher . Watcher

// Watcher polls go.dev for the max stable Go version and publishes a
// CreateTaskCommand when it advances beyond the cursor.
type Watcher interface {
	// Poll runs one scan cycle. Safe to call repeatedly on an interval.
	Poll(ctx context.Context) error
}

// NewWatcher wires the watcher's collaborators.
//
// seedVersion, when non-empty, is the Go version the cursor is seeded with on
// cold start instead of the current latest, so the first poll can emit a task
// for the current latest. Empty means seed to latest and emit nothing.
//
// imageChecker gates task emission on docker.io actually publishing the golang
// image for the new version (go.dev releases 12h–1day before the image exists).
func NewWatcher(
	client GoDevClient,
	imageChecker ImageChecker,
	publisher TaskPublisher,
	notificationPublisher NotificationPublisher,
	metrics Metrics,
	cursorPath string,
	cfg TaskConfig,
	seedVersion string,
) Watcher {
	return &watcher{
		client:                client,
		imageChecker:          imageChecker,
		publisher:             publisher,
		notificationPublisher: notificationPublisher,
		metrics:               metrics,
		cursorPath:            cursorPath,
		cfg:                   cfg,
		seedVersion:           seedVersion,
	}
}

type watcher struct {
	client                GoDevClient
	imageChecker          ImageChecker
	publisher             TaskPublisher
	notificationPublisher NotificationPublisher
	metrics               Metrics
	cursorPath            string
	cfg                   TaskConfig
	seedVersion           string
}

// Poll implements Watcher. One stateless cycle:
//  1. Query go.dev for the max stable version — on error record go_dev_error,
//     return nil.
//  2. Query go.dev for the second-max stable version, which supplies
//     previous_version and release_kind. A Job has no PVC and therefore no
//     cursor to diff against, so the diff comes from the upstream list itself.
//  3. Check the docker image for latest exists — if not yet published, record
//     image_not_ready and retry next cycle.
//  4. Build + publish one task carrying a `recurring:` marker. Emission is
//     unconditional: the controller holds a terminal recurring instance in its
//     slot (isRecurringTaskCommand → ReopenTerminal: false) instead of
//     re-materializing it, so a completed update task is never re-filed.
func (w *watcher) Poll(ctx context.Context) error {
	latest, err := w.client.LatestStable(ctx)
	if err != nil {
		w.metrics.IncPollCycle("go_dev_error")
		glog.Warningf("poll cycle aborted: go.dev query failed err=%v", err)
		return nil
	}

	previous, err := w.client.SecondLatestStable(ctx)
	if err != nil {
		w.metrics.IncPollCycle("go_dev_error")
		glog.Warningf("poll cycle aborted: second-latest query failed err=%v", err)
		return nil
	}

	if err := w.emitWhenImageReady(ctx, previous, latest); err != nil {
		return err
	}

	w.metrics.IncPollCycle("success")
	return nil
}

// emitWhenImageReady gates task emission on docker.io having published the
// image for latest. When the image is not yet available (go.dev releases 12h–
// 1day before the image exists) the cursor is held and the next cycle retries.
func (w *watcher) emitWhenImageReady(
	ctx context.Context,
	previous, latest Version,
) error {
	exists, err := w.imageChecker.ImageExists(ctx, latest)
	if err != nil {
		w.metrics.IncPollCycle("image_check_error")
		glog.Warningf("image check failed latest=%s err=%v", latest, err)
		return nil
	}
	if !exists {
		w.metrics.IncFilterSkipped("image_not_ready")
		glog.V(2).
			Infof("image not ready latest=%s previous=%s — holding, retry next cycle", latest, previous)
		w.metrics.IncPollCycle("success")
		return nil
	}
	if err := w.emit(ctx, previous, latest); err != nil {
		w.metrics.IncPollCycle("build_error")
		glog.Errorf("emit task failed latest=%s: %v", latest, err)
		return nil
	}
	return nil
}

// emit builds and publishes one task for the advance from previous to latest.
// A build error (e.g. a template-execution failure) is returned so the caller
// can skip cleanly. No cursor is written: the emitted task carries a
// `recurring:` marker, and the controller — not a pod-local file — is what
// dedupes, so a stateless Job re-emitting every run is a no-op once the task
// exists.
func (w *watcher) emit(ctx context.Context, previous, latest Version) error {
	releaseKind := "patch"
	if latest.Major != previous.Major || latest.Minor != previous.Minor {
		releaseKind = "minor"
	}
	cmd, err := BuildCreateCommand(ctx, latest.String(), previous.String(), releaseKind, w.cfg)
	if err != nil {
		return errors.Wrapf(ctx, err, "build create command for %s", latest)
	}
	if !w.publisher.PublishCreate(ctx, cmd) {
		return nil
	}
	// Publish the go-release notification alongside the task (same event, both
	// channels). Best-effort: the task is the deduplicated backstop and the
	// notification is not part of the dedup contract — a transient failure
	// surfaces via the notification_error metric, and the next run re-arms
	// delivery.
	if ncmd, nerr := BuildNotificationCommand(ctx, latest.String(), previous.String(), releaseKind); nerr != nil {
		glog.Warningf("build notification failed latest=%s err=%v", latest, nerr)
	} else if !w.notificationPublisher.PublishNotification(ctx, ncmd) {
		glog.Warningf("publish notification failed latest=%s", latest)
	}
	return nil
}
