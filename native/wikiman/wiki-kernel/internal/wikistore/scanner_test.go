package wikistore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const rootPage = "1760000000000000001"
const docPage = "1760000000000000002"
const tablePage = "1760000000000000003"
const otherRoot = "1760000000000000004"

func fixture(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(file), "../../../fixtures/shared-wiki/valid")
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return root
}

func metadataPath(root, id string) string { return filepath.Join(root, "pages", id, "page.json") }

func editPage(t *testing.T, root, id string, edit func(map[string]any)) {
	t.Helper()
	p := metadataPath(root, id)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	edit(value)
	data, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func fileHashes(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		result[rel] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestScanStableDetachedAndReadOnly(t *testing.T) {
	root := fixture(t)
	before := fileHashes(t, root)
	a, err := (Scanner{}).Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (Scanner{}).Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceDigest() != b.SourceDigest() || len(a.SourceDigest()) != 64 {
		t.Fatal("unstable digest")
	}
	if !reflect.DeepEqual(a.Children("", false), []string{rootPage, otherRoot}) {
		t.Fatal("root ordering")
	}
	if !reflect.DeepEqual(a.Children(rootPage, false), []string{docPage, tablePage}) {
		t.Fatal("sibling ordering")
	}
	if name, _ := a.Path(docPage); name != "/技术知识/单文档示例" {
		t.Fatal(name)
	}
	if p, ok := a.Get(tablePage); !ok || p.Type != "table" {
		t.Fatal("empty node lost")
	}
	p, _ := a.Get(docPage)
	*p.ParentID = otherRoot
	p.EntryDocument.DocID = "changed"
	p.Ext[0] = '['
	check, _ := a.Get(docPage)
	if *check.ParentID != rootPage || check.EntryDocument.DocID == "changed" || string(check.Ext) != "{}" {
		t.Fatal("snapshot mutable through record")
	}
	if !reflect.DeepEqual(a.Diagnostics(), b.Diagnostics()) {
		t.Fatal("unstable diagnostics")
	}
	if !reflect.DeepEqual(before, fileHashes(t, root)) {
		t.Fatal("scan changed files")
	}
}

func TestMetadataFailuresDoNotPublishPartialTree(t *testing.T) {
	cases := []struct {
		name, code string
		mutate     func(*testing.T, string)
	}{
		{"numeric identity", "invalid_metadata", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["pageId"] = float64(1760000000000000002) })
		}},
		{"wrong directory identity", "page_identity_mismatch", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["pageId"] = otherRoot })
		}},
		{"duplicate identity", "duplicate_page_id", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["pageId"] = rootPage })
		}},
		{"missing parent", "missing_parent", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["parentId"] = "1760000000000000099" })
		}},
		{"self cycle", "page_cycle", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["parentId"] = docPage })
		}},
		{"disconnected cycle", "page_cycle", func(t *testing.T, r string) {
			editPage(t, r, rootPage, func(p map[string]any) { p["parentId"] = docPage })
		}},
		{"deleted parent", "deleted_parent", func(t *testing.T, r string) {
			editPage(t, r, rootPage, func(p map[string]any) { p["deletedAt"] = "2026-10-19T02:00:00Z" })
		}},
		{"invalid type", "invalid_page_type", func(t *testing.T, r string) { editPage(t, r, docPage, func(p map[string]any) { p["type"] = "folder" }) }},
		{"invalid rank", "invalid_name_or_rank", func(t *testing.T, r string) { editPage(t, r, docPage, func(p map[string]any) { p["rank"] = "" }) }},
		{"invalid revision", "invalid_revision", func(t *testing.T, r string) { editPage(t, r, docPage, func(p map[string]any) { p["revision"] = 0 }) }},
		{"unknown format", "unsupported_schema", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["schemaVersion"] = 2 })
		}},
		{"non UTC timestamp", "invalid_timestamp", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["updatedAt"] = "2026-10-19T10:00:00+08:00" })
		}},
		{"ext array", "invalid_ext", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["ext"] = []string{} })
		}},
		{"entry path escape", "invalid_entry_document", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) { p["entryDocument"] = map[string]any{"boxId": "../..", "docId": "bad"} })
		}},
		{"missing entry", "invalid_entry_document", func(t *testing.T, r string) {
			editPage(t, r, docPage, func(p map[string]any) {
				p["entryDocument"] = map[string]any{"boxId": "20261019090000-box0001", "docId": "20261019090009-missing"}
			})
		}},
		{"orphan directory", "missing_or_unsafe_metadata", func(t *testing.T, r string) {
			if err := os.Remove(metadataPath(r, tablePage)); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate key", "invalid_metadata", func(t *testing.T, r string) {
			p := metadataPath(r, docPage)
			d, _ := os.ReadFile(p)
			d = []byte(strings.Replace(string(d), `"schemaVersion": 1`, `"schemaVersion": 1, "schemaVersion": 1`, 1))
			if err := os.WriteFile(p, d, 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"nested duplicate key", "invalid_metadata", func(t *testing.T, r string) {
			p := metadataPath(r, docPage)
			d, _ := os.ReadFile(p)
			d = []byte(strings.Replace(string(d), `"ext": {}`, `"ext": {"owner":1,"owner":2}`, 1))
			if err := os.WriteFile(p, d, 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"trailing JSON", "invalid_metadata", func(t *testing.T, r string) {
			p := metadataPath(r, docPage)
			d, _ := os.ReadFile(p)
			if err := os.WriteFile(p, append(d, []byte(` {}`)...), 0600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fixture(t)
			tc.mutate(t, r)
			before := fileHashes(t, r)
			snap, err := (Scanner{}).Scan(context.Background(), r)
			if snap != nil {
				t.Fatal("published invalid subset")
			}
			var failure *ScanError
			if !errors.As(err, &failure) {
				t.Fatalf("expected diagnostics, got %v", err)
			}
			found := false
			for _, d := range failure.Diagnostics {
				if d.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("wanted %s, got %+v", tc.code, failure.Diagnostics)
			}
			if !reflect.DeepEqual(before, fileHashes(t, r)) {
				t.Fatal("failure mutated source")
			}
		})
	}
}

func TestSourceDigestUsesBytesNotMtimeAndIgnoresBody(t *testing.T) {
	r := fixture(t)
	a, err := (Scanner{}).Scan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	p := metadataPath(r, rootPage)
	info, _ := os.Stat(p)
	editPage(t, r, rootPage, func(v map[string]any) { v["name"] = "另一个名" })
	if err := os.Chtimes(p, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	b, err := (Scanner{}).Scan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceDigest() == b.SourceDigest() {
		t.Fatal("same mtime hid change")
	}
	body := filepath.Join(r, "pages", docPage, "data", "20261019090000-box0001", "20261019090001-doc0001.docj")
	if err := os.WriteFile(body, []byte(`{"body":"not read by tree scan"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := (Scanner{}).Scan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if b.SourceDigest() != c.SourceDigest() {
		t.Fatal("body edit changed page tree digest")
	}
}

func TestLimitsCancellationAndSymlinks(t *testing.T) {
	r := fixture(t)
	for _, s := range []Scanner{{MaxPages: 3}, {MaxMetadataBytes: 8}, {MaxTotalMetadataBytes: 256}, {MaxDepth: 1}} {
		if _, err := s.Scan(context.Background(), r); err == nil {
			t.Fatalf("missing limit: %+v", s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Scanner{}).Scan(ctx, r); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(r, alias); err != nil {
		t.Fatal(err)
	}
	snap, err := (Scanner{}).Scan(context.Background(), alias)
	realRoot, resolveErr := filepath.EvalSymlinks(r)
	if err != nil || resolveErr != nil || snap.Root() != realRoot {
		t.Fatalf("root alias: %v", err)
	}
	entry := metadataPath(r, tablePage)
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(metadataPath(r, rootPage), entry); err != nil {
		t.Fatal(err)
	}
	if _, err := (Scanner{}).Scan(context.Background(), r); err == nil {
		t.Fatal("symlink metadata accepted")
	}
}

func TestDeletedSubtreeAndStableWarnings(t *testing.T) {
	r := fixture(t)
	for _, id := range []string{rootPage, docPage, tablePage} {
		editPage(t, r, id, func(p map[string]any) {
			p["deletedAt"] = time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC).Format(time.RFC3339)
		})
	}
	s, err := (Scanner{}).Scan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Children("", false), []string{otherRoot}) {
		t.Fatal("deleted node visible")
	}
	if len(s.Children("", true)) != 2 {
		t.Fatal("deleted source lost")
	}
}
