//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func mountDoc(id, title string) []byte {
	p := ast.NewNodeID()
	raw, _ := json.Marshal(map[string]interface{}{"ID": id, "Type": "NodeDocument", "Spec": "1", "Properties": map[string]string{"id": id, "title": title}, "Children": []interface{}{map[string]interface{}{"ID": p, "Type": "NodeParagraph", "Properties": map[string]string{"id": p}, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Imported content"}}}}})
	return raw
}
func TestArchiveMountRootAndNestedTargetKeepOriginalPathsAndJoinedIndexes(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested"}[nested], func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			to := "/"
			prefix := ""
			if nested {
				to = "/" + filepath.Base(source)
				prefix = strings.TrimSuffix(to, util.DocExt)
			}
			folder := t.TempDir()
			parent, child, hidden := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
			archiveWrite(t, folder, parent+".docj", mountDoc(parent, "Parent"))
			archiveWrite(t, folder, parent+"/"+child+".docj", mountDoc(child, "Child"))
			archiveWrite(t, folder, ".hidden/"+hidden+".docj", mountDoc(hidden, "Hidden"))
			archiveWrite(t, folder, "extra/file", []byte("other"))
			if err := e.mountArchive(folder, box, to, e.binding.scope.commit.parser()); err != nil {
				t.Fatal(err)
			}
			r := e.binding.scope.commit.r
			for id, p := range map[string]string{parent: prefix + "/" + parent + ".docj", child: prefix + "/" + parent + "/" + child + ".docj"} {
				if _, err := os.Stat(filepath.Join(r.data, box, p)); err != nil {
					t.Fatal(err)
				}
				rows, err := r.registry.Query(e.ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
				if err != nil || len(rows) != 1 || rows[0].Path != p || rows[0].BoxID != box {
					t.Fatal(rows, err)
				}
				var n int
				if err := r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&n); err != nil || n != 1 {
					t.Fatal("returned before index", id, n, err)
				}
			}
			var n int
			r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", hidden).Scan(&n)
			if n != 0 {
				t.Fatal("hidden source indexed")
			}
			base := filepath.Join(r.data, box, prefix)
			if _, err := os.Stat(filepath.Join(base, ".hidden", hidden+".docj")); err != nil {
				t.Fatal("original hidden copy rule", err)
			}
			raw, _ := os.ReadFile(filepath.Join(base, "extra/file"))
			if string(raw) != "other" {
				t.Fatal("non-document copy lost")
			}
		})
	}
}
func TestArchiveMountMissingTargetAndRegistryErrorDoNotPublish(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "sql"}[broken], func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			folder := t.TempDir()
			id := ast.NewNodeID()
			archiveWrite(t, folder, id+".docj", mountDoc(id, "Not mounted"))
			r := e.binding.scope.commit.r
			if broken {
				if _, err := r.registry.DB.Exec("ALTER TABLE blocktrees RENAME TO hidden_mount_probe"); err != nil {
					t.Fatal(err)
				}
				defer r.registry.DB.Exec("ALTER TABLE hidden_mount_probe RENAME TO blocktrees")
			}
			if err := e.mountArchive(folder, box, "/20261031110000-missing.docj", e.binding.scope.commit.parser()); err == nil {
				t.Fatal("missing/failed target became success")
			}
			if _, err := os.Stat(filepath.Join(r.data, box, id+".docj")); !os.IsNotExist(err) {
				t.Fatal("failed target copied documents", err)
			}
		})
	}
}
func TestArchiveMountLinkedTargetAndOldOwnerReject(t *testing.T) {
	for _, kind := range []string{"linked-target", "cancel", "old-root", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			folder := t.TempDir()
			id := ast.NewNodeID()
			archiveWrite(t, folder, id+".docj", mountDoc(id, "Rejected"))
			to := "/"
			switch kind {
			case "linked-target":
				to = "/" + filepath.Base(source)
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(util.DataDir, box, strings.TrimSuffix(filepath.Base(source), util.DocExt))); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.ctx = ctx
				e.binding.scope.ctx = ctx
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			}
			if err := e.finishArchive(folder, box, to, e.binding.scope.commit.parser(), map[string]string{"old": id}); err == nil {
				t.Fatal("invalid mount completed")
			}
			if Conf.repoTasks != nil {
				t.Fatal("failure admitted success")
			}
		})
	}
}
func TestArchiveMountIndexFailureAndLaterTreeFailureDrainWithoutSuccess(t *testing.T) {
	for _, kind := range []string{"main-sql", "registry-sql", "later-tree"} {
		t.Run(kind, func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			folder := t.TempDir()
			id := "20261031111000-doc0001"
			archiveWrite(t, folder, id+".docj", mountDoc(id, "Partial"))
			r := e.binding.scope.commit.r
			r.owner.syncRuntime.same.Store(9)
			switch kind {
			case "main-sql":
				if _, err := r.main.DB.Exec("CREATE TRIGGER fail_mount BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'mount SQL failure'); END"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					r.main.DB.Exec("DROP TRIGGER fail_mount")
					if err := (sql.LegacyMainQueue{DB: r.main.DB}).Flush(context.Background()); err != nil {
						t.Error(err)
					}
				}()
			case "registry-sql":
				if _, err := r.registry.DB.Exec("CREATE TRIGGER fail_mount BEFORE INSERT ON blocktrees BEGIN SELECT RAISE(ABORT,'registry mount failure'); END"); err != nil {
					t.Fatal(err)
				}
				defer r.registry.DB.Exec("DROP TRIGGER fail_mount")
			case "later-tree":
				archiveWrite(t, folder, "20261031111001-doc0002.docj", []byte("bad tree"))
			}
			if err := e.finishArchive(folder, box, "/", e.binding.scope.commit.parser(), map[string]string{"old": id}); err == nil {
				t.Fatal("failure swallowed")
			}
			if _, err := os.Stat(filepath.Join(r.data, box, id+".docj")); err != nil {
				t.Fatal("expected earlier copied source", err)
			}
			if Conf.repoTasks != nil || r.owner.syncRuntime.same.Load() != 9 {
				t.Fatal("failure published success or planned sync")
			}
			if kind == "later-tree" {
				var n int
				r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&n)
				if n != 1 {
					t.Fatal("admitted first index not drained")
				}
			}
		})
	}
}
func TestArchiveMountSuccessNoticeUsesFrozenIDsAfterIndexAndCapturedSync(t *testing.T) {
	e, source, _ := archiveEntryFixture(t)
	box := filepath.Base(filepath.Dir(source))
	folder := t.TempDir()
	id := ast.NewNodeID()
	archiveWrite(t, folder, id+".docj", mountDoc(id, "Success"))
	c := e.binding.scope.commit
	events := make(chan commitNotice, 2)
	c.publish = func(n commitNotice) {
		var count int
		c.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&count)
		if count != 1 {
			t.Error("notice before index committed")
		}
		events <- n
	}
	c.r.owner.syncRuntime.same.Store(7)
	ids := map[string]string{"old": id}
	if err := e.finishArchive(folder, box, "/", c.parser(), ids); err != nil {
		t.Fatal(err)
	}
	ids["old"] = "late mutation"
	if c.r.owner.syncRuntime.same.Load() != 0 {
		t.Fatal("captured sync not planned")
	}
	select {
	case n := <-events:
		if n.kind != "ids" || !bytes.Contains(n.payload, []byte(id)) || bytes.Contains(n.payload, []byte("late mutation")) {
			t.Fatal(n.kind, string(n.payload))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owner notification missing")
	}
}
func TestArchiveMountCompletionRejectsUnboundCanceledAndClosed(t *testing.T) {
	if err := (*ArchiveImportContext)(nil).completeArchive(nil); err == nil {
		t.Fatal("unbound completion")
	}
	for _, closed := range []bool{false, true} {
		e, _, _ := archiveEntryFixture(t)
		if closed {
			if err := Conf.CloseCloudRepo(context.Background()); err != nil {
				t.Fatal(err)
			}
		} else {
			ctx, cancel := context.WithCancel(context.Background())
			e.ctx = ctx
			e.binding.scope.ctx = ctx
			cancel()
		}
		if err := e.completeArchive(nil); err == nil {
			t.Fatal("invalid success admitted")
		}
		if Conf.repoTasks != nil {
			t.Fatal("invalid completion created tasks")
		}
	}
	if _, err := CaptureArchiveImport(nil); err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("nil context admitted", err)
	}
}

func TestArchiveMountMissingParentRepairAndSourceScanErrors(t *testing.T) {
	for _, kind := range []string{"missing-parent", "source-link"} {
		t.Run(kind, func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			folder := t.TempDir()
			parent, child := ast.NewNodeID(), ast.NewNodeID()
			archiveWrite(t, folder, parent+"/"+child+".docj", mountDoc(child, "Child"))
			if kind == "source-link" {
				outside := archiveWrite(t, t.TempDir(), "external", mountDoc(ast.NewNodeID(), "Outside"))
				if err := os.Symlink(outside, filepath.Join(folder, "linked.docj")); err != nil {
					t.Fatal(err)
				}
				if err := e.mountArchive(folder, box, "/", e.binding.scope.commit.parser()); err == nil {
					t.Fatal("source link accepted")
				}
				if _, err := os.Stat(filepath.Join(util.DataDir, box, parent)); !os.IsNotExist(err) {
					t.Fatal("scan error published target", err)
				}
				return
			}
			if err := e.mountArchive(folder, box, "/", e.binding.scope.commit.parser()); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{parent, child} {
				var n int
				if err := e.binding.scope.commit.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&n); err != nil || n != 1 {
					t.Fatal("repaired parent index missing", id, n, err)
				}
			}
		})
	}
}

func TestArchiveMountCheckedCandidateManifestFailures(t *testing.T) {
	for _, kind := range []string{"walk-incomplete", "scan-replaced", "post-merge-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, source, _ := archiveEntryFixture(t)
			box := filepath.Base(filepath.Dir(source))
			folder := t.TempDir()
			id := ast.NewNodeID()
			name := id + ".docj"
			archiveWrite(t, folder, name, mountDoc(id, "Checked mount"))
			archiveWrite(t, folder, "zz/other", []byte("later"))
			merged := false
			op := archiveMountIO{}
			switch kind {
			case "walk-incomplete":
				sentinel := errors.New("mount walk incomplete")
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "zz" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, sentinel)
					}
					return source.ReadDir(ctx, current)
				}
				op.merge = func(string, string) error { merged = true; return nil }
			case "scan-replaced":
				replaced := false
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "zz" && !replaced {
						replaced = true
						if err := os.Remove(filepath.Join(folder, name)); err != nil {
							t.Fatal(err)
						}
						archiveWrite(t, folder, name, mountDoc(ast.NewNodeID(), "Replacement"))
					}
					return source.ReadDir(ctx, current)
				}
				op.merge = func(string, string) error { merged = true; return nil }
			case "post-merge-replaced":
				op.merge = func(from, target string) error {
					merged = true
					if err := e.mergeArchiveDirectory(from, target); err != nil {
						return err
					}
					if err := os.Remove(filepath.Join(folder, name)); err != nil {
						return err
					}
					archiveWrite(t, folder, name, mountDoc(ast.NewNodeID(), "Replacement"))
					return nil
				}
			}
			err := e.mountArchiveWithIO(folder, box, "/", e.binding.scope.commit.parser(), op)
			if err == nil {
				t.Fatal("mount candidate failure became success")
			}
			if kind == "walk-incomplete" && (indexrepair.IsSourceNotExist(err) || merged) {
				t.Fatal("incomplete scan admitted merge or absence", merged, err)
			}
			if kind == "scan-replaced" && merged {
				t.Fatal("changed scan admitted merge")
			}
			if kind == "post-merge-replaced" && !merged {
				t.Fatal("post-merge check was not exercised")
			}
			var count int
			if queryErr := e.binding.scope.commit.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&count); queryErr != nil || count != 0 {
				t.Fatal("failed candidate scan admitted index", count, queryErr)
			}
		})
	}
}

func TestArchiveMountIDNoticesRemainSerialAndCloseJoinsPending(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	c := e.binding.scope.commit
	events := make(chan string, 4)
	c.publish = func(n commitNotice) {
		var ids map[string]string
		if json.Unmarshal(n.payload, &ids) == nil {
			events <- ids["id"]
		}
	}
	first, _ := json.Marshal(map[string]string{"id": "first"})
	if err := c.completeUI(e.ctx, []commitNotice{{kind: "ids", payload: first, delay: 100 * time.Millisecond}}, true); err != nil {
		t.Fatal(err)
	}
	if err := e.completeArchive(map[string]string{"id": "second"}); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-events:
		t.Fatal("later ID notice overtook delayed first", value)
	case <-time.After(40 * time.Millisecond):
	}
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatal(got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing ordered notice", want)
		}
	}
	pending, _ := json.Marshal(map[string]string{"id": "pending"})
	if err := c.completeUI(e.ctx, []commitNotice{{kind: "ids", payload: pending, delay: time.Hour}}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.completeArchive(map[string]string{"id": "after-pending"}); err != nil {
		t.Fatal(err)
	}
	if err := Conf.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-Conf.archiveIDNoticeTail:
	default:
		t.Fatal("ID task lifetime not joined")
	}
	select {
	case value := <-events:
		t.Fatal("closed owner published pending IDs", value)
	default:
	}
}
