//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
)

func markdownSortFixture(t *testing.T) (*markdownImportAssets, string, string, []string) {
	t.Helper()
	a, box, _, _ := markdownAssetsFixture(t, false)
	m := a.source.preparation
	rows, err := m.archive.binding.scope.commit.r.registry.Query(m.ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
	if err != nil || len(rows) == 0 {
		t.Fatal(rows, err)
	}
	w, err := m.openDocumentWriter(box)
	if err != nil {
		t.Fatal(err)
	}
	id := ast.NewNodeID()
	tree := treenode.NewTree(box, "/"+id+".docj", "/Sort second", "Sort second")
	if err = w.write(m.ctx, box, tree); err != nil {
		w.close()
		t.Fatal(err)
	}
	if err = w.close(); err != nil {
		t.Fatal(err)
	}
	name := path.Join(box, ".siyuan", "sort.json")
	if err := a.target.Data.MkdirAll(path.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.target.Data.WriteFile(name, []byte(`{"retained":41}`), 0640); err != nil {
		t.Fatal(err)
	}
	return a, box, name, []string{rows[0].ID, id}
}
func TestMarkdownSortMergePreservesExistingValuesAndFrozenLegacyPayload(t *testing.T) {
	a, box, name, ids := markdownSortFixture(t)
	before, err := a.target.Data.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{ids[0]: 0, ids[1]: 1}
	notices, err := a.persistSort(box, values)
	if err != nil || len(notices) != 1 {
		t.Fatal(notices, err)
	}
	var merged map[string]int
	raw, err := a.target.Data.ReadFile(name)
	if err != nil || json.Unmarshal(raw, &merged) != nil || merged["retained"] != 41 || merged[ids[0]] != 0 || merged[ids[1]] != 1 {
		t.Fatal(merged, err)
	}
	after, err := a.target.Data.Stat(name)
	if err != nil || after.Mode().Perm() != before.Mode().Perm() || os.SameFile(before, after) {
		t.Fatal("sort publication metadata", err)
	}
	values[ids[0]] = 99
	delete(values, ids[1])
	var payload markdownSortChange
	if err := json.Unmarshal(notices[0].payload, &payload); err != nil || notices[0].kind != "filetree-sort" || payload.ParentPath != "/" || len(payload.ChildIDs) != 2 || payload.ChildIDs[0] != ids[0] || payload.ChildIDs[1] != ids[1] {
		t.Fatal(payload, err)
	}
}
func TestMarkdownSortMissingEmptyAndUnindexedValuesKeepOriginalPolicy(t *testing.T) {
	a, box, name, ids := markdownSortFixture(t)
	if err := a.target.Data.Remove(name); err != nil {
		t.Fatal(err)
	}
	if notices, err := a.persistSort(box, map[string]int{ids[0]: 0}); err != nil || len(notices) != 0 {
		t.Fatal(notices, err)
	}
	if _, err := a.target.Data.Lstat(name); !os.IsNotExist(err) {
		t.Fatal("missing sort file created", err)
	}
	if err := a.target.Data.WriteFile(name, []byte(`{"retained":41}`), 0640); err != nil {
		t.Fatal(err)
	}
	if notices, err := a.persistSort(box, nil); err != nil || len(notices) != 0 {
		t.Fatal(notices, err)
	}
	absent := ast.NewNodeID()
	notices, err := a.persistSort(box, map[string]int{absent: 5})
	if err != nil || len(notices) != 0 {
		t.Fatal(notices, err)
	}
	var values map[string]int
	raw, _ := a.target.Data.ReadFile(name)
	json.Unmarshal(raw, &values)
	if values[absent] != 5 || values["retained"] != 41 {
		t.Fatal(values)
	}
}
func TestMarkdownSortMalformedLinksAndPermissionsReturnErrorsWithoutNotice(t *testing.T) {
	for _, kind := range []string{"json", "null", "array", "link", "read-permission", "write-permission"} {
		t.Run(kind, func(t *testing.T) {
			a, box, name, ids := markdownSortFixture(t)
			file := filepath.Join(a.authority.binding.scope.commit.r.data, filepath.FromSlash(name))
			outside := ""
			switch kind {
			case "json":
				os.WriteFile(file, []byte("{broken"), 0640)
			case "null":
				os.WriteFile(file, []byte("null"), 0640)
			case "array":
				os.WriteFile(file, []byte("[]"), 0640)
			case "link":
				outside = filepath.Join(t.TempDir(), "foreign.json")
				os.WriteFile(outside, []byte(`{"foreign":9}`), 0600)
				os.Remove(file)
				os.Symlink(outside, file)
			case "read-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(file, 0000)
				defer os.Chmod(file, 0640)
			case "write-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(filepath.Dir(file), 0500)
				defer os.Chmod(filepath.Dir(file), 0755)
			}
			before, _ := os.ReadFile(file)
			notices, err := a.persistSort(box, map[string]int{ids[0]: 0})
			if err == nil || len(notices) != 0 {
				t.Fatal(notices, err)
			}
			if kind != "read-permission" {
				after, readErr := os.ReadFile(file)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Fatal("failed sort changed source", readErr)
				}
			}
			if outside != "" {
				data, err := os.ReadFile(outside)
				if err != nil || string(data) != `{"foreign":9}` {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestMarkdownSortRegistryErrorAndCancellationPreventPublication(t *testing.T) {
	for _, kind := range []string{"registry", "cancel", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			a, box, name, ids := markdownSortFixture(t)
			before, _ := a.target.Data.ReadFile(name)
			switch kind {
			case "registry":
				db := a.authority.binding.scope.commit.r.registry.DB
				if _, err := db.Exec("ALTER TABLE blocktrees RENAME TO sort_hidden"); err != nil {
					t.Fatal(err)
				}
				defer db.Exec("ALTER TABLE sort_hidden RENAME TO blocktrees")
			case "cancel":
				ctx, cancel := context.WithCancel(a.authority.ctx)
				a.source.preparation.ctx = ctx
				cancel()
			case "readonly":
				owner := a.authority.binding.scope.commit.r.owner
				owner.m.Lock()
				owner.persistenceReadOnly = true
				owner.m.Unlock()
				defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			}
			if notices, err := a.persistSort(box, map[string]int{ids[0]: 0}); err == nil || len(notices) != 0 {
				t.Fatal(notices, err)
			}
			after, _ := a.target.Data.ReadFile(name)
			if !bytes.Equal(before, after) {
				t.Fatal("failure published sort")
			}
		})
	}
}
func TestMarkdownSortCompletionRetainsSnapshotAcrossCandidateCleanup(t *testing.T) {
	a, box, _, ids := markdownSortFixture(t)
	m := a.source.preparation
	notices, err := a.persistSort(box, map[string]int{ids[0]: 0, ids[1]: 1})
	if err != nil {
		t.Fatal(err)
	}
	originalPayload := bytes.Clone(notices[0].payload)
	notices[0].delay = 50 * time.Millisecond
	m.completion = notices
	m.discard()
	a.close()
	a.source.close()
	events := make(chan commitNotice, 1)
	m.archive.binding.scope.commit.publish = func(n commitNotice) { events <- n }
	m.archive.binding.scope.commit.r.owner.syncRuntime.same.Store(7)
	if err = m.completeSync(); err != nil {
		t.Fatal(err)
	}
	for i := range notices[0].payload {
		notices[0].payload[i] = 'x'
	}
	if m.completion != nil || m.archive.binding.scope.commit.r.owner.syncRuntime.same.Load() != 0 {
		t.Fatal("completion state retained")
	}
	select {
	case event := <-events:
		if event.kind != "filetree-sort" || !bytes.Equal(event.payload, originalPayload) {
			t.Fatal(event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sort notice missing")
	}
}
func TestMarkdownSortActualImportCloseFailureDiscardsPendingNotice(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	r := m.archive.binding.scope.commit.r
	sortFile := filepath.Join(r.data, box, ".siyuan/sort.json")
	if err := os.MkdirAll(filepath.Dir(sortFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sortFile, []byte(`{"retained":41}`), 0640); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("source close failed after sort")
	m.archive.closeRoot = func(root *os.Root) error {
		err := root.Close()
		if filepath.Base(root.Name()) == "Knowledge" {
			return errors.Join(err, sentinel)
		}
		return err
	}
	events := make(chan commitNotice, 2)
	m.archive.binding.scope.commit.publish = func(n commitNotice) { events <- n }
	r.owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, folder, "/", "", true); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if m.completion != nil || r.owner.syncRuntime.same.Load() != 7 {
		t.Fatal("failed close completed sort")
	}
	select {
	case n := <-events:
		t.Fatal("failed import notified", n.kind)
	default:
	}
}
func TestMarkdownSortZIPCompletionWaitsForOwnedScratchCleanup(t *testing.T) {
	d, box, _, _ := markdownZIPFixture(t)
	m := d.preparation
	r := m.archive.binding.scope.commit.r
	sortFile := filepath.Join(r.data, box, ".siyuan/sort.json")
	os.MkdirAll(filepath.Dir(sortFile), 0755)
	os.WriteFile(sortFile, []byte(`{"retained":41}`), 0640)
	raw := markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0}, archiveMember{"Z.md", []byte("Z"), 0})
	events := make(chan bool, 1)
	m.archive.binding.scope.commit.publish = func(n commitNotice) {
		entries, err := os.ReadDir(m.archive.temp)
		clean := err == nil
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "wiki-extract-") || strings.HasPrefix(e.Name(), "wiki-markdown-upload-") {
				clean = false
			}
		}
		events <- n.kind == "filetree-sort" && clean
	}
	if err := d.ImportZIPReader(bytes.NewReader(raw), box, "/", "sorted.zip"); err != nil {
		t.Fatal(err)
	}
	if m.completion != nil {
		t.Fatal("ZIP completion retained")
	}
	select {
	case clean := <-events:
		if !clean {
			t.Fatal("notice before owned cleanup")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ZIP notice missing")
	}
}
func TestMarkdownSortPendingNoticeCanceledClosedAndChangedOwnerIsSuppressed(t *testing.T) {
	for _, kind := range []string{"cancel-before", "close-after", "owner-before"} {
		t.Run(kind, func(t *testing.T) {
			a, box, _, ids := markdownSortFixture(t)
			m := a.source.preparation
			notices, err := a.persistSort(box, map[string]int{ids[0]: 0})
			if err != nil {
				t.Fatal(err)
			}
			notices[0].delay = 100 * time.Millisecond
			m.completion = notices
			events := make(chan commitNotice, 1)
			m.archive.binding.scope.commit.publish = func(n commitNotice) { events <- n }
			if kind == "cancel-before" {
				ctx, cancel := context.WithCancel(m.ctx)
				m.ctx = ctx
				cancel()
				if err := m.completeSync(); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if kind == "owner-before" {
				old := Conf
				Conf = NewAppConf()
				defer func() { Conf = old }()
				if err := m.completeSync(); err == nil {
					t.Fatal("changed owner admitted notice")
				}
			} else {
				if err := m.completeSync(); err != nil {
					t.Fatal(err)
				}
				if kind == "close-after" {
					if err := m.archive.binding.scope.commit.r.owner.CloseCloudRepo(context.Background()); err != nil {
						t.Fatal(err)
					}
				}
			}
			if m.completion != nil {
				t.Fatal("failed/completed pending state retained")
			}
			select {
			case event := <-events:
				t.Fatal("stale notification", event.kind)
			case <-time.After(160 * time.Millisecond):
			}
		})
	}
}

func TestMarkdownSortZIPCleanupFailureDropsPendingCompletion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	d, box, _, _ := markdownZIPFixture(t)
	m := d.preparation
	r := m.archive.binding.scope.commit.r
	file := filepath.Join(r.data, box, ".siyuan/sort.json")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"retained":41}`), 0640); err != nil {
		t.Fatal(err)
	}
	original := d.importLocal
	d.importLocal = func(box, folder, to, label string) error {
		if err := original(box, folder, to, label); err != nil {
			return err
		}
		if len(m.completion) != 1 {
			t.Fatal("pending sort notice not prepared")
		}
		return os.Chmod(m.archive.temp, 0500)
	}
	defer os.Chmod(m.archive.temp, 0700)
	events := make(chan commitNotice, 1)
	m.archive.binding.scope.commit.publish = func(n commitNotice) { events <- n }
	r.owner.syncRuntime.same.Store(7)
	raw := markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0}, archiveMember{"Z.md", []byte("Z"), 0})
	if err := d.ImportZIPReader(bytes.NewReader(raw), box, "/", "failed-sort.zip"); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	if m.completion != nil || r.owner.syncRuntime.same.Load() != 7 {
		t.Fatal("failed cleanup completed")
	}
	select {
	case n := <-events:
		t.Fatal("failed cleanup notified", n.kind)
	default:
	}
}
