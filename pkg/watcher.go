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
func NewWatcher(
	client GoDevClient,
	publisher TaskPublisher,
	metrics Metrics,
	cursorPath string,
	cfg TaskConfig,
) Watcher {
	return &watcher{
		client:     client,
		publisher:  publisher,
		metrics:    metrics,
		cursorPath: cursorPath,
		cfg:        cfg,
	}
}

type watcher struct {
	client     GoDevClient
	publisher  TaskPublisher
	metrics    Metrics
	cursorPath string
	cfg        TaskConfig
}

// Poll implements Watcher. One cycle:
//  1. Load cursor (cold-start safe).
//  2. Query go.dev for the max stable version — on error hold the cursor,
//     record go_dev_error, return nil.
//  3. Cold start (empty cursor): seed cursor to latest, emit nothing.
//  4. latest > cursor: build + publish one task; advance cursor on publish success.
//  5. latest <= cursor: record version_unchanged (no task).
//  6. Record success.
func (w *watcher) Poll(ctx context.Context) error {
	cursor, err := LoadCursor(ctx, w.cursorPath)
	if err != nil {
		return errors.Wrapf(ctx, err, "load cursor path=%s", w.cursorPath)
	}

	latest, err := w.client.LatestStable(ctx)
	if err != nil {
		w.metrics.IncPollCycle("go_dev_error")
		glog.Warningf("poll cycle aborted: go.dev query failed err=%v", err)
		return nil
	}

	if cursor.LastSeenVersion == "" {
		w.seedCursor(ctx, cursor, latest)
		w.metrics.IncPollCycle("success")
		return nil
	}

	previous, err := ParseVersion(ctx, cursor.LastSeenVersion)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse stored cursor version %q", cursor.LastSeenVersion)
	}

	if previous.Less(latest) {
		w.emit(ctx, cursor, previous, latest)
	} else {
		w.metrics.IncFilterSkipped("version_unchanged")
		glog.V(2).Infof("version unchanged latest=%s cursor=%s", latest, previous)
	}

	w.metrics.IncPollCycle("success")
	return nil
}

// seedCursor records the current max stable version on cold start without
// emitting a task (avoids a spurious update task on first run).
func (w *watcher) seedCursor(ctx context.Context, cursor *Cursor, latest Version) {
	cursor.LastSeenVersion = latest.String()
	if err := SaveCursor(ctx, w.cursorPath, cursor); err != nil {
		glog.Warningf("save cursor failed on cold-start path=%s err=%v", w.cursorPath, err)
	}
	glog.V(2).Infof("cold-start seed version=%s", latest)
}

// emit builds and publishes one task for the advance from previous to latest,
// advancing the cursor only when the publish succeeds.
func (w *watcher) emit(ctx context.Context, cursor *Cursor, previous, latest Version) {
	releaseKind := "patch"
	if latest.Major != previous.Major || latest.Minor != previous.Minor {
		releaseKind = "minor"
	}
	cmd := BuildCreateCommand(latest.String(), previous.String(), releaseKind, w.cfg)
	if !w.publisher.PublishCreate(ctx, cmd) {
		return
	}
	cursor.LastSeenVersion = latest.String()
	if err := SaveCursor(ctx, w.cursorPath, cursor); err != nil {
		// Task was already published; controller dedup absorbs re-emit next cycle.
		glog.Warningf("save cursor failed post-publish path=%s err=%v", w.cursorPath, err)
	}
}
