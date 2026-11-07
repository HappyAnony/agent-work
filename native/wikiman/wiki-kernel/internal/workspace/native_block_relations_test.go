//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

const relationCode = "20261019090003-cod0001"
const relationGhost = "20261027210000-ghost01"

func relationsSource(t *testing.T) string {
	t.Helper()
	root := positionSource(t)
	file := filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	children := d["Children"].([]interface{})
	var moved interface{}
	var kept []interface{}
	for _, v := range children {
		if v.(map[string]interface{})["ID"] == blockID {
			moved = v
		} else {
			kept = append(kept, v)
		}
	}
	for _, v := range kept {
		n := v.(map[string]interface{})
		if n["ID"] == infoList {
			item := n["Children"].([]interface{})[0].(map[string]interface{})
			item["Children"] = append([]interface{}{moved}, item["Children"].([]interface{})...)
		}
	}
	d["Children"] = kept
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return root
}

func TestNativeBlockRelationsActualDifferentOwnerGraphsAndNoFlush(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		infos, e := l.NativeBlockTreeInfos(ctx, []string{blockID, infoList, docID, getDocDefinition, "missing", blockID})
		if e != nil || len(infos) != 4 || infos[blockID].Type != "NodeParagraph" || infos[docID].Type != "NodeDocument" {
			t.Fatal(infos, e)
		}
		sibling, e := l.NativeBlockSiblingID(ctx, blockID)
		if e != nil {
			t.Fatal(e)
		}
		relevant, e := l.NativeBlockRelevantIDs(ctx, blockID)
		if e != nil {
			t.Fatal(e)
		}
		if l == a {
			if infos[blockID].ParentID != docID || infos[blockID].PreviousID != positionH1 || infos[blockID].NextID != relationCode || sibling != (docquery.RelationIDs{Parent: docID, Previous: positionH1, Next: relationCode}) || relevant != sibling {
				t.Fatal(infos[blockID], sibling, relevant)
			}
		} else {
			if infos[blockID].ParentID != infoItem || infos[blockID].NextID != positionRef || sibling != (docquery.RelationIDs{Parent: infoItem, Previous: relationCode, Next: positionSB}) || relevant != (docquery.RelationIDs{Parent: infoItem, Next: positionRef}) {
				t.Fatal(infos[blockID], sibling, relevant)
			}
		}
		infos[blockID].ParentID = "changed"
		again, e := l.NativeBlockTreeInfos(ctx, []string{blockID})
		if e != nil || again[blockID].ParentID == "changed" {
			t.Fatal(again, e)
		}
		empty, e := l.NativeBlockTreeInfos(ctx, nil)
		if e != nil || empty == nil || len(empty) != 0 {
			t.Fatal(empty, e)
		}
		if missing, e := l.NativeBlockSiblingID(ctx, "missing"); e != nil || missing != (docquery.RelationIDs{}) {
			t.Fatal(missing, e)
		}
		if _, e = l.NativeBlockRelevantIDs(ctx, "missing"); !errors.Is(e, ErrDocument) {
			t.Fatal(e)
		}
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("relation read implicitly flushed")
		}
	}
	if digest(t, root) != before {
		t.Fatal("relations changed source")
	}
}

func TestNativeBlockRelationsStaleRegistryCancellationAndBadSource(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	m := open(t, root, positionOptions(root))
	l := getDocLease(t, m, pageA)
	if _, e := l.NativeBlockTreeInfos(ctx, []string{blockID}); e != nil {
		t.Fatal(e)
	}
	if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		_, e = db.ExecContext(ctx, "INSERT INTO blocktrees SELECT ?,root_id,parent_id,box_id,path,hpath,updated,type FROM blocktrees WHERE id=?", relationGhost, blockID)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	infos, e := l.NativeBlockTreeInfos(ctx, []string{relationGhost})
	if e != nil || !reflect.DeepEqual(infos[relationGhost], &docquery.BlockTreeInfo{ID: relationGhost}) {
		t.Fatal(infos, e)
	}
	if _, e = l.NativeBlockRelevantIDs(ctx, relationGhost); !errors.Is(e, ErrDocument) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r, e := l.NativeBlockRelations(canceled, docquery.RelationRequest{Mode: "infos"}); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(r, docquery.RelationResult{}) {
		t.Fatal(r, e)
	}
	<-l.workspace.resources.gate
	deadline, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	_, e = l.NativeBlockRelevantIDs(deadline, blockID)
	stop()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")); e != nil {
		t.Fatal(e)
	}
	if infos, e = l.NativeBlockTreeInfos(ctx, []string{getDocDefinition, blockID}); e == nil || infos != nil {
		t.Fatal("partial/bad source fallback", infos, e)
	}
	l.Release()
	if _, e = l.NativeBlockSiblingID(ctx, blockID); e == nil {
		t.Fatal("released owner accepted")
	}
}

func TestNativeBlockRelationsConcurrentOwnersAndNoUnusedAVDependency(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, page := range []string{pageA, pageB} {
		if e := os.Remove(filepath.Join(root, "pages", page, "data", "storage", "av", getDocAV+".json")); e != nil {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			want := docID
			if l == b {
				want = infoItem
			}
			for i := 0; i < 3; i++ {
				rows, e := l.NativeBlockTreeInfos(ctx, []string{blockID})
				if e != nil || rows[blockID].ParentID != want {
					t.Error(rows, e)
					return
				}
			}
		}()
	}
	wg.Wait()
}
