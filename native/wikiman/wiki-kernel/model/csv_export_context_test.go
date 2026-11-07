//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
)

func csvRecords(t *testing.T, raw []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(raw, []byte{239, 187, 191}) {
		t.Fatal("missing UTF-8 BOM")
	}
	rows, err := csv.NewReader(bytes.NewReader(raw[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func csvDownload(t *testing.T, m *markdownExportContext, link string) map[string][]byte {
	t.Helper()
	if !strings.HasPrefix(link, "/export/csv/") || !strings.HasSuffix(link, ".db.zip") {
		t.Fatal(link)
	}
	relative, err := url.PathUnescape(strings.TrimPrefix(link, "/"))
	if err != nil {
		t.Fatal(err)
	}
	return readOutputZip(t, filepath.Join(m.read.temp, filepath.FromSlash(relative)))
}
func TestCSVExportOriginalLayoutsFilterSortAndIndependentPackages(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, a, tree, file := markdownAVFixture(t, layout)
			block := tree.Root.FirstChild.ID
			before, _ := os.ReadFile(file)
			docFile := filepath.Join(m.read.binding.scope.commit.r.data, tree.Box, tree.ID+".docj")
			docBefore, _ := os.ReadFile(docFile)
			prior := archiveWrite(t, m.read.temp, "export/csv/Export AV.db.zip", []byte("prior"))
			stale := archiveWrite(t, m.read.temp, "export/csv/Export AV/stale.csv", []byte("stale"))
			link, err := m.exportCSV(a.ID, block)
			if err != nil {
				t.Fatal(err)
			}
			contents := csvDownload(t, m, link)
			if len(contents) != 1 {
				t.Fatal(contents)
			}
			rows := csvRecords(t, contents["Export AV.csv"])
			if len(rows) != 3 || len(rows[0]) != 4 || rows[1][0] != "gamma" || rows[2][0] != "alpha\nrow" || rows[1][2] != "3" || rows[2][2] != "2" || rows[1][3] != "secret | text" {
				t.Fatal(rows)
			}
			second, err := ExportAv2CSVContext(context.Background(), a.ID, block)
			if err != nil || second == link {
				t.Fatal(second, err)
			}
			if got := csvDownload(t, m, second); !reflect.DeepEqual(contents, got) {
				t.Fatal("repeated candidates changed output")
			}
			for path, want := range map[string][]byte{file: before, docFile: docBefore, prior: []byte("prior"), stale: []byte("stale")} {
				raw, _ := os.ReadFile(path)
				if !bytes.Equal(raw, want) {
					t.Fatal("changed pre-existing file", path)
				}
			}
			entries, _ := os.ReadDir(m.read.temp)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
					t.Fatal("scratch remains")
				}
			}
		})
	}
}
func TestCSVExportBoundViewAndCapturedUntitledName(t *testing.T) {
	m, a, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
	second := *a.Views[0]
	second.ID = ast.NewNodeID()
	second.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 2, IsNotEmpty: true}}}}
	a.Views = append(a.Views, &second)
	a.Name = "   "
	raw, _ := json.Marshal(a)
	os.WriteFile(file, raw, 0600)
	m.read.binding.scope.commit.avUntitled = "未命名 数据库"
	nodeID := ast.NewNodeID()
	markdownDoc(t, m, tree.Box, "/"+ast.NewNodeID()+".docj", map[string]interface{}{"ID": nodeID, "Type": "NodeAttributeView", "AttributeViewID": a.ID, "Properties": map[string]string{av.NodeAttrView: second.ID}})
	link, err := m.exportCSV(a.ID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	members := csvDownload(t, m, link)
	rows := csvRecords(t, members["未命名 数据库.csv"])
	if len(rows) != 2 || rows[1][2] != "2" {
		t.Fatal(rows)
	}
}
func TestCSVExportSelectionSourceAndPublicationFailures(t *testing.T) {
	for _, kind := range []string{"invalid-id", "missing-av", "corrupt-av", "linked-av", "block-mismatch", "canceled", "closed", "linked-output"} {
		t.Run(kind, func(t *testing.T) {
			m, a, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
			block := tree.Root.FirstChild.ID
			outside := t.TempDir()
			sentinel := archiveWrite(t, outside, "sentinel", []byte("keep"))
			switch kind {
			case "invalid-id":
				a.ID = "../wrong"
			case "missing-av":
				os.Remove(file)
			case "corrupt-av":
				os.WriteFile(file, []byte("bad"), 0600)
			case "linked-av":
				raw, _ := os.ReadFile(file)
				target := archiveWrite(t, outside, "source.json", raw)
				os.Remove(file)
				if err := os.Symlink(target, file); err != nil {
					t.Fatal(err)
				}
			case "block-mismatch":
				block = tree.ID
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "closed":
				owner := m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			case "linked-output":
				os.MkdirAll(filepath.Join(m.read.temp, "export"), 0700)
				if err := os.Symlink(outside, filepath.Join(m.read.temp, "export/csv")); err != nil {
					t.Fatal(err)
				}
			}
			if link, err := m.exportCSV(a.ID, block); err == nil || link != "" {
				t.Fatal(link, err)
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "keep" {
				t.Fatal("foreign output touched")
			}
			entries, _ := os.ReadDir(m.read.temp)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
					t.Fatal("failed scratch remains")
				}
			}
		})
	}
	if link, err := ExportAv2CSVContext(nil, ast.NewNodeID(), ast.NewNodeID()); err == nil || link != "" {
		t.Fatal(link, err)
	}
}
func csvCell(v *av.Value) *av.TableCell { return &av.TableCell{BaseValue: &av.BaseValue{Value: v}} }
func TestCSVExportOriginalValueFormatsBOMAndQuoting(t *testing.T) {
	stamp := time.Date(2026, 10, 30, 12, 30, 0, 0, time.Local).UnixMilli()
	a := &av.AttributeView{KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "created", Type: av.KeyTypeCreated, Created: &av.Created{IncludeTime: false}}}, {Key: &av.Key{ID: "updated", Type: av.KeyTypeUpdated, Updated: &av.Updated{IncludeTime: false}}}}}
	table := &av.Table{}
	for _, name := range []string{"Date", "Created", "Updated", "Assets", "Line", "Text,\"quoted\"", "Empty"} {
		table.Columns = append(table.Columns, &av.TableColumn{BaseInstanceField: &av.BaseInstanceField{Name: name}})
	}
	for i := 0; i < 2; i++ {
		table.Rows = append(table.Rows, &av.TableRow{Cells: []*av.TableCell{
			csvCell(&av.Value{Type: av.KeyTypeDate, Date: &av.ValueDate{Content: stamp, IsNotTime: true}}),
			csvCell(&av.Value{KeyID: "created", Type: av.KeyTypeCreated, Created: &av.ValueCreated{Content: stamp}}),
			csvCell(&av.Value{KeyID: "updated", Type: av.KeyTypeUpdated, Updated: &av.ValueUpdated{Content: stamp}}),
			csvCell(&av.Value{Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Type: av.AssetTypeImage, Name: "image", Content: "assets/a.png"}, {Type: av.AssetTypeFile, Name: "file", Content: "assets/b.pdf"}, {Type: av.AssetType("other"), Content: "external"}}}),
			csvCell(&av.Value{Type: av.KeyTypeLineNumber}), csvCell(&av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "a,\"b\"\n第二行"}}), csvCell(nil),
		}})
	}
	var buf bytes.Buffer
	if err := writeAttributeViewCSV(&buf, a, table, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	rows := csvRecords(t, buf.Bytes())
	if len(rows) != 3 || rows[0][5] != "Text,\"quoted\"" || rows[1][0] != "2026-10-30" || rows[1][1] != "2026-10-30" || rows[1][2] != "2026-10-30" || rows[1][3] != "![image](assets/a.png) [file](assets/b.pdf) external" || rows[1][4] != "1" || rows[2][4] != "2" || rows[1][5] != "a,\"b\"\n第二行" || rows[1][6] != "" {
		t.Fatal(rows)
	}
}

type csvFailureWriter struct {
	calls int
	short bool
	err   error
}

func (w *csvFailureWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short {
		return 0, nil
	}
	if w.calls > 1 {
		return 0, w.err
	}
	return len(p), nil
}
func TestCSVExportWriterFlushAndCancellationFailures(t *testing.T) {
	failure := errors.New("fixture flush failure")
	table := &av.Table{Columns: []*av.TableColumn{{BaseInstanceField: &av.BaseInstanceField{Name: "header"}}}}
	if err := writeAttributeViewCSV(&csvFailureWriter{err: failure}, &av.AttributeView{}, table, func() error { return nil }); !errors.Is(err, failure) {
		t.Fatal("buffer flush failure lost", err)
	}
	if err := writeAttributeViewCSV(&csvFailureWriter{short: true}, &av.AttributeView{}, table, func() error { return nil }); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	table.Rows = []*av.TableRow{{Cells: []*av.TableCell{csvCell(nil)}}}
	calls := 0
	err := writeAttributeViewCSV(io.Discard, &av.AttributeView{}, table, func() error {
		calls++
		if calls > 1 {
			return context.Canceled
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestCSVExportPublicationSubdirectoryOwnershipAndCollision(t *testing.T) {
	for _, kind := range []string{"collision", "replaced", "foreign-published"} {
		t.Run(kind, func(t *testing.T) {
			e, o := outputFixture(t)
			o.prefix = ""
			if err := o.useSubdirectory("csv"); err != nil {
				t.Fatal(err)
			}
			o.payload.WriteFile("database.csv", []byte("csv"), 0600)
			if kind == "collision" {
				prior := archiveWrite(t, e.temp, "export/"+o.filename, []byte("prior"))
				if err := o.pack(); err == nil {
					t.Fatal("collision overwritten")
				}
				o.finish(false)
				raw, _ := os.ReadFile(prior)
				if string(raw) != "prior" {
					t.Fatal("prior changed")
				}
				return
			}
			if kind == "replaced" {
				csvDir := filepath.Join(e.temp, "export/csv")
				os.Rename(csvDir, csvDir+"-old")
				os.Mkdir(csvDir, 0700)
				if err := o.pack(); err == nil {
					t.Fatal("replacement accepted")
				}
				return
			}
			if err := o.pack(); err != nil {
				t.Fatal(err)
			}
			if err := o.useSubdirectory("other"); err == nil {
				t.Fatal("published output retargeted")
			}
			target := filepath.Join(e.temp, "export", filepath.FromSlash(o.filename))
			os.Remove(target)
			os.WriteFile(target, []byte("foreign"), 0600)
			if err := o.finish(false); err == nil {
				t.Fatal("foreign replacement removed")
			}
			raw, _ := os.ReadFile(target)
			if string(raw) != "foreign" {
				t.Fatal("foreign output changed")
			}
		})
	}
}
