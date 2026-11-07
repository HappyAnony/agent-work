//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestWorkspaceBlockReadsActualSourceSQLReadOnlyAndErrors(t *testing.T) {
	c, r, id, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	root := "20261028140001-refdoc1"
	b, e := GetBlockContext(ctx, id)
	if e != nil || b == nil || b.Content != "definition" {
		t.Fatal(b, e)
	}
	for _, embed := range []bool{false, true} {
		dom, e := GetBlockDOMsContext(ctx, []string{id, root}, embed)
		if e != nil || len(dom) != 2 || !strings.Contains(dom[id], "definition") {
			t.Fatal(dom, e)
		}
	}
	children, e := GetChildBlocksContext(ctx, root)
	if e != nil || len(children) != 1 || children[0].ID != id || children[0].Type != "p" || children[0].Content != "definition" {
		t.Fatal(children, e)
	}
	tail, e := GetTailChildBlocksContext(ctx, root, 1)
	if e != nil || len(tail) != 1 || tail[0].ID != id {
		t.Fatal(tail, e)
	}
	md, e := GetBlockKramdownContext(ctx, id, "md")
	if e != nil || !strings.Contains(md, "definition") {
		t.Fatal(md, e)
	}
	info, e := GetDocInfoContext(ctx, id)
	if e != nil || info.RootID != root {
		t.Fatal(info, e)
	}
	ref, e := GetBlockRefTextContext(ctx, id)
	if e != nil || ref != "definition" {
		t.Fatal(ref, e)
	}
	if ok, e := CheckBlockRefContext(ctx, []string{id, root}); e != nil || !ok {
		t.Fatal(ok, e)
	}
	defs, original, e := GetBlockRefsContext(ctx, id)
	if e != nil || len(defs) != 1 || original == nil {
		t.Fatal(defs, original, e)
	}
	refIDs, e := GetBlockDefIDsByRefTextContext(ctx, "definition", nil)
	if e != nil || len(refIDs) == 0 {
		t.Fatal(refIDs, e)
	}
	if _, e = r.main.DB.Exec("INSERT INTO file_annotation_refs (id,block_id,root_id,box,path,annotation_id) VALUES (?,?,?,?,?,?)", "annotation-ref", id, root, b.Box, b.Path, "annotation"); e != nil {
		t.Fatal(e)
	}
	annotations, e := GetBlockRefIDsByFileAnnotationIDContext(ctx, "annotation")
	if e != nil || len(annotations) != 1 || annotations[0] != id {
		t.Fatal(annotations, e)
	}
	index, e := GetBlockIndexContext(ctx, id)
	if e != nil || index < 0 {
		t.Fatal(index, e)
	}
	relations, e := GetBlockTreeInfosContext(ctx, []string{id, root})
	if e != nil || len(relations) != 2 {
		t.Fatal(relations, e)
	}
	blocks, count, _, _, _, e := FullTextSearchBlockContext(ctx, "definition", nil, nil, nil, 0, 0, 0, 1, 16)
	if e != nil || count < 1 || len(blocks) < 1 {
		t.Fatal(blocks, count, e)
	}
	if _, e = RecentUpdatedBlocksContext(ctx); e != nil {
		t.Fatal(e)
	}
	if raw, e := os.ReadFile(file); e != nil || !bytes.Equal(raw, before) {
		t.Fatal("read modified source", e)
	}
	for _, name := range []string{"searchignore", "refsearchignore"} {
		if _, e := os.Stat(filepath.Join(util.DataDir, ".siyuan", name)); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("read created ignore file", e)
		}
	}
	if _, e = r.main.DB.Exec("DROP TABLE blocks_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if _, _, _, _, _, e = FullTextSearchBlockContext(ctx, "definition", nil, nil, nil, 0, 0, 0, 1, 16); e == nil {
		t.Fatal("SQL error swallowed")
	}
	if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = GetBlockContext(ctx, "20261028140004-refpar1"); e == nil {
		t.Fatal("corrupt source swallowed")
	}
}

func TestWorkspaceBlockReadCancellationAndCapturedOwner(t *testing.T) {
	c, _, id, _, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checks := []func() error{
		func() error { _, e := GetBlockContext(ctx, id); return e },
		func() error { _, e := GetChildBlocksContext(ctx, id); return e },
		func() error { _, e := GetDocInfoContext(ctx, id); return e },
		func() error {
			_, _, _, _, _, e := FullTextSearchBlockContext(ctx, "x", nil, nil, nil, 0, 0, 0, 1, 16)
			return e
		},
		func() error { _, e := RecentUpdatedBlocksContext(ctx); return e },
	}
	for _, check := range checks {
		if e := check(); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	o, e := captureWorkspaceRead(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	old := Conf
	Conf = NewAppConf()
	defer func() { Conf = old }()
	if e = o.base.run(context.Background(), func(op, life context.Context) error { _, e := o.load(op, id); return e }); e == nil {
		t.Fatal("stale owner accepted")
	}
}
