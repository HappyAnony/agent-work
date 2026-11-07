//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func outputFixture(t *testing.T) (*archiveExportContext, *archiveExportOutput) {
	t.Helper()
	e, _, _ := exportContextFixture(t)
	o, err := e.newArchiveOutput("Original 文档")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { o.finish(false) })
	return e, o
}
func readOutputZip(t *testing.T, name string) map[string][]byte {
	t.Helper()
	z, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = body
	}
	return out
}
func TestArchiveExportOutputLifecycleOriginalZIPContract(t *testing.T) {
	e, o := outputFixture(t)
	o.payload.WriteFile("document.docj", []byte("native bytes"), 0750)
	o.payload.Mkdir("empty", 0755)
	stamp := time.Unix(1700000000, 0)
	o.payload.Chtimes("document.docj", stamp, stamp)
	if err := o.pack(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.temp, "export", o.workname)); !os.IsNotExist(err) {
		t.Fatal("private staging inside public export root", err)
	}
	file := filepath.Join(e.temp, "export", o.filename)
	entries := readOutputZip(t, file)
	if string(entries["Original 文档/document.docj"]) != "native bytes" || entries["Original 文档/empty/"] == nil {
		t.Fatal(entries)
	}
	z, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, "document.docj") && (f.Mode().Perm() != 0750 || f.Modified.Unix() != stamp.Unix() || f.Method != zip.Deflate) {
			t.Fatal("ZIP metadata/Deflate changed", f.FileHeader)
		}
	}
	z.Close()
	if err = o.finish(true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(e.temp, o.workname)); !os.IsNotExist(err) {
		t.Fatal("scratch leaked", err)
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal("finished ZIP lost", err)
	}
}
func TestArchiveExportOutputNamesAndLegacyResidueIsolation(t *testing.T) {
	e, _, _ := exportContextFixture(t)
	archiveWrite(t, e.temp, "export/Same/stale.docj", []byte("old"))
	old := archiveWrite(t, e.temp, "export/Same.sy.zip", []byte("old ZIP"))
	for _, name := range []string{"Same", "Same", "../outside", "/absolute", "", ".", "..", "文档 space..", "a\\b"} {
		o, err := e.newArchiveOutput(name)
		if err != nil {
			t.Fatal(name, err)
		}
		if strings.ContainsAny(o.prefix, "/\\") || o.prefix == "." || o.prefix == ".." {
			t.Fatal("unsafe prefix", o.prefix)
		}
		o.payload.WriteFile("fresh.docj", []byte("fresh"), 0600)
		if err = o.pack(); err != nil {
			t.Fatal(err)
		}
		entries := readOutputZip(t, filepath.Join(e.temp, "export", o.filename))
		if len(entries) != 1 || string(entries[o.prefix+"/fresh.docj"]) != "fresh" {
			t.Fatal("residue mixed", entries)
		}
		if err = o.finish(true); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "old ZIP" {
		t.Fatal("old ZIP overwritten")
	}
}
func TestArchiveExportOutputConcurrentSameTitleIndependent(t *testing.T) {
	e, _, _ := exportContextFixture(t)
	outputs := []*archiveExportOutput{}
	for i := 0; i < 3; i++ {
		captured, err := captureArchiveExport(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		o, err := captured.newArchiveOutput("Same")
		if err != nil {
			t.Fatal(err)
		}
		o.payload.WriteFile("fresh", []byte{byte(i)}, 0600)
		outputs = append(outputs, o)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for _, o := range outputs {
		wg.Add(1)
		go func(o *archiveExportOutput) {
			defer wg.Done()
			err := o.pack()
			errs <- errors.Join(err, o.finish(err == nil))
		}(o)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]bool{}
	for i, o := range outputs {
		if names[o.filename] {
			t.Fatal("colliding filename")
		}
		names[o.filename] = true
		entries := readOutputZip(t, filepath.Join(e.temp, "export", o.filename))
		if !bytes.Equal(entries["Same/fresh"], []byte{byte(i)}) {
			t.Fatal("concurrent data mixed")
		}
	}
}
func TestArchiveExportOutputSourceLinksAndPublishedCollisionReject(t *testing.T) {
	for _, kind := range []string{"source-link", "collision"} {
		t.Run(kind, func(t *testing.T) {
			e, o := outputFixture(t)
			outside := filepath.Join(t.TempDir(), "outside")
			os.WriteFile(outside, []byte("outside"), 0600)
			if kind == "source-link" {
				o.payload.Symlink(outside, "linked")
			} else {
				o.exports.WriteFile(o.filename, []byte("prior"), 0600)
				o.payload.WriteFile("new", []byte("new"), 0600)
			}
			if err := o.pack(); err == nil {
				t.Fatal("unsafe pack succeeded")
			}
			err := o.finish(false)
			if kind == "source-link" && err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(outside)
			if string(raw) != "outside" {
				t.Fatal("outside modified")
			}
			if kind == "collision" {
				got, _ := os.ReadFile(filepath.Join(e.temp, "export", o.filename))
				if string(got) != "prior" {
					t.Fatal("prior ZIP overwritten/deleted")
				}
			}
		})
	}
}
func TestArchiveExportOutputRootReplacementAndCancellation(t *testing.T) {
	for _, kind := range []string{"export-link", "work-replaced", "payload-replaced", "cancel-after-pack", "temp-changed"} {
		t.Run(kind, func(t *testing.T) {
			e, o := outputFixture(t)
			o.payload.WriteFile("file", []byte("payload"), 0600)
			if kind == "cancel-after-pack" || kind == "temp-changed" {
				if err := o.pack(); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "export-link":
				old := filepath.Join(e.temp, "export-old")
				os.Rename(filepath.Join(e.temp, "export"), old)
				outside := t.TempDir()
				os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600)
				os.Symlink(outside, filepath.Join(e.temp, "export"))
			case "work-replaced":
				o.temp.Rename(o.workname, o.workname+"-old")
				o.temp.Mkdir(o.workname, 0700)
				o.temp.WriteFile(o.workname+"/keep", []byte("foreign"), 0600)
			case "payload-replaced":
				o.work.Rename("content", "content-old")
				o.work.Mkdir("content", 0700)
			case "cancel-after-pack":
				ctx, cancel := context.WithCancel(context.Background())
				e.binding.scope.ctx = ctx
				cancel()
			case "temp-changed":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			}
			if err := e.guard(); err == nil {
				t.Fatal("owner change accepted", kind)
			}
			err := o.finish(kind == "cancel-after-pack" || kind == "temp-changed")
			if (kind == "work-replaced" || kind == "cancel-after-pack" || kind == "temp-changed") && err == nil {
				t.Fatal("cleanup/owner error hidden", kind)
			}
			if kind == "work-replaced" {
				got, _ := os.ReadFile(filepath.Join(e.temp, o.workname, "keep"))
				if string(got) != "foreign" {
					t.Fatal("replacement deleted")
				}
			}
			if kind == "cancel-after-pack" || kind == "temp-changed" {
				if _, err = os.Stat(filepath.Join(e.temp, "export", o.filename)); !os.IsNotExist(err) {
					t.Fatal("failed export published", err)
				}
			}
		})
	}
}
func TestArchiveExportOutputCleanupFailureSuppressesReadyArtifact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	e, o := outputFixture(t)
	o.payload.WriteFile("file", []byte("body"), 0600)
	if err := o.pack(); err != nil {
		t.Fatal(err)
	}
	os.Chmod(o.folder, 0000)
	defer os.Chmod(o.folder, 0700)
	if err := o.finish(true); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	if _, err := os.Stat(filepath.Join(e.temp, "export", o.filename)); !os.IsNotExist(err) {
		t.Fatal("ready artifact retained after cleanup error", err)
	}
}

type brokenArchiveWriter struct{ err error }

func (w brokenArchiveWriter) Write([]byte) (int, error) { return 0, w.err }
func TestArchiveExportOutputZIPCloseAndCopyFailuresPropagate(t *testing.T) {
	_, o := outputFixture(t)
	o.payload.WriteFile("file", []byte("body"), 0600)
	fault := errors.New("zip output failure")
	source := indexrepair.Source{Data: o.payload}
	if err := writeOwnedArchive(source, "Root", brokenArchiveWriter{fault}, func() error { return nil }, func(w io.Writer, r io.Reader) error { _, err := io.Copy(w, r); return err }); !errors.Is(err, fault) {
		t.Fatal("ZIP close/flush error ignored", err)
	}
	var buffer bytes.Buffer
	if err := writeOwnedArchive(source, "Root", &buffer, func() error { return nil }, func(io.Writer, io.Reader) error { return fault }); !errors.Is(err, fault) {
		t.Fatal("copy error ignored", err)
	}
}

func TestArchiveExportOutputCheckedManifestBeforeMembers(t *testing.T) {
	primary := errors.New("archive output enumeration incomplete")
	for _, kind := range []string{"walk-incomplete", "scan-replaced"} {
		t.Run(kind, func(t *testing.T) {
			_, o := outputFixture(t)
			if err := o.payload.WriteFile("early.txt", []byte("early"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := o.payload.Mkdir("z", 0700); err != nil {
				t.Fatal(err)
			}
			if err := o.payload.WriteFile("z/late.txt", []byte("late"), 0600); err != nil {
				t.Fatal(err)
			}
			op := archiveOutputWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name == "z" {
					if kind == "walk-incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					if err := source.Data.Rename("early.txt", "early.txt.old"); err != nil {
						return nil, err
					}
					if err := source.Data.WriteFile("early.txt", []byte("replacement"), 0600); err != nil {
						return nil, err
					}
				}
				return source.ReadDir(ctx, name)
			}}
			var output bytes.Buffer
			err := writeOwnedArchiveWithIO(context.Background(), indexrepair.Source{Data: o.payload}, "Root", &output, func() error { return nil }, func(dst io.Writer, src io.Reader) error {
				_, err := io.Copy(dst, src)
				return err
			}, op)
			if err == nil || output.Len() != 0 {
				t.Fatal("failed manifest created ZIP bytes", output.Len(), err)
			}
			if kind == "walk-incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete archive output walk error lost or admitted as missing", err)
				}
				if raw, readErr := o.payload.ReadFile("early.txt"); readErr != nil || string(raw) != "early" {
					t.Fatal("failed archive output walk changed source", readErr)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("scan-time archive output replacement accepted", err)
			}
		})
	}
}

func TestArchiveExportOutputRejectsLinkedParentAndClosedContext(t *testing.T) {
	for _, kind := range []string{"linked-export", "linked-temp", "closed-owner"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := exportContextFixture(t)
			if kind == "closed-owner" {
				Conf.CloseCloudRepo(context.Background())
			} else {
				os.MkdirAll(e.temp, 0700)
				if kind == "linked-export" {
					os.Symlink(t.TempDir(), filepath.Join(e.temp, "export"))
				} else {
					os.RemoveAll(e.temp)
					os.Symlink(t.TempDir(), e.temp)
				}
			}
			if o, err := e.newArchiveOutput("Title"); err == nil {
				o.finish(false)
				t.Fatal("invalid output owner accepted", kind)
			}
		})
	}
}

func TestArchiveExportOutputPrivateStagingNotServed(t *testing.T) {
	e, o := outputFixture(t)
	if err := o.work.WriteFile("archive.part", []byte("unfinished ZIP"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(e.temp, "export"))))
	defer server.Close()
	response, err := http.Get(server.URL + "/" + o.workname + "/archive.part")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatal("private staging reachable through export route", response.StatusCode)
	}
}
