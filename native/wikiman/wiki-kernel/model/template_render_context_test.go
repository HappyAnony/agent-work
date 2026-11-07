//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestTemplateRenderOwnedActualSourceSQLAndReadOnly(t *testing.T) {
	c, _, id, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	source := filepath.Join(util.DataDir, "templates", "actual.md")
	if e := os.MkdirAll(filepath.Dir(source), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(source, []byte("Title .action{.title} / .action{.id}\n\n.action{range queryBlocks \"SELECT * FROM blocks WHERE id='"+id+"'\"}.action{.Content}.action{end}"), 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	_, dom, e := RenderTemplateContext(ctx, source, id, true)
	if e != nil || !strings.Contains(dom, id) || !strings.Contains(dom, "definition") {
		t.Fatal(dom, e)
	}
	icon, e := RenderDynamicIconContentTemplateContext(ctx, ".action{.id} / .action{.name}", id)
	if e != nil || !strings.Contains(icon, id) {
		t.Fatal(icon, e)
	}
	out, e := RenderGoTemplateContext(ctx, `{{ "**two**" | markdown2text }} / {{ add 1 2 }}`)
	if e != nil || out != "two / 3" {
		t.Fatal(out, e)
	}
	if _, e = DocSaveAsTemplateContext(ctx, id, "saved", false); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	if raw, e := os.ReadFile(file); e != nil || string(raw) != string(before) {
		t.Fatal("read migrated source", e)
	}
	c.persistenceReadOnly = false
	if code, e := DocSaveAsTemplateContext(ctx, id, "saved", false); e != nil || code != 0 {
		t.Fatal(code, e)
	}
	saved := filepath.Join(util.DataDir, "templates", "saved.md")
	raw, e := os.ReadFile(saved)
	if e != nil || !strings.Contains(string(raw), "definition") {
		t.Fatal(string(raw), e)
	}
	if code, e := DocSaveAsTemplateContext(ctx, id, "saved", false); e != nil || code != 1 {
		t.Fatal(code, e)
	}
	if got, e := os.ReadFile(saved); e != nil || string(got) != string(raw) {
		t.Fatal("no-overwrite changed file", e)
	}
	if _, _, e = RenderTemplateContext(ctx, filepath.Join(t.TempDir(), "outside.md"), id, true); e == nil {
		t.Fatal("outside read accepted")
	}
	if _, e = RenderGoTemplateContext(ctx, `{{ env "HOME" }}`); e == nil {
		t.Fatal("ambient environment function accepted")
	}
}

func TestTemplateRenderCancellationStaleOwnerAndReadCapability(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := RenderGoTemplateContext(ctx, "text"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	o, e := captureWorkspaceRead(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = o.r.openRepairIOChecked(context.Background(), true); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal("read adapter gained write capability", e)
	}
	old := Conf
	Conf = NewAppConf()
	defer func() { Conf = old }()
	called := false
	if e = o.base.run(context.Background(), func(context.Context, context.Context) error { called = true; return nil }); e == nil || called {
		t.Fatal("stale adapter accepted", e)
	}
}
