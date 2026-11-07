//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func TestNativeIndexRepairActualOwnersNoFlushNoSourceMutation(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	before := digest(t, root)
	m := open(t, root, getDocOptions())
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		queueGetDoc(t, l)
		if e := l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.Main)
			tree, _ := bundle.Database(indexdb.BlockTree)
			updated := "20261028103000"
			if l == a {
				updated = "20261028100000"
			}
			if _, e := db.Exec("INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('same','same','d',?,'','','','','','','','','','','','',0,'','',0,'')", updated); e != nil {
				return e
			}
			if _, e := tree.Exec("INSERT INTO blocktrees(id,root_id,type,updated,parent_id,box_id,path,hpath) VALUES('same','same','d','20261028103000','','','','')"); e != nil {
				return e
			}
			if _, e := db.Exec("INSERT INTO blocks_fts_case_insensitive(id,root_id) VALUES('dup','same'),('dup','same')"); e != nil {
				return e
			}
			if l == a {
				_, e := db.Exec("INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('orphan','orphan','d','20261028100000','','','','','','','','','','','','',0,'','',0,'')")
				return e
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	pa, e := a.InspectNativeIndexRepair(ctx)
	if e != nil || !reflect.DeepEqual(pa.ReindexRoots, []string{docID, getDocOther, "same"}) || !reflect.DeepEqual(pa.RemoveRoots, []string{"orphan"}) || !reflect.DeepEqual(pa.DuplicateRoots, []string{"same"}) {
		t.Fatal(pa, e)
	}
	pb, e := b.InspectNativeIndexRepair(ctx)
	if e != nil || !reflect.DeepEqual(pb, indexrepair.Plan{ReindexRoots: []string{docID, getDocOther}}) {
		t.Fatal("cross owner/case preference", pb, e)
	}
	if a.workspace.resources.contentQueue.Pending() != 2 || b.workspace.resources.contentQueue.Pending() != 2 {
		t.Fatal("diagnostic consumed pending work")
	}
	if digest(t, root) != before {
		t.Fatal("diagnostic changed source")
	}
	pa.ReindexRoots[0] = "mutated"
	again, e := a.InspectNativeIndexRepair(ctx)
	if e != nil || again.ReindexRoots[0] != docID {
		t.Fatal("aliased result", again, e)
	}
	recordCloudBusinessEvidence(t, "repair-native.json", map[string]interface{}{"owners": 2, "sameIDIsolated": true, "casePreferenceIsolated": true, "pendingQueueUnchanged": true, "sourceUnchanged": true, "detachedPlans": true})
}
func TestNativeIndexRepairCanceledClosedAndStaleLease(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if p, e := l.InspectNativeIndexRepair(cancelCtx); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(p, indexrepair.Plan{}) {
		t.Fatal(p, e)
	}
	<-l.workspace.resources.gate
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	p, e := l.InspectNativeIndexRepair(short)
	stop()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) || !reflect.DeepEqual(p, indexrepair.Plan{}) {
		t.Fatal(p, e)
	}
	var tree *sql.DB
	if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error { var e error; tree, e = b.Database(indexdb.BlockTree); return e }); e != nil {
		t.Fatal(e)
	}
	tree.Close()
	if p, e = l.InspectNativeIndexRepair(ctx); e == nil || !reflect.DeepEqual(p, indexrepair.Plan{}) {
		t.Fatal("closed DB became a plan", p, e)
	}
	l.Release()
	if _, e = l.InspectNativeIndexRepair(ctx); e == nil {
		t.Fatal("released lease admitted")
	}
}
