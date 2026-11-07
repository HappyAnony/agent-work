// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"

	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

type legacySQLEventSession struct {
	owner                    *AppConf
	workspace, data, history string
	lang                     string
	cloudRegion              int
	policy                   conf.SQLProgressPolicy
}

func captureLegacySQLEventSession(ctx context.Context) (*legacySQLEventSession, error) {
	if ctx == nil {
		return nil, errors.New("sql_event_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &legacySQLEventSession{
		owner:     Conf,
		workspace: util.WorkspaceDir,
		data:      util.DataDir,
		history:   util.HistoryDir,
		policy: conf.SQLProgressPolicy{
			InsertBlocks: pushSQLInsertBlocksFTSMsg.Load(),
			DeleteBlocks: pushSQLDeleteBlocksMsg.Load(),
			Mobile:       util.ContainerAndroid == util.Container || util.ContainerIOS == util.Container || util.ContainerHarmony == util.Container,
		},
	}
	if s.owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	// Queue admission can occur inside an existing setting snapshot. Read only
	// the two projection fields under the data mutex to avoid re-entering the
	// non-reentrant settings gate.
	s.owner.m.Lock()
	s.lang, s.cloudRegion = s.owner.Lang, s.owner.CloudRegion
	s.owner.m.Unlock()
	if !s.current() {
		return nil, errors.New("sql_event_owner_closed")
	}
	return s, nil
}

func captureLegacySQLEventToken() interface{} {
	s, err := captureLegacySQLEventSession(context.Background())
	if err != nil {
		return nil
	}
	return s
}

func (s *legacySQLEventSession) current() bool {
	return s != nil && s.owner != nil && s.owner == Conf && s.workspace == util.WorkspaceDir && s.data == util.DataDir && s.history == util.HistoryDir
}

func (s *legacySQLEventSession) language(code int) string {
	if s == nil || s.owner == nil || s.owner.business == nil {
		return ""
	}
	return s.owner.business.Language(s.lang, code, true, s.cloudRegion)
}

func (s *legacySQLEventSession) publish(fn func(context.Context, *legacySQLEventSession)) {
	if s == nil || fn == nil || !s.current() {
		return
	}
	s.owner.repoMu.Lock()
	if s.owner.repoClosed || !s.current() {
		s.owner.repoMu.Unlock()
		return
	}
	if s.owner.repoTasks == nil {
		s.owner.repoTasks = taskgroup.Open()
	}
	done := make(chan struct{})
	accepted := s.owner.repoTasks.Go(func(ctx context.Context) {
		defer close(done)
		defer logging.Recover()
		if ctx.Err() == nil && s.current() {
			fn(ctx, s)
		}
	})
	s.owner.repoMu.Unlock()
	if accepted {
		<-done
	}
}

func legacySQLEventSessionFrom(payload map[string]interface{}) *legacySQLEventSession {
	s, _ := sql.LegacySQLEventOwner(payload).(*legacySQLEventSession)
	return s
}

func withLegacySQLEvent(payload map[string]interface{}, fn func(context.Context, *legacySQLEventSession)) {
	legacySQLEventSessionFrom(payload).publish(fn)
}
