//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestHistoryRenderActualOwnedReadOnlyAndPathBoundary(t *testing.T) {
	c, _, _, _, source := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	oldHistory := util.HistoryDir
	util.HistoryDir = filepath.Join(util.WorkspaceDir, "history")
	t.Cleanup(func() { util.HistoryDir = oldHistory })
	name := filepath.Join(util.HistoryDir, "stamp", "history.docj")
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(name, source, 0600); e != nil {
		t.Fatal(e)
	}
	id, root, dom, large, e := GetDocHistoryContentContext(ctx, name, "definition", true)
	if e != nil || id == "" || root != id || large || !strings.Contains(dom, "definition") || strings.Contains(dom, `contenteditable="true"`) {
		t.Fatal(id, root, dom, large, e)
	}
	after, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(after, source) {
		t.Fatal("changed history source", e)
	}
	if _, _, _, _, e = GetDocHistoryContentContext(ctx, filepath.Join(t.TempDir(), "outside.docj"), "", false); e == nil {
		t.Fatal("outside owner accepted")
	}
	if id, _, dom, _, e = GetDocHistoryContentContext(ctx, filepath.Join(filepath.Dir(name), "missing.docj"), "", false); e != nil || id != "" || dom != "" {
		t.Fatal(id, dom, e)
	}
	if e = os.WriteFile(name, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if id, root, dom, large, e = GetDocHistoryContentContext(ctx, name, "", false); e == nil || id != "" || root != "" || dom != "" || large {
		t.Fatal(id, root, dom, large, e)
	}
}
