// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	stdsql "database/sql"
	"testing"
)

func TestLegacySQLEventOwnerCapturedAtQueueAdmission(t *testing.T) {
	first, second := &struct{ name string }{"first"}, &struct{ name string }{"second"}
	selected := interface{}(first)
	SetLegacySQLEventOwnerProvider(func() interface{} { return selected })
	defer SetLegacySQLEventOwnerProvider(nil)

	owner := &stdsql.DB{}
	oldDB := db
	dbQueueLock.Lock()
	oldQueue := operationQueue
	operationQueue = nil
	dbQueueLock.Unlock()
	db = owner
	t.Cleanup(func() {
		dbQueueLock.Lock()
		operationQueue = oldQueue
		dbQueueLock.Unlock()
		db = oldDB
	})

	UpdateBlockContentQueue(&Block{ID: "same", Content: "first"})
	selected = second
	UpdateBlockContentQueue(&Block{ID: "same", Content: "second"})
	dbQueueLock.Lock()
	if len(operationQueue) != 1 || operationQueue[0].eventOwner != second || operationQueue[0].block.Content != "second" {
		t.Fatal("coalesced operation did not retain latest admission owner")
	}
	dbQueueLock.Unlock()

	payload := LegacySQLEventContext(second, map[string]interface{}{"current": 1})
	if LegacySQLEventOwner(payload) != second || payload["current"] != 1 {
		t.Fatal("event owner payload was not round-tripped")
	}
	if LegacySQLEventOwner(LegacySQLEventContext(nil, payload)) != nil || payload["current"] != 1 {
		t.Fatal("clearing event owner changed existing payload")
	}

	oldHistory, oldAsset := historyDB, assetContentDB
	historyDB, assetContentDB = owner, owner
	t.Cleanup(func() {
		historyQueues.Lock()
		delete(historyQueues.eventOwners, owner)
		historyQueues.Unlock()
		legacyAssetEventOwners.Lock()
		delete(legacyAssetEventOwners.byDB, owner)
		legacyAssetEventOwners.Unlock()
		historyDB, assetContentDB = oldHistory, oldAsset
	})
	selected = first
	firstHistory := CaptureLegacyHistoryQueue()
	firstAsset := CaptureLegacyAssetQueue()
	firstHistory.admitEventOwner()
	firstAsset.admitEventOwner()
	selected = second
	if LegacySQLEventOwner(CaptureLegacyHistoryQueue().EventContext(nil)) != first {
		t.Fatal("history progress borrowed flush-time owner")
	}
	if LegacySQLEventOwner(CaptureLegacyAssetQueue().EventContext(nil)) != first {
		t.Fatal("asset progress borrowed flush-time owner")
	}
}
