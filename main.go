// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command go-version-watcher polls https://go.dev/dl/?mode=json on an interval,
// computes the max stable Go version, and publishes one CreateTaskCommand to
// Kafka per new version so the Go update runbook can be run.
//
// See [[Go Version Watcher]] for scope + DoD; [[Watcher Writing Guide]] for the
// producer-side contract; [[Agent Task File Contract]] for the frontmatter/body
// shape this watcher emits.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	"github.com/bborbe/go-version-watcher/pkg"
	"github.com/bborbe/go-version-watcher/pkg/factory"
	libhttp "github.com/bborbe/http"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// httpClientTimeout bounds each go.dev request.
const httpClientTimeout = 30 * time.Second

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen       string           `required:"false" arg:"listen"        env:"LISTEN"        usage:"HTTP listen address (healthz/readiness/metrics)"  default:":9090"`
	Stage        string           `required:"true"  arg:"stage"         env:"STAGE"         usage:"Deployment stage (dev|prod)"`
	PollInterval string           `required:"false" arg:"poll-interval" env:"POLL_INTERVAL" usage:"Poll interval (Go duration)"                      default:"24h"`
	CursorPath   string           `required:"false" arg:"cursor-path"   env:"CURSOR_PATH"   usage:"Cursor persistence path (mount a PVC)"            default:"/data/cursor.json"`
	KafkaBrokers libkafka.Brokers `required:"true"  arg:"kafka-brokers"  env:"KAFKA_BROKERS" usage:"Comma-separated Kafka broker list"`

	// TopicPrefix selects the Kafka topic prefix used for CQRS topic construction
	// (e.g. "develop" / "master"); independent of Stage. Empty means unprefixed topics.
	TopicPrefix base.TopicPrefix `required:"false" arg:"topic-prefix" env:"TOPIC_PREFIX" usage:"Kafka topic prefix for CQRS topic construction"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	pollInterval, err := time.ParseDuration(a.PollInterval)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse poll interval %q", a.PollInterval)
	}

	syncProducer, err := libkafka.NewSyncProducerWithName(ctx, a.KafkaBrokers, "go-version-watcher")
	if err != nil {
		return errors.Wrap(ctx, err, "create sync producer")
	}
	defer func() {
		if cerr := syncProducer.Close(); cerr != nil {
			glog.Warningf("close kafka sync producer: %v", cerr)
		}
	}()

	httpClient := &http.Client{Timeout: httpClientTimeout}
	metrics := pkg.NewMetrics(nil)
	sender := factory.CreateKafkaSender(syncProducer, a.TopicPrefix)
	w := factory.CreateWatcher(httpClient, sender, a.CursorPath, metrics, a.Stage)

	glog.V(2).Infof(
		"go-version-watcher starting stage=%s interval=%s listen=%s",
		a.Stage, a.PollInterval, a.Listen,
	)

	return run.CancelOnFirstFinish(ctx,
		a.pollLoop(w.Poll, pollInterval),
		a.createHTTPServer(),
	)
}

// createHTTPServer serves the mandatory triple (/healthz, /readiness, /metrics).
func (a *application) createHTTPServer() run.Func {
	return func(ctx context.Context) error {
		router := mux.NewRouter()
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		glog.V(2).Infof("http server listening on %s", a.Listen)
		return libhttp.NewServer(a.Listen, router).Run(ctx)
	}
}

// pollLoop fires one cycle immediately on start, then on each interval tick.
func (a *application) pollLoop(poll run.Func, interval time.Duration) run.Func {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		if err := poll(ctx); err != nil {
			glog.Errorf("initial poll: %v", err)
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := poll(ctx); err != nil {
					glog.Errorf("poll: %v", err)
				}
			}
		}
	}
}
