//go:build fts5

package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/util"
)

func TestWorkspaceStatisticsActualSourcesPersistenceAndCapturedJob(t *testing.T) {
	c, r, _, _, _ := referenceModelFixture(t)
	archiveWrite(t, util.DataDir, "assets/file.txt", []byte("owned assets"))
	archiveWrite(t, util.DataDir, "not-assets-name.txt", []byte("substring original rule"))
	wantData, wantAssets := util.DataSize()
	run, e := CaptureStatJob(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = run(context.Background()); e != nil {
		t.Fatal(e)
	}
	if c.Stat == nil || c.Stat.TreeCount != 2 || c.Stat.BlockCount < 4 || c.Stat.DataSize != wantData || c.Stat.AssetsSize != wantAssets {
		t.Fatal(c.Stat, wantData, wantAssets)
	}
	var persisted map[string]json.RawMessage
	raw, e := os.ReadFile(c.confPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &persisted); e != nil || persisted["stat"] == nil {
		t.Fatal("statistics not persisted", e)
	}
	before, _ := json.Marshal(c.Stat)
	if e = os.Symlink(t.TempDir(), filepath.Join(util.DataDir, "linked")); e != nil {
		t.Fatal(e)
	}
	if e = run(context.Background()); e == nil {
		t.Fatal("source link ignored")
	}
	after, _ := json.Marshal(c.Stat)
	if string(before) != string(after) {
		t.Fatal("incomplete source overwrote stats")
	}
	old := Conf
	Conf = NewAppConf()
	e = run(context.Background())
	Conf = old
	if e == nil {
		t.Fatal("job rebound to new owner")
	}
	os.Remove(filepath.Join(util.DataDir, "linked"))
	if e = r.registry.DB.Close(); e != nil {
		t.Fatal(e)
	}
	if e = run(context.Background()); e == nil {
		t.Fatal("failed index returned partial stats")
	}
}
