//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	repoutil "github.com/siyuan-note/dejavu/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestHistoricalAVReadNeverPublishesCurrentSource(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, layout)
			Conf.Export = conf.NewExport()
			Conf.persistenceReadOnly = true
			current, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			stamp := time.Now().Unix()
			created := strconv.FormatInt(stamp, 10)
			historical := filepath.Join(util.HistoryDir, time.Unix(stamp, 0).Format("2006-01-02-150405")+"-update", "storage", "av", a.ID+".json")
			if e = os.MkdirAll(filepath.Dir(historical), 0700); e != nil {
				t.Fatal(e)
			}
			a.Name = "Historical AV"
			raw, _ := json.Marshal(a)
			if e = os.WriteFile(historical, raw, 0600); e != nil {
				t.Fatal(e)
			}
			view, got, e := RenderHistoryAttributeViewContext(context.Background(), "", a.ID, "", "", 1, -1, nil, created)
			if e != nil || view == nil || got == nil || got.Name != "Historical AV" {
				t.Fatal(view, got, e)
			}
			after, e := os.ReadFile(file)
			if e != nil || !bytes.Equal(current, after) {
				t.Fatal("current source changed", e)
			}
			after, e = os.ReadFile(historical)
			if e != nil || !bytes.Equal(raw, after) {
				t.Fatal("historical source changed", e)
			}
			if e = os.Remove(historical); e != nil {
				t.Fatal(e)
			}
			_, got, e = RenderHistoryAttributeViewContext(context.Background(), "", a.ID, "", "", 1, -1, nil, created)
			if e != nil || got == nil || got.Name != "Scoped AV" {
				t.Fatal(got, e)
			}
			if e = os.WriteFile(historical, []byte("broken"), 0600); e != nil {
				t.Fatal(e)
			}
			view, got, e = RenderHistoryAttributeViewContext(context.Background(), "", a.ID, "", "", 1, -1, nil, created)
			if e == nil || got != nil || view != nil {
				t.Fatal("selected corruption hidden", got, e)
			}
			if e = os.Remove(historical); e != nil {
				t.Fatal(e)
			}
			if e = os.Remove(file); e != nil {
				t.Fatal(e)
			}
			view, got, e = RenderHistoryAttributeViewContext(context.Background(), "", a.ID, "", "", 1, -1, nil, created)
			if e != nil || got == nil || view == nil {
				t.Fatal("detached default", got, e)
			}
			if _, e = os.Stat(file); !os.IsNotExist(e) {
				t.Fatal("default published", e)
			}
			view, got, e = RenderHistoryAttributeViewContext(context.Background(), "", a.ID, "", "", 1, -1, nil, "0")
			if e != nil || got != nil || view != nil {
				t.Fatal("missing prefix", got, e)
			}
		})
	}
}

func TestSnapshotAVActualEncryptedMetadataAndDetachedRepair(t *testing.T) {
	_, _, a, current := deepAVFixture(t, av.LayoutTypeGallery)
	c := Conf
	c.Export = conf.NewExport()
	p := metadataRepoPaths{Workspace: util.WorkspaceDir, Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir}
	if e := c.repoOwner.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	service, e := c.newSnapshotRepoService(p)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = service
	before, e := os.ReadFile(current)
	if e != nil {
		t.Fatal(e)
	}
	a.Name = "Encrypted historical AV"
	raw, _ := json.Marshal(a)
	store, e := dejavu.NewStore(p.Repo, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	chunk := &entity.Chunk{ID: repoutil.Hash(raw), Data: raw}
	f := entity.NewFile("/storage/av/"+a.ID+".json", int64(len(raw)), 1000)
	f.Chunks = []string{chunk.ID}
	id := strings.Repeat("a", 40)
	if e = store.PutChunk(chunk); e != nil {
		t.Fatal(e)
	}
	if e = store.PutFile(f); e != nil {
		t.Fatal(e)
	}
	if e = store.PutIndex(&entity.Index{ID: id, Files: []string{f.ID}, Count: 1, Created: 1000}); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	view, got, e := RenderRepoSnapshotAttributeViewContext(context.Background(), id, a.ID)
	if e != nil || view == nil || got == nil || got.Name != a.Name {
		t.Fatal(view, got, e)
	}
	after, e := os.ReadFile(current)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("snapshot render changed current source", e)
	}
}
