// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	stderrors "errors"
	"testing"

	task "github.com/bborbe/agent/command/task"
)

// fakeCompleteSender records the commands it is handed so a test can assert on
// them without a Kafka producer.
type fakeCompleteSender struct {
	calls []task.CompleteCommand
	err   error
}

func (f *fakeCompleteSender) SendCommand(_ context.Context, cmd task.CompleteCommand) error {
	f.calls = append(f.calls, cmd)
	return f.err
}

func TestCompleteCheckTask(t *testing.T) {
	tests := []struct {
		name        string
		taskID      string
		targetVault string
		sendErr     error
		wantCalls   int
		wantErr     bool
		wantID      string
	}{
		{
			name:      "unset TASK_ID is a no-op",
			taskID:    "",
			wantCalls: 0,
			wantErr:   false,
		},
		{
			name:        "set TASK_ID sends one complete command",
			taskID:      "8ece5756-b32f-4fe9-a50b-043e561ca642",
			targetVault: "personal",
			wantCalls:   1,
			wantErr:     false,
			wantID:      "8ece5756-b32f-4fe9-a50b-043e561ca642",
		},
		{
			name:      "a sender error is returned, not swallowed",
			taskID:    "8ece5756-b32f-4fe9-a50b-043e561ca642",
			sendErr:   stderrors.New("kafka down"),
			wantCalls: 1,
			wantErr:   true,
			wantID:    "8ece5756-b32f-4fe9-a50b-043e561ca642",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeCompleteSender{err: tt.sendErr}
			app := &application{TaskID: tt.taskID, TargetVault: tt.targetVault}

			err := app.completeCheckTask(context.Background(), sender)

			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(sender.calls) != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", len(sender.calls), tt.wantCalls)
			}
			if tt.wantCalls > 0 {
				if got := string(sender.calls[0].TaskIdentifier); got != tt.wantID {
					t.Errorf("TaskIdentifier = %q, want %q", got, tt.wantID)
				}
				if sender.calls[0].TargetVault != tt.targetVault {
					t.Errorf(
						"TargetVault = %q, want %q",
						sender.calls[0].TargetVault, tt.targetVault,
					)
				}
			}
		})
	}
}
