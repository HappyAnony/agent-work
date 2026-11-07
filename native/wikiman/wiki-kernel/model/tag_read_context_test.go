//go:build fts5

package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestTagReadContextActualTreeSearchAndSortPersistence(t *testing.T) {
	c, env, _, _, _ := commitFixture(t)
	isolatedConflictQueue(t, flushTx)
	c.Tag = conf.NewTag()
	c.FileTree = conf.NewFileTree()
	c.FileTree.MaxListCount = 100
	if _, e := env.commitContext.r.main.DB.Exec(`INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES('tag-one','b','r','box','/d','Alpha','Alpha','tag',''),('tag-two','b2','r','box','/d','Alpha/sub','Alpha/sub','tag','')`); e != nil {
		t.Fatal(e)
	}
	sort := util.SortModeNameDESC
	tags, e := BuildTagsContext(context.Background(), &sort, true, "test")
	if e != nil || tags == nil || len(*tags) != 1 || (*tags)[0].Label != "Alpha" {
		t.Fatalf("tree %v %v", tags, e)
	}
	raw, e := os.ReadFile(c.confPath)
	if e != nil {
		t.Fatal(e)
	}
	var saved struct {
		Tag conf.Tag `json:"tag"`
	}
	if e = json.Unmarshal(raw, &saved); e != nil || saved.Tag.Sort != sort || c.Tag.Sort != sort {
		t.Fatalf("sort not persisted: %v %v", saved, e)
	}
	labels, e := SearchTagsContext(context.Background(), "Alpha")
	if e != nil || len(labels) != 2 {
		t.Fatalf("search %v %v", labels, e)
	}
	if _, e = env.commitContext.r.main.DB.Exec("DROP TABLE spans"); e != nil {
		t.Fatal(e)
	}
	if _, e = BuildTagsContext(context.Background(), nil, false, "test"); e == nil {
		t.Fatal("tree SQL error hidden")
	}
	if _, e = SearchTagsContext(context.Background(), "Alpha"); e == nil {
		t.Fatal("search SQL error hidden")
	}
}

func TestTagReadContextPrecancelDoesNotSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sort := 1
	if _, e := BuildTagsContext(ctx, &sort, false, "app"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := SearchTagsContext(ctx, "keyword"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestTagSortSaveFailureKeepsPublishedSetting(t *testing.T) {
	c, _, _, _, _ := commitFixture(t)
	c.Tag = conf.NewTag()
	c.FileTree = conf.NewFileTree()
	before := c.Tag.Sort
	original := c.confPath
	c.confPath = t.TempDir()
	defer func() { c.confPath = original }()
	sort := util.SortModeNameDESC
	if _, err := BuildTagsContext(context.Background(), &sort, false, "app"); err == nil {
		t.Fatal("save failure hidden")
	}
	if c.Tag.Sort != before {
		t.Fatal("failed save published new sort")
	}
}
