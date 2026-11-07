// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"testing"
)

func TestConfigurationWarningUsesOwnerCancellationAndAdmission(t *testing.T) {
	for _, test := range []struct {
		name      string
		message   string
		wait      func(context.Context) error
		wantStart bool
		wantQueue bool
	}{
		{name: "success", message: "warning", wait: func(context.Context) error { return nil }, wantStart: true, wantQueue: true},
		{name: "cancelled", message: "warning", wait: func(context.Context) error { return context.Canceled }, wantStart: true},
		{name: "empty", wait: func(context.Context) error { return nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			started, queued := false, false
			ctx, cancel := context.WithCancel(context.Background())
			ports := configurationWarningPorts{
				start: func(fn func(context.Context)) bool {
					started = true
					fn(ctx)
					return true
				},
				waitUI: test.wait,
				enqueue: func(_ context.Context, value string) {
					queued = true
					if value != test.message {
						t.Fatalf("unexpected warning %q", value)
					}
				},
			}
			if got := scheduleConfigurationWarning(test.message, ports); got != test.wantStart {
				t.Fatalf("unexpected admission %v", got)
			}
			cancel()
			if started != test.wantStart || queued != test.wantQueue {
				t.Fatalf("started=%v queued=%v", started, queued)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	queued := false
	if !scheduleConfigurationWarning("warning", configurationWarningPorts{
		start:   func(fn func(context.Context)) bool { fn(cancelled); return true },
		waitUI:  func(context.Context) error { return nil },
		enqueue: func(context.Context, string) { queued = true },
	}) || queued {
		t.Fatal("cancelled owner published warning")
	}

	if scheduleConfigurationWarning("warning", configurationWarningPorts{waitUI: func(context.Context) error { return errors.New("fixture") }}) {
		t.Fatal("unbound warning ports were accepted")
	}
}
