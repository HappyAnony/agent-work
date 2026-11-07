//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/util"
)

func notebookLifecycleFixture(t *testing.T) (*AppConf, *indexRepairContext, string, string, []byte) {
	t.Helper()
	c, r, _, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.Editor.VirtualBlockRef = false
	data := util.DataDir
	id := filepath.Base(filepath.Dir(file))
	meta := filepath.Join(data, id, ".siyuan/conf.json")
	if e := os.MkdirAll(filepath.Dir(meta), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(meta, []byte(`{"name":"existing","closed":false}`), 0600); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var labels struct {
		Time map[string]string `json:"_time"`
	}
	if e = json.Unmarshal(raw, &labels); e != nil {
		t.Fatal(e)
	}
	c.business = conf.NewBusiness(func(ctx context.Context) (out []conf.Notebook, result error) {
		root, e := os.OpenRoot(data)
		if e != nil {
			return nil, e
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		plan, e := notebooks.Scan(ctx, root, ".siyuan", 0)
		return plan.Notebooks, e
	}, nil, nil, map[string]map[string]string{c.Lang: labels.Time})
	return c, r, id, file, before
}
func TestNotebookMountActualIndexAndUnmountJoin(t *testing.T) {
	c, r, id, file, before := notebookLifecycleFixture(t)
	ctx := context.Background()
	if e := UnmountContext(ctx, id); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := r.main.DB.QueryRow("SELECT count(*) FROM blocks WHERE box=?", id).Scan(&count); e != nil || count != 0 {
		t.Fatal("unindex not joined", count, e)
	}
	value, e := NotebookConfigurationContext(ctx, id)
	if e != nil || !value.Closed {
		t.Fatal(value, e)
	}
	c.persistenceReadOnly = true
	if _, e = MountContext(ctx, id); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal("readonly mount", e)
	}
	c.persistenceReadOnly = false
	already, e := MountContext(ctx, id)
	if e != nil || already {
		t.Fatal(already, e)
	}
	if e = r.main.DB.QueryRow("SELECT count(*) FROM blocks WHERE box=?", id).Scan(&count); e != nil || count < 2 {
		t.Fatal("index not joined", count, e)
	}
	already, e = MountContext(ctx, id)
	if e != nil || !already {
		t.Fatal("already mounted", already, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(after, before) {
		t.Fatal("mount changed valid source", e)
	}
}
func TestNotebookGuideReadonlyHasExplicitDeviceCopy(t *testing.T) {
	c, _, _, file, before := notebookLifecycleFixture(t)
	ctx := context.Background()
	guide := "20210808180117-czj9bvb"
	oldWorking := util.WorkingDir
	util.WorkingDir = t.TempDir()
	t.Cleanup(func() {
		if e := c.CloseCloudRepo(context.Background()); e != nil {
			t.Error(e)
		}
		util.WorkingDir = oldWorking
	})
	root := filepath.Join(util.WorkingDir, "guide", guide)
	if e := os.MkdirAll(filepath.Join(root, ".siyuan"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, ".siyuan/conf.json"), []byte(`{"name":"guide","closed":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	// A valid original document exercises actual guide indexing, not only metadata.
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	raw = bytes.ReplaceAll(raw, []byte("20261028140003-refdoc2"), []byte("20261104123400-guidoc1"))
	raw = bytes.ReplaceAll(raw, []byte("20261028140004-refpar1"), []byte("20261104123401-guipar1"))
	name := "20261104123400-guidoc1.docj"
	if e = os.WriteFile(filepath.Join(root, name), raw, 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e = MountContext(ctx, guide); e != nil {
		t.Fatal(e)
	}
	copied, e := os.ReadFile(filepath.Join(util.DataDir, guide, name))
	if e != nil || !bytes.Equal(copied, raw) {
		t.Fatal("guide copy missing", e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(after, before) {
		t.Fatal("guide wrote unrelated notebook", e)
	}
	if e = RemoveBoxContext(ctx, guide); e != nil {
		t.Fatal("guide read-only removal", e)
	}
	if _, e = os.Stat(filepath.Join(util.DataDir, guide)); !os.IsNotExist(e) {
		t.Fatal("guide not removed", e)
	}
}
