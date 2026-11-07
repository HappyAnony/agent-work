package model

import (
	"bytes"
	"context"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	repoutil "github.com/siyuan-note/dejavu/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestRepositoryReadActualEncryptedObjectsPreviewLogsDiffAndFailures(t *testing.T) {
	c, source, p, _ := restoreFixture(t)
	before, _ := os.ReadFile(source)
	ctx := context.Background()
	private := t.TempDir()
	p.Repo, p.History, p.Temp = filepath.Join(private, "repo"), filepath.Join(private, "history"), filepath.Join(private, "temp")
	oldRepo, oldHistory, oldTemp := util.RepoDir, util.HistoryDir, util.TempDir
	util.RepoDir, util.HistoryDir, util.TempDir = p.Repo, p.History, p.Temp
	t.Cleanup(func() {
		if e := c.CloseCloudRepo(context.Background()); e != nil {
			t.Error(e)
		}
		util.RepoDir, util.HistoryDir, util.TempDir = oldRepo, oldHistory, oldTemp
	})
	c.Editor, c.Export, c.Search = conf.NewEditor(), conf.NewExport(), conf.NewSearch()
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: t.TempDir(), ConcurrentReqs: 2, Timeout: 1}
	if e := c.repoOwner.Close(ctx); e != nil {
		t.Fatal(e)
	}
	service, e := c.newSnapshotRepoService(p)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = service
	store, e := dejavu.NewStore(p.Repo, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	put := func(name string, body []byte, updated int64) *entity.File {
		t.Helper()
		chunk := &entity.Chunk{ID: repoutil.Hash(body), Data: body}
		if e := store.PutChunk(chunk); e != nil {
			t.Fatal(e)
		}
		file := entity.NewFile(name, int64(len(body)), updated)
		file.Chunks = []string{chunk.ID}
		if e := store.PutFile(file); e != nil {
			t.Fatal(e)
		}
		return file
	}
	raw := []byte(`{"ID":"20261104210000-repo001","Type":"NodeDocument","Properties":{"id":"20261104210000-repo001","title":"Actual snapshot"},"Children":[{"ID":"20261104210001-repo002","Type":"NodeParagraph","Properties":{"id":"20261104210001-repo002"},"Children":[{"Type":"NodeText","Data":"old body"}]}]}`)
	a := put("/box/document.docj", raw, 1000)
	newer := []byte(strings.ReplaceAll(string(raw), "old body", "new body"))
	b := put("/box/document.docj", newer, 2000)
	asset := put("/box/document/assets/fixture.png", []byte("actual image"), 1000)
	left, right := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, index := range []*entity.Index{{ID: left, Files: []string{a.ID, asset.ID}, Count: 2, Created: 1000}, {ID: right, Files: []string{b.ID, asset.ID}, Count: 2, Created: 2000}} {
		if e = store.PutIndex(index); e != nil {
			t.Fatal(e)
		}
	}
	c.persistenceReadOnly = true
	data, name, e := GetRepoFileContext(ctx, a.ID)
	if e != nil || !bytes.Equal(data, raw) || name != a.Path {
		t.Fatal(name, e)
	}
	title, dom, text, updated, e := OpenRepoSnapshotDocContext(ctx, a.ID)
	if e != nil || title != "Actual snapshot" || text || updated != 1000 || !strings.Contains(dom, "old body") || strings.Contains(dom, `contenteditable="true"`) {
		t.Fatal(title, dom, text, updated, e)
	}
	_, imagePath, _, _, e := OpenRepoSnapshotDocContext(ctx, asset.ID)
	if e != nil || imagePath != "repo/diff/assets/fixture.png" {
		t.Fatal(imagePath, e)
	}
	cached, e := os.ReadFile(filepath.Join(p.Temp, filepath.FromSlash(imagePath)))
	if e != nil || string(cached) != "actual image" {
		t.Fatal(string(cached), e)
	}
	logs, pages, total, e := GetRepoSnapshotsContext(ctx, 1)
	if e != nil || len(logs) != 2 || pages != 1 || total != 2 || logs[0].Files != nil {
		t.Fatal(logs, pages, total, e)
	}
	diff, e := DiffRepoSnapshotsContext(ctx, left, right)
	if e != nil || diff == nil || len(diff.UpdatesLeft) != 1 || len(diff.UpdatesRight) != 1 {
		t.Fatal(diff, e)
	}
	if rows, e := GetTagSnapshotsContext(ctx); e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	c.persistenceReadOnly = false
	if e = TagSnapshot(left, "backup"); e != nil {
		t.Fatal(e)
	}
	tags, e := GetTagSnapshotsContext(ctx)
	if e != nil || len(tags) != 1 || tags[0].ID != left {
		t.Fatal(tags, e)
	}
	if e = store.PutIndex(&entity.Index{ID: strings.Repeat("d", 40), Files: []string{strings.Repeat("c", 40)}, Count: 1, Created: 3000}); e != nil {
		t.Fatal(e)
	}
	if logs, _, _, e = GetRepoSnapshotsContext(ctx, 1); e == nil || logs != nil {
		t.Fatal("missing selected object returned partial logs", logs, e)
	}
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("snapshot reads changed source", e)
	}
}
