//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"

	"wiki-kernel/cache"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func TestIncrementalIndexActualSameIDMoveAndCaches(t *testing.T) {
	_, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	id := strings.TrimSuffix(filepath.Base(file), ".docj")
	old := "/" + box + "/" + id + ".docj"
	parent := "20261029180000-parenta"
	p := "/" + parent + "/" + id + ".docj"
	os.MkdirAll(filepath.Join(filepath.Dir(file), parent), 0700)
	target := filepath.Join(filepath.Dir(file), parent, filepath.Base(file))
	if e := os.Rename(file, target); e != nil {
		t.Fatal(e)
	}
	if e := cache.OpenLegacyIAL(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cache.CloseLegacyIAL)
	cache.PublishBlockIAL("20261028140004-refpar1", map[string]string{"stale": "true"})
	cache.PutDocIAL("/"+id+".docj", map[string]string{"stale": "true"})
	deadline := time.Now().Add(time.Second)
	for cache.GetDocIAL("/" + id + ".docj")["stale"] != "true" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cache.GetBlockIAL("20261028140004-refpar1")["stale"] != "true" || cache.GetDocIAL("/" + id + ".docj")["stale"] != "true" {
		t.Fatal("cache fixture not materialized")
	}
	out, e := r.incrementalIndex(ctx, []string{"/" + box + p}, []string{old})
	if e != nil || len(out.UpsertRootIDs) != 1 || len(out.RemoveRootIDs) != 1 {
		t.Fatal(out, e)
	}
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if e != nil || len(rows) != 1 || rows[0].Path != p {
		t.Fatal(rows, e)
	}
	b := sql.GetBlock(id)
	if b == nil || b.Path != p {
		t.Fatal("SQL move masked by remove", b)
	}
	var roots int
	if e = r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&roots); e != nil || roots != 1 {
		t.Fatal("move created duplicate roots", roots, e)
	}
	after, e := os.ReadFile(target)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("canonical source unexpectedly changed", e)
	}
	if cache.GetDocIAL("/"+id+".docj") != nil || cache.GetBlockIAL("20261028140004-refpar1") != nil {
		t.Fatal("stale IAL retained")
	}
	recordCloudBusinessEvidence(t, "incremental-move.json", map[string]interface{}{"sameIDMove": true, "removeBeforeUpsert": true, "actualSQLAndRegistry": true, "sourceUnchanged": true, "staleCacheRemoved": true})
}
func TestIncrementalIndexLoadFailureReturnsNoSuccessAndRetries(t *testing.T) {
	_, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	rel := filepath.Base(filepath.Dir(file)) + "/" + filepath.Base(file)
	os.WriteFile(file, []byte("corrupt fixture"), 0600)
	out, e := r.incrementalIndex(ctx, []string{rel}, nil)
	if e == nil || out.UpsertRootIDs != nil {
		t.Fatal("load failure hidden", out, e)
	}
	after, _ := os.ReadFile(file)
	if string(after) != "corrupt fixture" {
		t.Fatal("bad source overwritten")
	}
	os.WriteFile(file, before, 0600)
	out, e = r.incrementalIndex(ctx, []string{rel}, nil)
	if e != nil || len(out.UpsertRootIDs) != 1 {
		t.Fatal(out, e)
	}
	recordCloudBusinessEvidence(t, "incremental-load.json", map[string]interface{}{"loadFailureReturned": true, "badSourcePreserved": true, "noSuccessIDsOnFailure": true, "retry": true})
}
func TestIncrementalIndexActualSQLFailureRetainsQueueAndRetries(t *testing.T) {
	_, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	rel := filepath.Base(filepath.Dir(file)) + "/" + filepath.Base(file)
	id := strings.TrimSuffix(filepath.Base(file), ".docj")
	before = bytes.ReplaceAll(before, []byte("reference fixture"), []byte("incremental changed fixture"))
	if e := os.WriteFile(file, before, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := r.main.DB.Exec("CREATE TRIGGER fail_incremental BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'incremental fixture'); END"); e != nil {
		t.Fatal(e)
	}
	out, e := r.incrementalIndex(ctx, []string{rel}, nil)
	if e == nil || out.UpsertRootIDs != nil {
		t.Fatal("flush failure hidden", out, e)
	}
	if _, e = r.main.DB.Exec("DROP TRIGGER fail_incremental"); e != nil {
		t.Fatal(e)
	}
	out, e = r.incrementalIndex(ctx, []string{rel}, nil)
	if e != nil || len(out.UpsertRootIDs) != 1 || sql.GetBlock(id) == nil {
		t.Fatal(out, e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("derived failure changed canonical source")
	}
	recordCloudBusinessEvidence(t, "incremental-sql.json", map[string]interface{}{"actualTriggerFailure": true, "failedQueueRetry": true, "noSuccessIDsOnFailure": true, "sourceUnchanged": true})
}
func TestIncrementalIndexOwnedFolderPathsReadonlyAndStaleContext(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	paths, e := r.expandIndexPaths(ctx, []string{"/" + box + "/", box + "/" + filepath.Base(file)})
	if e != nil || len(paths) != 2 {
		t.Fatal(paths, e)
	}
	if e = UpsertIndexesContext(ctx, []string{box + "/"}); e != nil {
		t.Fatal(e)
	}
	old := util.DataDir
	util.DataDir = t.TempDir()
	_, e = r.expandIndexPaths(ctx, []string{box + "/"})
	util.DataDir = old
	if e == nil {
		t.Fatal("stale root accepted")
	}
	for _, p := range []string{"../", "/", "/" + box + "/../"} {
		if _, e = r.expandIndexPaths(ctx, []string{p}); e == nil {
			t.Fatal(p)
		}
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(filepath.Dir(file), "link"))
	if _, e = r.expandIndexPaths(ctx, []string{box + "/"}); e == nil {
		t.Fatal("symlink scan accepted")
	}
	os.Remove(filepath.Join(filepath.Dir(file), "link"))
	c.m.Lock()
	c.persistenceReadOnly = true
	c.m.Unlock()
	e = RemoveIndexesContext(ctx, []string{box + "/"})
	c.m.Lock()
	c.persistenceReadOnly = false
	c.m.Unlock()
	if !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = UpsertIndexesContext(canceled, []string{box + "/"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("guard changed source")
	}
	if e = c.CloseCloudRepo(ctx); e != nil {
		t.Fatal(e)
	}
	if e = RemoveIndexesContext(ctx, []string{box + "/"}); !errors.Is(e, cloudrepo.ErrClosed) {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "incremental-owned.json", map[string]interface{}{"folderDeduplicated": true, "staleContextRejected": true, "readonlyRejected": true, "symlinkRejected": true, "canceledRejected": true, "closedRejected": true, "sourceUnchanged": true})
}

func TestIncrementalIndexFolderCheckedEnumeration(t *testing.T) {
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			_, r, _, file, before := referenceModelFixture(t)
			ctx := context.Background()
			box := filepath.Base(filepath.Dir(file))
			late := "20261102180000-latefld"
			latePath := filepath.Join(filepath.Dir(file), late)
			if err := os.MkdirAll(latePath, 0700); err != nil {
				t.Fatal(err)
			}
			hidden := filepath.Join(filepath.Dir(file), ".hidden")
			if err := os.MkdirAll(hidden, 0700); err != nil {
				t.Fatal(err)
			}
			hiddenDoc := filepath.Join(hidden, "20261102180001-hiddend.docj")
			if err := os.WriteFile(hiddenDoc, before, 0600); err != nil {
				t.Fatal(err)
			}
			primary := errors.New("incremental folder enumeration failed")
			op := incrementalIndexWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name != filepath.ToSlash(filepath.Join(box, late)) {
					return source.ReadDir(ctx, name)
				}
				if kind == "incomplete" {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				old := file + ".enumerated"
				if err := os.Rename(file, old); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(old) })
				if err := os.WriteFile(file, before, 0600); err != nil {
					t.Fatal(err)
				}
				return source.ReadDir(ctx, name)
			}}
			paths, err := r.expandIndexPathsWithIO(ctx, []string{box + "/"}, op)
			if paths != nil {
				t.Fatal("partial incremental paths returned", paths)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal(err)
			}
			after, readErr := os.ReadFile(file)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("checked enumeration changed source bytes", readErr)
			}
		})
	}

	_, r, _, file, _ := referenceModelFixture(t)
	box := filepath.Base(filepath.Dir(file))
	hidden := filepath.Join(filepath.Dir(file), ".hidden")
	if err := os.MkdirAll(hidden, 0700); err != nil {
		t.Fatal(err)
	}
	hiddenRel := filepath.ToSlash(filepath.Join(box, ".hidden", "20261102180001-hiddend.docj"))
	if err := os.WriteFile(filepath.Join(hidden, "20261102180001-hiddend.docj"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := r.expandIndexPaths(context.Background(), []string{box + "/"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range paths {
		found = found || name == hiddenRel
	}
	if !found {
		t.Fatal("hidden directory compatibility lost", paths)
	}
}
func TestIncrementalSyncMergeLoadErrorStopsSuccessFollowup(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	id := "20261029183000-newdoca"
	// Keep the original full-rebuild threshold outside this incremental fixture.
	for i := 0; i < 8; i++ {
		root := fmt.Sprintf("202610291831%02d-dummyxx", i)
		tr := treenode.NewTree(box, "/"+root+".docj", "/fixture", "fixture")
		if e := r.registry.Upsert(ctx, tr); e != nil {
			t.Fatal(e)
		}
	}
	mr := &dejavu.MergeResult{Time: time.Now(), Upserts: []*entity.File{{Path: "/" + box + "/" + id + ".docj"}}}
	if e := processSyncMergeResult(ctx, false, true, mr, &dejavu.TrafficStat{}, "fixture", time.Millisecond); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("merge load error hidden", e)
	}
	c.repoMu.Lock()
	tasks := c.repoTasks
	c.repoMu.Unlock()
	if tasks != nil {
		t.Fatal("scheduled success UI after failed incremental index")
	}
	raw := bytes.ReplaceAll(before, []byte("20261028140003-refdoc2"), []byte(id))
	raw = bytes.ReplaceAll(raw, []byte("20261028140004-refpar1"), []byte("20261029183001-newpara"))
	os.WriteFile(filepath.Join(filepath.Dir(file), id+".docj"), raw, 0600)
	if e := processSyncMergeResult(ctx, false, true, mr, &dejavu.TrafficStat{}, "fixture", time.Millisecond); e != nil {
		t.Fatal("merge retry", e)
	}
	if sql.GetBlock(id) == nil {
		t.Fatal("merge success before SQL flush")
	}
	if e := c.CloseCloudRepo(ctx); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "incremental-merge.json", map[string]interface{}{"originalMergeFailureReturned": true, "noSuccessFollowupOnFailure": true, "retry": true, "actualSQLCommittedBeforeSuccess": true})
}
