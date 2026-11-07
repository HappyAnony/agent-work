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
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestSQLEventSessionFreezesOwnerPolicyAndJoinsCallback(t *testing.T) {
	c, _, _ := repoMetadataFixture(t)
	c.business = conf.NewBusiness(nil, map[string]map[int]string{
		"en_US": {90: "owned %d %d %d %s"},
		"zh_CN": {90: "replacement %d %d %d %s"},
	}, nil)
	c.Lang = "en_US"
	oldInsert, oldDelete := pushSQLInsertBlocksFTSMsg.Load(), pushSQLDeleteBlocksMsg.Load()
	pushSQLInsertBlocksFTSMsg.Store(true)
	pushSQLDeleteBlocksMsg.Store(true)
	t.Cleanup(func() {
		pushSQLInsertBlocksFTSMsg.Store(oldInsert)
		pushSQLDeleteBlocksMsg.Store(oldDelete)
	})
	session, err := captureLegacySQLEventSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.Lang = "zh_CN"
	pushSQLInsertBlocksFTSMsg.Store(false)
	pushSQLDeleteBlocksMsg.Store(false)
	if session.language(90) != "owned %d %d %d %s" || !session.policy.InsertBlocks || !session.policy.DeleteBlocks {
		t.Fatal("SQL event projection was not frozen at admission")
	}
	payload := sql.LegacySQLEventContext(session, map[string]interface{}{"current": 1, "total": 2})
	var calls atomic.Int32
	withLegacySQLEvent(payload, func(context.Context, *legacySQLEventSession) { calls.Add(1) })
	if calls.Load() != 1 {
		t.Fatal("captured SQL event was not admitted")
	}
	old := Conf
	Conf = NewAppConf()
	withLegacySQLEvent(payload, func(context.Context, *legacySQLEventSession) { calls.Add(1) })
	Conf = old
	if calls.Load() != 1 {
		t.Fatal("replacement workspace borrowed SQL event")
	}

	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		withLegacySQLEvent(payload, func(context.Context, *legacySQLEventSession) {
			close(entered)
			<-release
		})
		close(done)
	}()
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = c.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("SQL event owner reported a false callback join", err)
	}
	close(release)
	<-done
	if err = c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLEventSubscriptionsRequireTokenAndPreserveIndexState(t *testing.T) {
	c, _, _ := repoMetadataFixture(t)
	handlers := repositoryEventScopeFixture(t)
	subscribeSQLEvents()
	subscribeSQLHistoryEvents()
	subscribeSQLAssetContentEvents()
	if legacyEventRegistrationError != nil || len(handlers) != 9 {
		t.Fatal("SQL subscriptions incomplete", len(handlers), legacyEventRegistrationError)
	}
	session, err := captureLegacySQLEventSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	changed := handlers[eventbus.EvtSQLIndexChanged].(func(map[string]interface{}))
	flushed := handlers[eventbus.EvtSQLIndexFlushed].(func(map[string]interface{}))
	changed(map[string]interface{}{})
	if c.DataIndexState != 0 {
		t.Fatal("untagged index event changed owner state")
	}
	payload := sql.LegacySQLEventContext(session, nil)
	changed(payload)
	if c.DataIndexState != 1 {
		t.Fatal("owned index change was not recorded")
	}
	flushed(payload)
	if c.DataIndexState != 0 {
		t.Fatal("owned index flush was not recorded")
	}
	if _, ok := handlers[util.EvtSQLHistoryRebuild].(func(map[string]interface{})); !ok {
		t.Fatal("history rebuild did not require owner payload")
	}
	if _, ok := handlers[util.EvtSQLAssetContentRebuild].(func(map[string]interface{})); !ok {
		t.Fatal("asset rebuild did not require owner payload")
	}
}
