package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/catalogue"
)

func TestCatalogueOwnedWidgetSearchOriginalFieldsAndErrors(t *testing.T) {
	c, _, paths := repoMetadataFixture(t)
	c.Appearance = conf.NewAppearance()
	c.Lang = "zh_CN"
	dir := filepath.Join(paths.Data, "widgets", "owned")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(catalogue.Package{Name: "owned", DisplayName: &catalogue.DisplayName{Default: "default", ZhCN: "中文挂件"}, Description: &catalogue.Description{Default: "needle"}})
	file := filepath.Join(dir, "widget.json")
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	got, e := SearchWidgetContext(context.Background(), "needle")
	if e != nil || len(got) != 1 || got[0].Name != "中文挂件" || got[0].Content != "owned" {
		t.Fatal(got, e)
	}
	if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = SearchWidgetContext(context.Background(), ""); e == nil {
		t.Fatal("metadata corruption became empty success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = SearchWidgetContext(ctx, ""); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
