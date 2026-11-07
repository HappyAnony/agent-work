//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func setFoldSource(t *testing.T, root, page, id, fold string) {
	t.Helper()
	file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	var change func(map[string]interface{})
	change = func(n map[string]interface{}) {
		if n["ID"] == id {
			p, ok := n["Properties"].(map[string]interface{})
			if !ok {
				p = map[string]interface{}{}
				n["Properties"] = p
			}
			p["fold"] = fold
		}
		if children, ok := n["Children"].([]interface{}); ok {
			for _, c := range children {
				change(c.(map[string]interface{}))
			}
		}
	}
	change(d)
	raw, e = json.Marshal(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
}
func foldSource(t *testing.T) string {
	t.Helper()
	root := positionSource(t)
	setFoldSource(t, root, pageA, positionH1, "1")
	setFoldSource(t, root, pageB, positionH1, "0")
	return root
}

func TestNativeBlockFoldActualHeadingOwnersNoFlushAndFreshSource(t *testing.T) {
	ctx := context.Background()
	root := foldSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		want := blockID
		if l == a {
			want = positionH1
		}
		parent, e := l.NativeUnfoldedParentID(ctx, blockID)
		if e != nil || parent != want {
			t.Fatal(parent, want, e)
		}
		folded, isRoot, e := l.NativeIsBlockFolded(ctx, blockID)
		if e != nil || folded != (l == a) || isRoot {
			t.Fatal(folded, isRoot, e)
		}
		// The registry has physical parent docID: fold traversal must still
		// see the preceding logical heading, without flushing main SQL.
		if e = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, e := bundle.Database(indexdb.BlockTree)
			if e != nil {
				return e
			}
			var parent string
			e = db.QueryRowContext(ctx, "SELECT parent_id FROM blocktrees WHERE id=?", blockID).Scan(&parent)
			if e == nil && parent != docID {
				t.Fatal(parent)
			}
			return e
		}); e != nil {
			t.Fatal(e)
		}
		folded, isRoot, e = l.NativeIsBlockFolded(ctx, docID)
		if e != nil || folded || !isRoot {
			t.Fatal(folded, isRoot, e)
		}
		parent, e = l.NativeUnfoldedParentID(ctx, "missing")
		if e != nil || parent != "" {
			t.Fatal(parent, e)
		}
		folded, isRoot, e = l.NativeIsBlockFolded(ctx, "missing")
		if e != nil || folded || isRoot {
			t.Fatal(folded, isRoot, e)
		}
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("fold read flushed main queue")
		}
	}
	if digest(t, root) != before {
		t.Fatal("fold read changed source")
	}
	// No long-lived fold/tree cache and no main-index fold dependency.
	setFoldSource(t, root, pageA, positionH1, "0")
	if folded, _, e := a.NativeIsBlockFolded(ctx, blockID); e != nil || folded {
		t.Fatal(folded, e)
	}
	if parent, e := a.NativeUnfoldedParentID(ctx, blockID); e != nil || parent != blockID {
		t.Fatal(parent, e)
	}
	setFoldSource(t, root, pageA, docID, "1")
	if folded, isRoot, e := a.NativeIsBlockFolded(ctx, docID); e != nil || !folded || !isRoot {
		t.Fatal(folded, isRoot, e)
	}
}

func TestNativeBlockFoldStaleRegistryCancelBadSourceAndRelease(t *testing.T) {
	ctx := context.Background()
	root := foldSource(t)
	m := open(t, root, positionOptions(root))
	l := getDocLease(t, m, pageA)
	if _, e := l.NativeUnfoldedParentID(ctx, blockID); e != nil {
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
	for _, mode := range []string{"state", "unfolded-parent"} {
		r, e := l.NativeBlockFold(ctx, docquery.FoldRequest{Mode: mode, ID: relationGhost})
		if e != nil || r != (docquery.FoldResult{}) {
			t.Fatal(r, e)
		}
		c, stop := context.WithCancel(ctx)
		stop()
		r, e = l.NativeBlockFold(c, docquery.FoldRequest{Mode: mode, ID: blockID})
		if !errors.Is(e, context.Canceled) || r != (docquery.FoldResult{}) {
			t.Fatal(r, e)
		}
	}
	<-l.workspace.resources.gate
	c, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	_, e := l.NativeUnfoldedParentID(c, blockID)
	stop()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"state", "unfolded-parent"} {
		r, e := l.NativeBlockFold(ctx, docquery.FoldRequest{Mode: mode, ID: blockID})
		if e == nil || r != (docquery.FoldResult{}) {
			t.Fatal("bad source returned success", r, e)
		}
	}
	l.Release()
	if _, e = l.NativeUnfoldedParentID(ctx, blockID); e == nil {
		t.Fatal("released owner accepted")
	}
}

func TestNativeBlockFoldConcurrentOwnersWithoutAV(t *testing.T) {
	ctx := context.Background()
	root := foldSource(t)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, page := range []string{pageA, pageB} {
		if e := os.Remove(filepath.Join(root, "pages", page, "data", "storage", "av", getDocAV+".json")); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	var wg sync.WaitGroup
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				folded, isRoot, e := l.NativeIsBlockFolded(ctx, blockID)
				if e != nil || folded != (l == a) || isRoot {
					t.Error(folded, isRoot, e)
					return
				}
			}
		}()
	}
	wg.Wait()
	if digest(t, root) != before {
		t.Fatal("concurrent fold reads changed source")
	}
}
