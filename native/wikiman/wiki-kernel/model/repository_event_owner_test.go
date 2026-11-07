// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/siyuan-note/eventbus"
	"wiki-kernel/conf"
	"wiki-kernel/internal/eventscope"
)

func repositoryEventScopeFixture(t *testing.T) map[string]interface{} {
	t.Helper()
	handlers := map[string]interface{}{}
	bridge := eventscope.NewBridge(func(topic string, fn interface{}) error {
		handlers[topic] = fn
		return nil
	})
	previousScope, previousErr := legacyEventScope, legacyEventRegistrationError
	legacyEventScope, legacyEventRegistrationError = bridge.Open(), nil
	t.Cleanup(func() {
		_ = legacyEventScope.Close(context.Background())
		legacyEventScope, legacyEventRegistrationError = previousScope, previousErr
	})
	return handlers
}

func TestRepositoryEventSessionRejectsForeignReplacementAndJoinsEnteredCallback(t *testing.T) {
	c, _, _ := repoMetadataFixture(t)
	c.business = conf.NewBusiness(nil, map[string]map[int]string{
		"en_US": {158: "owned %s"},
		"zh_CN": {158: "replacement %s"},
	}, nil)
	c.Lang = "en_US"
	handlers := repositoryEventScopeFixture(t)
	session, err := captureLegacyRepoEventSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	subscribeLegacyRepoEvent("test.repository.owner", func(payload map[string]interface{}, block bool) {
		calls.Add(1)
		if block {
			close(entered)
			<-release
		}
	})
	if legacyEventRegistrationError != nil {
		t.Fatal(legacyEventRegistrationError)
	}
	relay := handlers["test.repository.owner"].(func(map[string]interface{}, bool))
	relay(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToNone}, false)
	if calls.Load() != 0 {
		t.Fatal("untagged event admitted")
	}
	relay(session.payload(eventbus.CtxPushMsgToNone), false)
	if calls.Load() != 1 || legacyRepoEventLanguage(session.payload(0), 158, true) != "owned %s" {
		t.Fatal("captured session or language missing", calls.Load())
	}

	old := Conf
	Conf = NewAppConf()
	relay(session.payload(eventbus.CtxPushMsgToNone), false)
	Conf = old
	if calls.Load() != 1 {
		t.Fatal("replacement owner event admitted")
	}

	done := make(chan struct{})
	go func() { relay(session.payload(eventbus.CtxPushMsgToNone), true); close(done) }()
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = c.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("owner reported false callback join", err)
	}
	close(release)
	<-done
	if err = c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryEventSubscriptionsAndSessionLocalState(t *testing.T) {
	_, _, _ = repoMetadataFixture(t)
	handlers := repositoryEventScopeFixture(t)
	subscribeRepoEvents()
	if legacyEventRegistrationError != nil || len(handlers) != 39 {
		t.Fatal("repository subscriptions incomplete", len(handlers), legacyEventRegistrationError)
	}
	first, err := captureLegacyRepoEventSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := captureLegacyRepoEventSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if legacyRepoEventCount(first.payload(0), "index-walk") != 0 || legacyRepoEventCount(first.payload(0), "index-walk") != 1 || legacyRepoEventCount(second.payload(0), "index-walk") != 0 {
		t.Fatal("progress counters leaked between operations")
	}
	legacyRepoSetProgressPart(first.payload(0), 5)
	if legacyRepoEventProgressPart(first.payload(0)) != 2 || legacyRepoEventProgressPart(second.payload(0)) != 0 {
		t.Fatal("progress part leaked between operations")
	}

	active := 0
	stopFirst := first.activate()
	stopSecond := second.activate()
	withActiveLegacyRepoEvent(func(*legacyRepoEventSession) { active++ })
	stopSecond()
	withActiveLegacyRepoEvent(func(s *legacyRepoEventSession) {
		if s != first {
			t.Fatal("wrong active session")
		}
		active++
	})
	stopFirst()
	if active != 1 {
		t.Fatal("ambiguous context-free event admitted", active)
	}
}
