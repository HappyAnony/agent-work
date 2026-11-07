//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/conf"
)

const manifestAVID = "20261022090000-av00001"

func nativeAVQueryLease(t *testing.T) (string, *Lease) {
	t.Helper()
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	node := data["Children"].([]interface{})[0].(map[string]interface{})
	node["Type"] = "NodeAttributeView"
	node["AttributeViewID"] = manifestAVID
	delete(node, "Children")
	raw, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "av")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	doc := map[string]interface{}{"id": manifestAVID, "name": "Manifest AV", "views": []map[string]string{{"id": "view", "name": "Main", "type": "table"}}}
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestAVID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err = msgpack.Marshal(map[string][]string{manifestAVID: {blockID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocks.msgpack"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	manager := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, _ string) (*conf.Business, error) {
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: boxID, Name: "Manifest notebook"}}, nil
		}, nil, nil), nil
	}})
	lease := getDocLease(t, manager, pageA)
	return root, lease
}

func replaceNativeAVQueryFile(t *testing.T, root, relative string) {
	t.Helper()
	target := filepath.Join(root, "pages", pageA, "data", filepath.FromSlash(relative))
	fileInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = 'x'
	}
	replacement := target + ".replacement"
	if err := os.WriteFile(replacement, raw, fileInfo.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(parent, parentInfo.ModTime(), parentInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeAVQueryRejectsUnstableEnumerationAndFileWindows(t *testing.T) {
	for _, test := range []struct {
		name string
		op   func(*testing.T, string) (nativeAVQueryIO, error)
	}{
		{name: "incomplete confirmation", op: func(_ *testing.T, _ string) (nativeAVQueryIO, error) {
			incomplete := errors.New("incomplete AV query enumeration")
			calls := 0
			return nativeAVQueryIO{readDir: func(_ string, dir *os.File, count int) ([]fs.DirEntry, error) {
				entries, err := dir.ReadDir(count)
				calls++
				if calls == 3 {
					return entries, incomplete
				}
				return entries, err
			}}, incomplete
		}},
		{name: "replacement between visible and opened file", op: func(t *testing.T, root string) (nativeAVQueryIO, error) {
			replaced := false
			return nativeAVQueryIO{openFile: func(w *WorkspaceContext, name string) (*os.File, error) {
				if name == "storage/av/"+manifestAVID+".json" && !replaced {
					replaced = true
					replaceNativeAVQueryFile(t, root, name)
				}
				return openNativeFile(w, name)
			}}, errNativeAVQuerySourceChanged
		}},
		{name: "directory close failure", op: func(_ *testing.T, _ string) (nativeAVQueryIO, error) {
			closeErr := errors.New("AV query directory close failed")
			calls := 0
			return nativeAVQueryIO{closeDir: func(_ string, dir *os.File) error {
				calls++
				err := dir.Close()
				if calls == 1 {
					return errors.Join(err, closeErr)
				}
				return err
			}}, closeErr
		}},
		{name: "candidate file close failure", op: func(_ *testing.T, _ string) (nativeAVQueryIO, error) {
			closeErr := errors.New("AV query file close failed")
			return nativeAVQueryIO{closeFile: func(name string, file *os.File) error {
				err := file.Close()
				if name == "storage/av/"+manifestAVID+".json" {
					return errors.Join(err, closeErr)
				}
				return err
			}}, closeErr
		}},
		{name: "final manifest change", op: func(t *testing.T, root string) (nativeAVQueryIO, error) {
			changed := false
			return nativeAVQueryIO{closeFile: func(name string, file *os.File) error {
				err := file.Close()
				if name == "storage/av/"+manifestAVID+".json" && !changed {
					changed = true
					late := filepath.Join(root, "pages", pageA, "data", "storage", "av", "late.tmp")
					if writeErr := os.WriteFile(late, []byte("late"), 0600); writeErr != nil {
						t.Fatal(writeErr)
					}
				}
				return err
			}}, errNativeAVQuerySourceChanged
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, lease := nativeAVQueryLease(t)
			defer lease.Release()
			op, expected := test.op(t, root)
			got, err := lease.searchNativeAttributeViewsWithIO(context.Background(), "", nil, op)
			if !errors.Is(err, expected) || len(got) != 0 {
				t.Fatal(got, err)
			}
		})
	}
}

func TestNativeAVQueryPreservesUnrelatedEntriesAndMissingRelations(t *testing.T) {
	root, lease := nativeAVQueryLease(t)
	defer lease.Release()
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "av")
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "unrelated-link")); err != nil {
		t.Fatal(err)
	}
	opens := map[string]int{}
	got, err := lease.searchNativeAttributeViewsWithIO(context.Background(), "", nil, nativeAVQueryIO{openFile: func(w *WorkspaceContext, name string) (*os.File, error) {
		opens[name]++
		return openNativeFile(w, name)
	}})
	if err != nil || len(got) != 1 || got[0].AvID != manifestAVID || got[0].AvName != "Manifest AV" || opens["storage/av/"+manifestAVID+".json"] != 1 {
		t.Fatal(got, opens, err)
	}
	if err := os.Remove(filepath.Join(dir, "blocks.msgpack")); err != nil {
		t.Fatal(err)
	}
	got, err = lease.SearchNativeAttributeViews(context.Background(), "", nil)
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
}
