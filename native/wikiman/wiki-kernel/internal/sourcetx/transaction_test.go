package sourcetx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"wiki-kernel/internal/wikistore"
)

const p1 = "1760000000000000001"
const p2 = "1760000000000000002"
const docPath = "pages/" + p2 + "/data/20261019090000-box0001/20261019090001-doc0001.docj"

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	return root
}
func open(t *testing.T, root string) (*Store, *Writer) {
	t.Helper()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Writer(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, w
}
func bytesAt(t *testing.T, root, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func sourceHash(t *testing.T, root string) string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, name)
		if relative == ".local" && d.IsDir() {
			return fs.SkipDir
		}
		if !d.IsDir() {
			files[filepath.ToSlash(relative)] = Hash(bytesAt(t, root, relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(files)
	return Hash(raw)
}
func rename(t *testing.T, root, id, name, op string) Request {
	t.Helper()
	file := "pages/" + id + "/page.json"
	raw := bytesAt(t, root, file)
	var p wikistore.Page
	if json.Unmarshal(raw, &p) != nil {
		t.Fatal("bad fixture")
	}
	p.Name = name
	p.Revision++
	next, _ := json.Marshal(p)
	return Request{OperationID: op, Changes: []Change{{Path: file, ExpectedHash: Hash(raw), Data: next}}}
}
func pair(t *testing.T, root, op string) Request {
	r := rename(t, root, p1, "Changed root", op)
	raw := bytesAt(t, root, docPath)
	r.Changes = append(r.Changes, Change{Path: docPath, ExpectedHash: Hash(raw), Data: []byte(strings.Replace(string(raw), "Synthetic knowledge", "Changed knowledge", 1))})
	return r
}

func TestApplyProjectionCASAndDedup(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	initial, err := w.Rebuild(ctx, "all")
	if err != nil || !initial.TreeFresh || !initial.ContentFresh {
		t.Fatal(initial, err)
	}
	request := pair(t, root, "apply-pair")
	r, err := w.Apply(ctx, request, nil)
	if err != nil || !r.SourceCommitted || !r.IndexFresh {
		t.Fatal(r, err)
	}
	view, err := s.ProjectedTree(ctx)
	if err != nil || view.State.Generation <= initial.Generation {
		t.Fatal(view.State, err)
	}
	display, _ := view.Tree.Path(p2)
	if !strings.HasPrefix(display, "/Changed root/") {
		t.Fatal(display)
	}
	before := sourceHash(t, root)
	replay, err := w.Apply(ctx, request, nil)
	if err != nil || !reflect.DeepEqual(r, replay) || sourceHash(t, root) != before {
		t.Fatal(replay, err)
	}
	request.Changes[0].Data = append(request.Changes[0].Data, ' ')
	if _, err := w.Apply(ctx, request, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	rowsBefore := indexRows(t, root)
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal(err)
	}
	if after := indexRows(t, root); !reflect.DeepEqual(after, rowsBefore) {
		t.Fatal("incremental vs full rebuild mismatch", rowsBefore, after)
	}
	events := s.Events()
	found := false
	for _, event := range events {
		if event.OperationID == "apply-pair" && event.Kind == "pageTreeChanged" {
			found = true
		}
	}
	if !found {
		t.Fatal(events)
	}
}

func TestRegisteredStoreIdentityCannotBeRetargeted(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	req := rename(t, root, p1, "Must not publish", "foreign-store")
	name := filepath.Join(root, "store.json")
	raw := bytesAt(t, root, "store.json")
	raw = []byte(strings.Replace(string(raw), "f36f1884-2299-4fde-88c1-9da8f26d2aac", "536f1884-2299-4fde-88c1-9da8f26d2aac", 1))
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := sourceHash(t, root)
	ctx := context.Background()
	if err := s.Read(ctx, func() error { t.Fatal("foreign read callback ran"); return nil }); !errors.Is(err, ErrIdentity) {
		t.Fatal(err)
	}
	if _, err := w.Preview(ctx, req); !errors.Is(err, ErrIdentity) {
		t.Fatal(err)
	}
	if _, err := w.Apply(ctx, req, nil); !errors.Is(err, ErrIdentity) {
		t.Fatal(err)
	}
	if _, err := w.Rebuild(ctx, "all"); !errors.Is(err, ErrIdentity) {
		t.Fatal(err)
	}
	if _, err := w.Recover(ctx, nil); !errors.Is(err, ErrIdentity) {
		t.Fatal(err)
	}
	if before != sourceHash(t, root) {
		t.Fatal("wrote into replaced store identity")
	}
}

func indexRows(t *testing.T, root string) map[string][]string {
	t.Helper()
	db, err := sql.Open("sqlite3", sqliteDSN(filepath.Join(root, ".local/index/page.db"), "ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result := map[string][]string{}
	for table, query := range map[string]string{"pages": "SELECT record||'|'||path FROM pages ORDER BY id", "documents": "SELECT path||'|'||hash FROM documents ORDER BY path", "blocks": "SELECT path||'|'||id||'|'||start||'|'||end FROM blocks ORDER BY path,id", "refs": "SELECT path||'|'||source||'|'||target FROM refs ORDER BY path,source,target"} {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		result[table] = []string{}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], value)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return result
}

func TestCursorInvalidationSpecialPathsAndCancellableWriteQueue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wiki ? # % & ' 中文")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS("../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	s, w := open(t, root)
	ctx := context.Background()
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal(err)
	}
	view, err := s.ProjectedTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cursor := view.Cursor()
	if err := s.CheckCursor(ctx, cursor); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(ctx, rename(t, root, p1, "Cursor invalidation", "cursor-write"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckCursor(ctx, cursor); !errors.Is(err, ErrCursor) {
		t.Fatal(err)
	}
	<-w.queue
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := w.Rebuild(deadline, "tree"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	w.queue <- struct{}{}
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProjectedTree(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightRejectsInvalidGraphPathAndEntryWithoutSourceWrites(t *testing.T) {
	root := fixture(t)
	_, w := open(t, root)
	ctx := context.Background()
	before := sourceHash(t, root)
	r := rename(t, root, p1, "valid", "conflict")
	r.Changes[0].ExpectedHash = strings.Repeat("a", 64)
	if _, err := w.Apply(ctx, r, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	r = rename(t, root, p1, "cycle", "cycle")
	var p wikistore.Page
	_ = json.Unmarshal(r.Changes[0].Data, &p)
	id := p2
	p.ParentID = &id
	r.Changes[0].Data, _ = json.Marshal(p)
	if _, err := w.Apply(ctx, r, nil); err == nil {
		t.Fatal("accepted cycle")
	}
	r = Request{OperationID: "entry-delete", Changes: []Change{{Path: docPath, ExpectedHash: Hash(bytesAt(t, root, docPath)), Delete: true}}}
	if _, err := w.Apply(ctx, r, nil); err == nil {
		t.Fatal("deleted active entry")
	}
	for i, name := range []string{"../outside", ".local/locks/source.lock", "pages/" + p1 + "/page.json/child", "pages/" + p1 + "/data/../../outside"} {
		r = Request{OperationID: "unsafe" + string(rune('a'+i)), Changes: []Change{{Path: name, Data: []byte("x")}}}
		if _, err := w.Apply(ctx, r, nil); err == nil {
			t.Fatal("accepted unsafe path", name)
		}
	}
	if sourceHash(t, root) != before {
		t.Fatal("preflight failure modified source")
	}
	entries, err := os.ReadDir(filepath.Join(root, ".local/journal"))
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}

func TestOverlayEntryRejectsUnstableEnumerationWithoutSourceWrites(t *testing.T) {
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	page := wikistore.Page{PageID: p2, EntryDocument: &wikistore.EntryDocument{BoxID: box, DocID: doc}}

	t.Run("incomplete confirmation", func(t *testing.T) {
		rootPath := fixture(t)
		before := sourceHash(t, rootPath)
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		base := "pages/" + p2 + "/data/" + box
		calls := 0
		incomplete := errors.New("incomplete enumeration")
		op := readManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			if name == base {
				calls++
				if calls == 2 {
					return nil, incomplete
				}
			}
			return fs.ReadDir(root.FS(), name)
		}}
		err = overlayEntryWithIO(root, nil, op)(context.Background(), rootPath, page)
		if !errors.Is(err, incomplete) || sourceHash(t, rootPath) != before {
			t.Fatal(err)
		}
	})

	t.Run("same metadata replacement", func(t *testing.T) {
		rootPath := fixture(t)
		before := sourceHash(t, rootPath)
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		calls := 0
		op := readManifestIO{lstat: func(root *os.Root, name string) (fs.FileInfo, error) {
			if name == docPath {
				calls++
				if calls == 3 {
					replaceReadManifestTarget(t, rootPath, name)
				}
			}
			return root.Lstat(name)
		}}
		err = overlayEntryWithIO(root, nil, op)(context.Background(), rootPath, page)
		if !errors.Is(err, errReadSourceChanged) || sourceHash(t, rootPath) == before {
			t.Fatal(err)
		}
	})
}

func TestOverlayEntryPreservesOverlayHiddenAndLinkRules(t *testing.T) {
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	page := wikistore.Page{PageID: p2, EntryDocument: &wikistore.EntryDocument{BoxID: box, DocID: doc}}

	t.Run("missing source supplied by overlay", func(t *testing.T) {
		rootPath := fixture(t)
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		page.EntryDocument.BoxID = "20261019090000-absent1"
		name := "pages/" + p2 + "/data/" + page.EntryDocument.BoxID + "/" + doc + ".docj"
		if err := overlayEntry(root, map[string]Change{name: {Path: name, Data: []byte("{}")}})(context.Background(), rootPath, page); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("hidden duplicate remains visible", func(t *testing.T) {
		rootPath := fixture(t)
		hidden := filepath.Join(rootPath, "pages", p2, "data", box, ".hidden", doc+".docj")
		if err := os.MkdirAll(filepath.Dir(hidden), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hidden, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := overlayEntry(root, nil)(context.Background(), rootPath, page); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	})

	t.Run("symlink rejected", func(t *testing.T) {
		rootPath := fixture(t)
		link := filepath.Join(rootPath, "pages", p2, "data", box, "linked")
		if err := os.Symlink(filepath.Join(rootPath, docPath), link); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := overlayEntry(root, nil)(context.Background(), rootPath, page); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	})
}

func TestRecoveryAndReaderAdmissionAtEveryPublicationBoundary(t *testing.T) {
	for _, point := range []string{"prepared", "published:0", "published:1", "committed"} {
		t.Run(point, func(t *testing.T) {
			root := fixture(t)
			s, w := open(t, root)
			ctx := context.Background()
			before := sourceHash(t, root)
			_, err := w.Apply(ctx, pair(t, root, "interrupt"), func(at string) error {
				if at == point {
					return syscall.ENOSPC
				}
				return nil
			})
			if !errors.Is(err, syscall.ENOSPC) {
				t.Fatal(err)
			}
			called := false
			if err := s.Read(ctx, func() error { called = true; return nil }); !errors.Is(err, ErrRecovery) || called {
				t.Fatal("reader exposed unfinished journal", called, err)
			}
			result, err := w.Recover(ctx, nil)
			if err != nil || len(result) != 1 || !result[0].IndexFresh {
				t.Fatal(result, err)
			}
			committed := point == "committed"
			if result[0].SourceCommitted != committed {
				t.Fatal(result)
			}
			if (sourceHash(t, root) == before) == committed {
				t.Fatal("incorrect rollback/rollforward", point)
			}
			after := sourceHash(t, root)
			again, err := w.Recover(ctx, nil)
			if err != nil || len(again) != 0 || after != sourceHash(t, root) {
				t.Fatal("recovery not idempotent", again, err)
			}
			if _, err := s.ProjectedTree(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectionFailureReportsCommittedAndRebuildsWithoutReplayingSource(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	r, err := w.Apply(ctx, pair(t, root, "stale-index"), func(at string) error {
		if at == "projection" {
			return syscall.ENOSPC
		}
		return nil
	})
	if !errors.Is(err, ErrIndex) || !r.SourceCommitted || r.IndexFresh {
		t.Fatal(r, err)
	}
	if _, err := s.ProjectedTree(ctx); !errors.Is(err, ErrIndex) {
		t.Fatal(err)
	}
	if _, err := s.ScanTree(ctx); err != nil {
		t.Fatal("consistent source should still be readable", err)
	}
	before := sourceHash(t, root)
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal(err)
	}
	if sourceHash(t, root) != before {
		t.Fatal("index rebuild wrote source")
	}
	if _, err := s.ProjectedTree(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".local/index/page.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProjectedTree(ctx); err == nil {
		t.Fatal("reported missing DB fresh")
	}
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".local/index/page.db"), []byte("corrupt derived database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Rebuild(ctx, "all"); err != nil {
		t.Fatal("could not rebuild corrupt projection", err)
	}
}

func TestRecoveryConflictAndCorruptMaterialNeverOverwriteForeignData(t *testing.T) {
	for _, variant := range []string{"target", "image", "duplicate-key", "path"} {
		t.Run(variant, func(t *testing.T) {
			root := fixture(t)
			s, w := open(t, root)
			ctx := context.Background()
			_, err := w.Apply(ctx, pair(t, root, "damaged"), func(at string) error {
				if at == "published:0" {
					return errors.New("interrupt")
				}
				return nil
			})
			if err == nil {
				t.Fatal("did not interrupt")
			}
			switch variant {
			case "target":
				if err := os.WriteFile(filepath.Join(root, docPath), []byte("external edit must survive"), 0600); err != nil {
					t.Fatal(err)
				}
			case "image":
				if err := os.WriteFile(filepath.Join(root, ".local/staging/damaged/before/1"), []byte("bad image"), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-key":
				raw := bytesAt(t, root, journalPath("damaged"))
				raw = append([]byte(`{"version":1,`), raw[1:]...)
				if err := os.WriteFile(filepath.Join(root, journalPath("damaged")), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "path":
				raw := bytesAt(t, root, journalPath("damaged"))
				raw = []byte(strings.Replace(string(raw), "pages/"+p1+"/page.json", "../outside", 1))
				if err := os.WriteFile(filepath.Join(root, journalPath("damaged")), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := sourceHash(t, root)
			if _, err := w.Recover(ctx, nil); err == nil {
				t.Fatal("accepted corrupt recovery")
			}
			if sourceHash(t, root) != before {
				t.Fatal("recovery changed source before complete validation")
			}
			if err := s.Read(ctx, func() error { return nil }); err == nil {
				t.Fatal("failed recovery reopened read gate")
			}
		})
	}
}

func TestRebuildScopesAndNeverUnlinkWAL(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	st, err := w.Rebuild(ctx, "tree")
	if err != nil || !st.TreeFresh || st.ContentFresh {
		t.Fatal(st, err)
	}
	st, err = w.Rebuild(ctx, "content")
	if err != nil || !st.ContentFresh {
		t.Fatal(st, err)
	}
	before := sourceHash(t, root)
	wal := filepath.Join(root, ".local/index/page.db-wal")
	if err := os.WriteFile(wal, []byte("foreign WAL"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Rebuild(ctx, "all"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if string(bytesAt(t, root, ".local/index/page.db-wal")) != "foreign WAL" || sourceHash(t, root) != before {
		t.Fatal("WAL or source overwritten")
	}
	if _, err := s.ProjectedTree(ctx); !errors.Is(err, ErrIndex) {
		t.Fatal("stale rebuild gate opened", err)
	}
}

func TestLocksAliasesReadersAndCancellation(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	other, err := Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Writer(ctx, false); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := other.Writer(deadline, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := s.Read(ctx, func() error { return nil }); err != nil {
		t.Fatal("writer session blocked shared source reader", err)
	}
	before, err := os.Stat(filepath.Join(root, ".local/locks/session.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := other.Writer(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	after, err := os.Stat(filepath.Join(root, ".local/locks/session.lock"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("lock inode replaced", err)
	}
}

func TestInterruptedPreparationCanResumeSameOperationWithoutTouchingSource(t *testing.T) {
	for _, point := range []string{"staging", "staged:0"} {
		t.Run(point, func(t *testing.T) {
			root := fixture(t)
			s, w := open(t, root)
			ctx := context.Background()
			before := sourceHash(t, root)
			req := pair(t, root, "resume-prepare")
			if _, err := w.Apply(ctx, req, func(at string) error {
				if at == point {
					return syscall.ENOSPC
				}
				return nil
			}); !errors.Is(err, syscall.ENOSPC) {
				t.Fatal(err)
			}
			if sourceHash(t, root) != before {
				t.Fatal("pre-journal failure wrote source")
			}
			if err := s.Read(ctx, func() error { return nil }); err != nil {
				t.Fatal("preparation-only source must be readable", err)
			}
			r, err := w.Apply(ctx, req, nil)
			if err != nil || !r.SourceCommitted || !r.IndexFresh {
				t.Fatal(r, err)
			}
		})
	}
}

func TestIncrementalCreateMoveRankDeleteRestoreMatchesFullRebuild(t *testing.T) {
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	p3 := "1760000000000000003"
	p4 := "1760000000000000004"
	p99 := "1760000000000000099"
	change := func(id string, edit func(*wikistore.Page)) Change {
		name := "pages/" + id + "/page.json"
		raw := bytesAt(t, root, name)
		var p wikistore.Page
		_ = json.Unmarshal(raw, &p)
		edit(&p)
		p.Revision++
		next, _ := json.Marshal(p)
		return Change{Path: name, ExpectedHash: Hash(raw), Data: next}
	}
	apply := func(op string, changes ...Change) {
		t.Helper()
		r, err := w.Apply(ctx, Request{OperationID: op, Changes: changes}, nil)
		if err != nil || !r.IndexFresh {
			t.Fatal(op, r, err)
		}
		got := indexRows(t, root)
		before := sourceHash(t, root)
		if _, err := w.Rebuild(ctx, "all"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, indexRows(t, root)) || sourceHash(t, root) != before {
			t.Fatal("incremental differs from rebuild", op)
		}
		if _, err := s.ProjectedTree(ctx); err != nil {
			t.Fatal(err)
		}
	}
	apply("move", change(p3, func(p *wikistore.Page) { p.ParentID = &p4; p.Rank = "0000" }))
	raw := bytesAt(t, root, "pages/"+p4+"/page.json")
	var created wikistore.Page
	_ = json.Unmarshal(raw, &created)
	created.PageID = p99
	created.ParentID = &p4
	created.Name = "New nested page"
	created.Rank = "0002"
	created.Revision = 1
	next, _ := json.Marshal(created)
	apply("create", Change{Path: "pages/" + p99 + "/page.json", Data: next})
	apply("rename-parent", change(p4, func(p *wikistore.Page) { p.Name = "New parent path" }))
	deleted := "2026-10-19T12:00:00Z"
	apply("subtree-delete", change(p3, func(p *wikistore.Page) { p.DeletedAt = &deleted }), change(p4, func(p *wikistore.Page) { p.DeletedAt = &deleted }), change(p99, func(p *wikistore.Page) { p.DeletedAt = &deleted }))
	apply("subtree-restore", change(p3, func(p *wikistore.Page) { p.DeletedAt = nil }), change(p4, func(p *wikistore.Page) { p.DeletedAt = nil }), change(p99, func(p *wikistore.Page) { p.DeletedAt = nil }))
	apply("reorder", change(p3, func(p *wikistore.Page) { p.Rank = "0004" }), change(p99, func(p *wikistore.Page) { p.Rank = "0003" }))
}

func TestPublicationAndRollbackPreserveBasicFilePermissions(t *testing.T) {
	root := fixture(t)
	_, w := open(t, root)
	ctx := context.Background()
	name := "pages/" + p1 + "/data/script.sh"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic script before"), 0750); err != nil {
		t.Fatal(err)
	}
	before := bytesAt(t, root, name)
	_, err := w.Apply(ctx, Request{OperationID: "file-permissions", Changes: []Change{{Path: name, ExpectedHash: Hash(before), Data: []byte("synthetic script after")}}}, func(at string) error {
		if at == "published:0" {
			return syscall.ENOSPC
		}
		return nil
	})
	if err == nil {
		t.Fatal("did not interrupt")
	}
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatal(info, err)
	}
	if _, err := w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(filepath.Join(root, name))
	if err != nil || info.Mode().Perm() != 0750 || string(bytesAt(t, root, name)) != string(before) {
		t.Fatal(info, err)
	}
}
