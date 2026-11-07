//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	"testing"

	"github.com/siyuan-note/eventbus"
	"wiki-kernel/internal/eventscope"
)

func TestMainQueuePublishesAdmissionOwnerOnChangedAndFlushed(t *testing.T) {
	mainQueueFixture(t)
	token := &struct{ name string }{"owner"}
	SetLegacySQLEventOwnerProvider(func() interface{} { return token })
	t.Cleanup(func() { SetLegacySQLEventOwnerProvider(nil) })

	bridge := eventscope.NewBridge(eventbus.Subscribe)
	scope := bridge.Open()
	t.Cleanup(func() { _ = scope.Close(context.Background()) })
	changed, flushed := make(chan interface{}, 1), make(chan interface{}, 1)
	if err := scope.Subscribe(eventbus.EvtSQLIndexChanged, func(payload map[string]interface{}) {
		changed <- LegacySQLEventOwner(payload)
	}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Subscribe(eventbus.EvtSQLIndexFlushed, func(payload map[string]interface{}) {
		flushed <- LegacySQLEventOwner(payload)
	}); err != nil {
		t.Fatal(err)
	}

	UpdateBlockContentQueue(&Block{ID: "a", Content: "owned"})
	if got := <-changed; got != token {
		t.Fatal("index-changed owner mismatch", got)
	}
	if err := FlushQueueContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-flushed; got != token {
		t.Fatal("index-flushed owner mismatch", got)
	}
}
