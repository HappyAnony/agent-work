// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestReferenceRefreshGroupsTreesAndFlushesBeforeCountPublication(t *testing.T) {
	var calls []string
	s := ReferenceServices{References: func(context.Context, string) ([]indexdb.RefRecord, error) {
		return []indexdb.RefRecord{{RootID: "using", DefBlockID: "def"}, {RootID: "using", DefBlockID: "def"}}, nil
	}, Load: func(_ context.Context, id string) (*parse.Tree, error) {
		calls = append(calls, "load")
		return &parse.Tree{ID: id, Root: &ast.Node{ID: id}}, nil
	}, Lookup: func(context.Context, string) (*indexdb.BlockTreeRecord, error) {
		return &indexdb.BlockTreeRecord{ID: "root", RootID: "root"}, nil
	}, Index: func(context.Context, *parse.Tree) error { calls = append(calls, "index"); return nil }, Flush: func(context.Context) error { calls = append(calls, "flush"); return nil }}
	ids, e := RefreshReferences(context.Background(), "root", s)
	if e != nil || !reflect.DeepEqual(ids, []string{"def", "using", "root"}) || !reflect.DeepEqual(calls, []string{"load", "index", "flush"}) {
		t.Fatal(ids, calls, e)
	}
	for _, fault := range []string{"load", "flush", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			next := s
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if fault == "load" {
				next.Load = func(context.Context, string) (*parse.Tree, error) { return nil, errors.New("bad source") }
			}
			if fault == "flush" {
				next.Flush = func(context.Context) error { return errors.New("db failed") }
			}
			if fault == "cancel" {
				cancel()
			}
			if ids, e := RefreshReferences(ctx, "root", next); e == nil || ids != nil {
				t.Fatal("failed work published counts", ids, e)
			}
		})
	}
}
func TestReferenceCountsOriginalDocumentAndBlockProjection(t *testing.T) {
	s := CountServices{Lookup: func(_ context.Context, id string) (*indexdb.BlockTreeRecord, error) {
		if id == "missing" {
			return nil, nil
		}
		return &indexdb.BlockTreeRecord{ID: id, RootID: "root"}, nil
	}, References: func(_ context.Context, id string, children bool) ([]string, error) {
		if id == "root" && children {
			return []string{"a", "b"}, nil
		}
		return []string{"a"}, nil
	}, Children: func(context.Context, string) ([]string, error) { return []string{"def-a", "def-b"}, nil }}
	doc, e := ReadReferenceCount(context.Background(), "root", s)
	if e != nil || doc.Count != 2 || doc.RootCount != 2 || len(doc.DefinitionIDs) != 2 {
		t.Fatal(doc, e)
	}
	block, e := ReadReferenceCount(context.Background(), "block", s)
	if e != nil || block.Count != 1 || block.RootCount != 2 || !reflect.DeepEqual(block.DefinitionIDs, []string{"block"}) {
		t.Fatal(block, e)
	}
	if missing, e := ReadReferenceCount(context.Background(), "missing", s); e != nil || missing != nil {
		t.Fatal(missing, e)
	}
	s.References = func(context.Context, string, bool) ([]string, error) { return nil, errors.New("query failed") }
	if count, e := ReadReferenceCount(context.Background(), "block", s); e == nil || count != nil {
		t.Fatal(count, e)
	}
}
