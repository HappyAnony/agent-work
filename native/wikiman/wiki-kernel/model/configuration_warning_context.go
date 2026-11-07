// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"time"

	"wiki-kernel/task"
	"wiki-kernel/util"
)

type configurationWarningPorts struct {
	start   func(func(context.Context)) bool
	waitUI  func(context.Context) error
	enqueue func(context.Context, string)
}

func scheduleConfigurationWarning(message string, ports configurationWarningPorts) bool {
	if message == "" || ports.start == nil || ports.waitUI == nil || ports.enqueue == nil {
		return false
	}
	return ports.start(func(ctx context.Context) {
		if err := ports.waitUI(ctx); err != nil {
			return
		}
		if err := ctx.Err(); err != nil {
			return
		}
		ports.enqueue(ctx, message)
	})
}

func scheduleLegacyConfigurationWarning(message string) bool {
	owner, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		return false
	}
	return scheduleConfigurationWarning(message, configurationWarningPorts{
		start:  owner.owner.startRepoBackground,
		waitUI: util.WaitForUILoadedContext,
		enqueue: func(ctx context.Context, value string) {
			_ = owner.schedule(ctx, task.PushMsg, 2*time.Second, func() { util.PushErrMsg(value, 15000) }, value, 15000)
		},
	})
}
