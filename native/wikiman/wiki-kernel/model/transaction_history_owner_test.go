//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func transactionHistoryFixture(t *testing.T) (*legacyCommitContext, *historyIndexContext, *parse.Tree, string) {
	t.Helper()
	_, env, tx, _, _ := commitFixture(t)
	old := util.HistoryDir
	util.HistoryDir = t.TempDir()
	t.Cleanup(func() { util.HistoryDir = old })
	h := &historyIndexContext{owner: env.commitContext.r.owner, root: util.HistoryDir, workspace: util.WorkspaceDir, queue: sql.CaptureLegacyHistoryQueue()}
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	return env.commitContext, h, tree, filepath.Join(env.commitContext.r.data, tree.Box, tree.Path)
}

func TestTransactionHistoryDocumentAVAndIndexOrdering(t *testing.T) {
	c, h, tree, file := transactionHistoryFixture(t)
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	id := ast.NewNodeID()
	tree.Root.AppendChild(&ast.Node{Type: ast.NodeAttributeView, AttributeViewID: id})
	avDir := filepath.Join(c.r.data, "storage", "av")
	if e = os.MkdirAll(avDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(avDir, id+".json"), []byte(`{"name":"before"}`), 0600); e != nil {
		t.Fatal(e)
	}
	calls := 0
	want := errors.New("index failed")
	e = writeTransactionHistory(context.Background(), c, h, tree, time.Unix(0, 0), func(name string) error {
		calls++
		got, e := os.ReadFile(filepath.Join(h.root, name, tree.Box, tree.Path))
		if e != nil || string(got) != string(before) {
			t.Fatalf("document preimage changed: %v", e)
		}
		got, e = os.ReadFile(filepath.Join(h.root, name, "storage", "av", id+".json"))
		if e != nil || string(got) != `{"name":"before"}` {
			t.Fatalf("AV preimage changed: %q %v", got, e)
		}
		return want
	})
	if !errors.Is(e, want) || calls != 1 {
		t.Fatalf("index error/order: %v %d", e, calls)
	}
}

func TestTransactionHistoryRejectsCancelStaleAndMissingAV(t *testing.T) {
	for _, kind := range []string{"cancel", "stale", "missing-av", "invalid-path", "write-failure"} {
		t.Run(kind, func(t *testing.T) {
			c, h, tree, _ := transactionHistoryFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			old := Conf
			switch kind {
			case "cancel":
				cancel()
			case "stale":
				Conf = &AppConf{}
				defer func() { Conf = old }()
			case "missing-av":
				tree.Root.AppendChild(&ast.Node{Type: ast.NodeAttributeView, AttributeViewID: ast.NewNodeID()})
			case "invalid-path":
				tree.Path = "../escape.docj"
			case "write-failure":
				if e := os.WriteFile(filepath.Join(h.root, time.Unix(0, 0).Format("2006-01-02-150405")+"-"+HistoryOpUpdate), []byte("block"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			err := writeTransactionHistory(ctx, c, h, tree, time.Unix(0, 0), func(string) error { calls++; return nil })
			if err == nil || calls != 0 {
				t.Fatalf("failure admitted index: %v %d", err, calls)
			}
		})
	}
}
