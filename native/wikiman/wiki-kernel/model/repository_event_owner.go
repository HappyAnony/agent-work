// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

const legacyRepoEventSessionKey = "wiki-kernel.repository-event-session"

type legacyRepoEventSession struct {
	owner                                   *AppConf
	workspace, data, repo, history, temp    string
	lang                                    string
	cloudRegion                             int
	indexWalk, checkoutWalk, checkoutUpsert atomic.Int64
	bootProgress                            atomic.Int32
}

func captureLegacyRepoEventSession(ctx context.Context) (*legacyRepoEventSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &legacyRepoEventSession{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, repo: util.RepoDir, history: util.HistoryDir, temp: util.TempDir}
	if s.owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := s.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		s.lang, s.cloudRegion = v.Lang, v.CloudRegion
		return nil
	}); err != nil {
		return nil, err
	}
	if !s.current() {
		return nil, conf.ErrBusinessUnbound
	}
	return s, nil
}

func (s *legacyRepoEventSession) current() bool {
	return s != nil && s.owner != nil && s.owner == Conf && s.workspace == util.WorkspaceDir && s.data == util.DataDir && s.repo == util.RepoDir && s.history == util.HistoryDir && s.temp == util.TempDir
}

func (s *legacyRepoEventSession) payload(push int) map[string]interface{} {
	return map[string]interface{}{eventbus.CtxPushMsg: push, legacyRepoEventSessionKey: s}
}

func captureLegacyRepoEventPayload(ctx context.Context, push int) (*legacyRepoEventSession, map[string]interface{}, error) {
	s, err := captureLegacyRepoEventSession(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s, s.payload(push), nil
}

func (s *legacyRepoEventSession) bind(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload[legacyRepoEventSessionKey] = s
	return payload
}

func (s *legacyRepoEventSession) language(code int, html bool) string {
	return s.owner.business.Language(s.lang, code, html, s.cloudRegion)
}

// publish admits the callback under the same owner gate as repository work.
// CloseCloudRepo can mark the owner closed without blocking on this mutex, then
// joins callbacks that were accepted before close through repoTasks.
func (s *legacyRepoEventSession) publish(fn func(*legacyRepoEventSession)) {
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
	accepted := s.owner.repoTasks.Go(func(context.Context) {
		defer close(done)
		defer logging.Recover()
		fn(s)
	})
	s.owner.repoMu.Unlock()
	if accepted {
		<-done
	}
}

func legacyRepoEventSessionFrom(payload map[string]interface{}) *legacyRepoEventSession {
	if payload == nil {
		return nil
	}
	s, _ := payload[legacyRepoEventSessionKey].(*legacyRepoEventSession)
	return s
}

func legacyRepoEventLanguage(payload map[string]interface{}, code int, html bool) string {
	s := legacyRepoEventSessionFrom(payload)
	if s == nil {
		return ""
	}
	return s.language(code, html)
}

func legacyRepoEventCount(payload map[string]interface{}, kind string) int64 {
	s := legacyRepoEventSessionFrom(payload)
	if s == nil {
		return 0
	}
	switch kind {
	case "index-walk":
		return s.indexWalk.Add(1) - 1
	case "checkout-walk":
		return s.checkoutWalk.Add(1) - 1
	case "checkout-upsert":
		return s.checkoutUpsert.Add(1) - 1
	default:
		return 0
	}
}

func legacyRepoSetProgressPart(payload map[string]interface{}, total int) {
	if s := legacyRepoEventSessionFrom(payload); s != nil {
		s.bootProgress.Store(legacyRepoProgressPart(total))
	}
}

func legacyRepoEventProgressPart(payload map[string]interface{}) int32 {
	if s := legacyRepoEventSessionFrom(payload); s != nil {
		return s.bootProgress.Load()
	}
	return 0
}

func withLegacyRepoEvent(payload map[string]interface{}, fn func(*legacyRepoEventSession)) {
	legacyRepoEventSessionFrom(payload).publish(fn)
}

var legacyRepoActiveSessions = struct {
	sync.Mutex
	counts map[*legacyRepoEventSession]int
}{counts: map[*legacyRepoEventSession]int{}}

func (s *legacyRepoEventSession) activate() func() {
	legacyRepoActiveSessions.Lock()
	legacyRepoActiveSessions.counts[s]++
	legacyRepoActiveSessions.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			legacyRepoActiveSessions.Lock()
			legacyRepoActiveSessions.counts[s]--
			if legacyRepoActiveSessions.counts[s] == 0 {
				delete(legacyRepoActiveSessions.counts, s)
			}
			legacyRepoActiveSessions.Unlock()
		})
	}
}

func withActiveLegacyRepoEvent(fn func(*legacyRepoEventSession)) {
	legacyRepoActiveSessions.Lock()
	var selected *legacyRepoEventSession
	for session := range legacyRepoActiveSessions.counts {
		if selected != nil && selected != session {
			selected = nil
			break
		}
		selected = session
	}
	legacyRepoActiveSessions.Unlock()
	selected.publish(fn)
}

func subscribeLegacyRepoEvent(topic string, fn interface{}) {
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func || value.Type().NumIn() == 0 || value.Type().In(0) != reflect.TypeOf(map[string]interface{}{}) {
		legacyEventRegistrationError = errors.Join(legacyEventRegistrationError, errors.New("legacy_repo_event_signature: "+topic))
		return
	}
	wrapper := reflect.MakeFunc(value.Type(), func(args []reflect.Value) []reflect.Value {
		payload, _ := args[0].Interface().(map[string]interface{})
		withLegacyRepoEvent(payload, func(*legacyRepoEventSession) { value.Call(args) })
		return nil
	})
	subscribeLegacyEvent(topic, wrapper.Interface())
}

func subscribeActiveLegacyRepoEvent(topic string, fn func(*legacyRepoEventSession)) {
	subscribeLegacyEvent(topic, func() {
		withActiveLegacyRepoEvent(fn)
	})
}

func legacyRepoProgressPart(total int) int32 {
	if total <= 0 {
		return 0
	}
	return int32(10 / float64(total))
}

func legacyRepoShortID(id string) string {
	if len(id) <= 7 {
		return id
	}
	return id[:7]
}
