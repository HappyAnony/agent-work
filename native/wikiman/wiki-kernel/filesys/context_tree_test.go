//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package filesys

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"

	"wiki-kernel/internal/indexrepair"
)

const treeBox = "20261029120000-boxaaaa"
const treeID = "20261029120001-docaaaa"
const parentID = "20261029120002-parenta"

func treeFixture(t *testing.T) (TreeContext, string, []byte) {
	t.Helper()
	root, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	root.MkdirAll(treeBox, 0700)
	raw := []byte(`{"ID":"` + treeID + `","Type":"NodeDocument","Properties":{"id":"` + treeID + `","title":"Original"},"Children":[{"ID":"20261029120003-blockaa","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"source text"}]}]}`)
	name := treeBox + "/" + treeID + ".docj"
	if e = root.WriteFile(name, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return TreeContext{Source: indexrepair.Source{Data: root}}, name, raw
}
func TestTreeContextReadonlyDoesNotRepairOrCreateParents(t *testing.T) {
	c, name, before := treeFixture(t)
	ctx := context.Background()
	engine := lute.New()
	tr, e := c.Load(ctx, treeBox, "/"+treeID+".docj", engine, false)
	if e != nil {
		t.Fatal(e)
	}
	if tr.Root.Spec != "" {
		t.Fatal("readonly migrated spec")
	}
	after, _ := c.Source.Data.ReadFile(name)
	if !bytes.Equal(before, after) {
		t.Fatal("readonly changed bytes")
	}
	if _, e = c.Load(ctx, treeBox, "/"+treeID+".docj", engine, true); !errors.Is(e, indexrepair.ErrSourceWrite) {
		t.Fatal(e)
	}
	c.Source.Data.MkdirAll(treeBox+"/"+parentID, 0700)
	nested := treeBox + "/" + parentID + "/" + treeID + ".docj"
	c.Source.Data.WriteFile(nested, before, 0600)
	if _, e = c.Load(ctx, treeBox, "/"+parentID+"/"+treeID+".docj", engine, false); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Source.Data.Stat(treeBox + "/" + parentID + ".docj"); !os.IsNotExist(e) {
		t.Fatal("readonly added parent", e)
	}
}
func TestTreeContextRepairRetainsMigrationParentAndWriteBehavior(t *testing.T) {
	c, _, raw := treeFixture(t)
	ctx := context.Background()
	engine := lute.New()
	c.Source.Authorize = func(ctx context.Context) error { return ctx.Err() }
	var upserts, writes int
	c.Upsert = func(context.Context, *parse.Tree) error { upserts++; return nil }
	c.Written = func(context.Context, *parse.Tree, uint64) error { writes++; return nil }
	c.Source.Data.MkdirAll(treeBox+"/"+parentID, 0700)
	name := treeBox + "/" + parentID + "/" + treeID + ".docj"
	c.Source.Data.WriteFile(name, raw, 0600)
	tr, e := c.Load(ctx, treeBox, "/"+parentID+"/"+treeID+".docj", engine, true)
	if e != nil {
		t.Fatal(e)
	}
	if tr.Root.Spec != "1" || tr.HPath != "/Untitled/Original" || writes != 2 || upserts == 0 {
		t.Fatalf("spec=%s hpath=%s writes=%d upserts=%d", tr.Root.Spec, tr.HPath, writes, upserts)
	}
	b, e := c.Source.Data.ReadFile(name)
	if e != nil || bytes.Equal(b, raw) || !json.Valid(b) {
		t.Fatal("migration not persisted", e)
	}
	if _, e = c.Source.Data.Stat(treeBox + "/" + parentID + ".docj"); e != nil {
		t.Fatal(e)
	}
	if e = c.Write(ctx, tr, raw, engine); !errors.Is(e, indexrepair.ErrSourceChanged) {
		t.Fatal("stale bytes accepted", e)
	}
}
func TestTreeContextRepairFailureAndIDCorrection(t *testing.T) {
	c, name, raw := treeFixture(t)
	ctx := context.Background()
	engine := lute.New()
	c.Source.Authorize = func(context.Context) error { return nil }
	other := "20261029120004-otheraa"
	wrong := bytes.ReplaceAll(raw, []byte(treeID), []byte(other))
	c.Source.Data.WriteFile(name, wrong, 0600)
	if _, e := c.Load(ctx, treeBox, "/"+treeID+".docj", engine, false); !errors.Is(e, indexrepair.ErrReferenceTree) {
		t.Fatal(e)
	}
	sentinel := errors.New("cache failed")
	c.Written = func(context.Context, *parse.Tree, uint64) error { return sentinel }
	if _, e := c.Load(ctx, treeBox, "/"+treeID+".docj", engine, true); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	b, _ := c.Source.Data.ReadFile(name)
	var doc struct{ ID, Spec string }
	json.Unmarshal(b, &doc)
	if doc.ID != treeID || doc.Spec != "1" {
		t.Fatal(string(b))
	}
	// A post-publication failure is reported, never described as a source rollback.
	c.Written = nil
	tr, e := c.Load(ctx, treeBox, "/"+treeID+".docj", engine, true)
	if e != nil || tr.ID != treeID {
		t.Fatal(e)
	}
}
