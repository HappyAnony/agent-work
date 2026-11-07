//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func exportPathsFixture(t *testing.T) (*archiveExportContext, string, string) {
	t.Helper()
	e, box, _ := exportContextFixture(t)
	base := e.binding.scope.commit.r.data
	for _, n := range []string{"a.docj", "folder/x.docj", "folder.docj", "assets/ignored.docj", ".hidden/ignored.docj", "skip.tmp/ignored.docj", "old.tmp", "keep\\name.tmp", "other.txt"} {
		archiveWrite(t, base, box+"/scan/"+n, []byte("unchanged fixture"))
	}
	return e, box, filepath.Join(base, box, "scan")
}

func TestArchiveExportPathsOriginalOrderFilteringAndNoSourceDeletion(t *testing.T) {
	e, box, dir := exportPathsFixture(t)
	out, err := e.documentPaths(box, "/scan", false)
	want := []string{"/scan/a.docj", "/scan/folder/x.docj", "/scan/folder.docj"}
	if err != nil || !reflect.DeepEqual(out, want) {
		t.Fatal(out, err)
	}
	for _, n := range []string{"old.tmp", "keep\\name.tmp", "assets/ignored.docj", "skip.tmp/ignored.docj"} {
		raw, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil || string(raw) != "unchanged fixture" {
			t.Fatal(n, err)
		}
	}
}

func TestArchiveExportPathsNoPartialListOnReadAndRootCloseErrors(t *testing.T) {
	for _, kind := range []string{"read", "read-root", "optional-root"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportPathsFixture(t)
			primary, closing := errors.New("directory close/read"), errors.New("outer close")
			r := e.binding.scope.commit.r
			closes := 0
			if kind != "read" {
				r.closeRoot = func(root *os.Root) error { closes++; return errors.Join(root.Close(), closing) }
			}
			reads := 0
			op := archivePathIO{readDir: func(ctx context.Context, s indexrepair.Source, n string) ([]fs.DirEntry, error) {
				reads++
				if reads == 2 {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				return s.ReadDir(ctx, n)
			}}
			dir := "/scan"
			optional := false
			if kind == "optional-root" {
				dir = "/absent/child"
				optional = true
			}
			out, err := e.documentPathsWithIO(box, dir, optional, op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
			if kind != "optional-root" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if kind != "read" && (!errors.Is(err, closing) || closes != 1) {
				t.Fatal(err, closes)
			}
		})
	}
}

func TestArchiveExportPathsPostCloseMutationAndOwnerChecks(t *testing.T) {
	for _, kind := range []string{"file", "directory", "root", "owner", "cancel", "absent-appears", "parent-link"} {
		t.Run(kind, func(t *testing.T) {
			e, box, dir := exportPathsFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var err error
			e, err = captureArchiveExport(ctx)
			if err != nil {
				t.Fatal(err)
			}
			r := e.binding.scope.commit.r
			data := util.DataDir
			defer func() { util.DataDir = data }()
			closes := 0
			r.closeRoot = func(root *os.Root) error {
				closes++
				err := root.Close()
				switch kind {
				case "file":
					os.WriteFile(filepath.Join(dir, "a.docj"), []byte("changed"), 0600)
				case "directory":
					os.Rename(dir, dir+".old")
					os.Mkdir(dir, 0700)
				case "root":
					os.Rename(data, data+".old")
					t.Cleanup(func() { os.RemoveAll(data + ".old") })
					os.Mkdir(data, 0700)
				case "owner":
					util.DataDir = t.TempDir()
				case "cancel":
					cancel()
				case "absent-appears":
					os.Mkdir(filepath.Join(data, box, "absent"), 0700)
				case "parent-link":
					p := filepath.Join(data, box)
					os.Rename(p, p+".old")
					os.Symlink(p+".old", p)
				}
				return err
			}
			destination := "/scan"
			optional := false
			if kind == "absent-appears" {
				destination = "/absent/child"
				optional = true
			}
			out, err := e.documentPaths(box, destination, optional)
			if out != nil || err == nil || closes != 1 {
				t.Fatal(out, err, closes)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestArchiveExportPathsInitialMissingAndIncludedLinks(t *testing.T) {
	e, box, dir := exportPathsFixture(t)
	if out, err := e.documentPaths(box, "/absent/child", true); err != nil || out != nil {
		t.Fatal(out, err)
	}
	if out, err := e.documentPaths(box, "/absent/child", false); err == nil || out != nil {
		t.Fatal(out, err)
	}
	if err := os.Symlink("a.docj", filepath.Join(dir, "link.docj")); err != nil {
		t.Fatal(err)
	}
	if out, err := e.documentPaths(box, "/scan", false); err == nil || out != nil {
		t.Fatal(out, err)
	}
}
