// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the go-version-watcher binary.
package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	cqrsiam "github.com/bborbe/cqrs/iam"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/bborbe/notification/command/notification"

	"github.com/bborbe/go-version-watcher/pkg"
)

// CreateKafkaSender constructs a typed create-task command sender backed by a
// Kafka sync producer. defaultVault is the Obsidian vault slug substituted into
// each command's TargetVault when unset (empty = controller default, openclaw).
func CreateKafkaSender(
	syncProducer libkafka.SyncProducer,
	topicPrefix base.TopicPrefix,
	defaultVault string,
) task.CreateCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, topicPrefix, log.DefaultSamplerFactory)
	return task.NewCreateCommandSender(sender, defaultVault)
}

// CreateKafkaNotificationSender constructs a typed notification-publish command
// sender backed by the same Kafka sync producer used for task commands. The
// CDB sender derives the topic ({branch}-core-notification-v1-request) from
// core.NotificationV1SchemaID + topicPrefix.
func CreateKafkaNotificationSender(
	syncProducer libkafka.SyncProducer,
	topicPrefix base.TopicPrefix,
) notification.NotificationPublishCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, topicPrefix, log.DefaultSamplerFactory)
	return notification.NewNotificationPublishCommandSender(
		base.NewCommandCreator(base.RequestIDChannel(context.Background())),
		sender,
		cqrsiam.Initiator("go-version-watcher"),
	)
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
//
// Pure composition — no I/O. The Kafka sync producer and the task sender are
// constructed by the caller (so it controls connection lifecycle + cleanup).
func CreateWatcher(
	httpClient *http.Client,
	sender task.CreateCommandSender,
	notificationSender notification.NotificationPublishCommandSender,
	cursorPath string,
	metrics pkg.Metrics,
	cfg pkg.TaskConfig,
	seedVersion string,
) pkg.Watcher {
	client := pkg.NewGoDevClient(httpClient, pkg.DefaultGoDevURL)
	imageChecker := pkg.NewImageChecker(
		httpClient,
		pkg.DefaultDockerHubTokenURL,
		pkg.DefaultDockerHubRegistryURL,
	)
	publisher := pkg.NewTaskPublisher(sender, metrics)
	notificationPublisher := pkg.NewNotificationPublisher(notificationSender, metrics)
	return pkg.NewWatcher(
		client,
		imageChecker,
		publisher,
		notificationPublisher,
		metrics,
		cursorPath,
		cfg,
		seedVersion,
	)
}
