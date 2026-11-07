//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/util"
)

type cloudInputFault struct {
	*os.File
	read    func([]byte) (int, error)
	stat    func() (fs.FileInfo, error)
	onClose func() error
	closes  int
}

func (f *cloudInputFault) Read(b []byte) (int, error) {
	if f.read != nil {
		return f.read(b)
	}
	return f.File.Read(b)
}
func (f *cloudInputFault) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *cloudInputFault) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return err
}

func lazyCloudFixture(t *testing.T) (*cloudUploadFile, string) {
	t.Helper()
	dir := t.TempDir()
	name := filepath.Join(dir, "asset.bin")
	os.WriteFile(name, []byte("synthetic bytes"), 0600)
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	info, err := r.Lstat("asset.bin")
	if err != nil {
		t.Fatal(err)
	}
	return &cloudUploadFile{ctx: context.Background(), root: r, name: "asset.bin", identity: info, guard: func() error { return nil }, open: cloudUploadIO{}.defaults().openFile}, name
}
func cloudUploadFixture(t *testing.T, denied ...bool) (*AppConf, string, *int) {
	t.Helper()
	c, _, dir := cloudBusinessFixture(t)
	uploads := new(int)
	s, err := c.newCloudAccount(func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error) {
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"uploadToken": "synthetic"}}}, nil
	}, func(context.Context, int, string, cloudaccount.UploadFile, string, string) (cloudaccount.Response, error) {
		*uploads++
		if len(denied) > 0 && denied[0] {
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(9), "msg": "synthetic refusal"}}, nil
		}
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0)}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c.cloudAccount = s
	return c, dir, uploads
}

func TestCloudUploadReaderLazyCloseOnceAndReadAfterClose(t *testing.T) {
	f, _ := lazyCloudFixture(t)
	opens := 0
	f.open = func(r *os.Root, n string) (cloudUploadInput, error) { opens++; return r.Open(n) }
	if err := f.Close(); err != nil || opens != 0 {
		t.Fatal(err, opens)
	}
	if _, err := f.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) || opens != 0 {
		t.Fatal(err, opens)
	}
	f, _ = lazyCloudFixture(t)
	var input *cloudInputFault
	closing := errors.New("file close")
	f.open = func(r *os.Root, n string) (cloudUploadInput, error) {
		v, err := r.Open(n)
		if err != nil {
			return nil, err
		}
		input = &cloudInputFault{File: v, onClose: func() error { return closing }}
		return input, nil
	}
	raw, err := io.ReadAll(f)
	if err != nil || string(raw) != "synthetic bytes" {
		t.Fatal(string(raw), err)
	}
	for i := 0; i < 2; i++ {
		if err = f.Close(); !errors.Is(err, closing) {
			t.Fatal(err)
		}
	}
	if input.closes != 1 {
		t.Fatal(input.closes)
	}
	if _, err = input.File.Stat(); err == nil {
		t.Fatal("file still open")
	}
}

func TestCloudUploadReaderIdentityReadCloseAndCancellation(t *testing.T) {
	for _, kind := range []string{"replace-before", "replace-read", "mutate-close", "cancel-read", "cancel-close", "owner-close", "read-close", "stat", "wrong-descriptor", "no-progress", "open"} {
		t.Run(kind, func(t *testing.T) {
			f, name := lazyCloudFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.ctx = ctx
			primary, closing := errors.New("input read"), errors.New("input close")
			owner := true
			f.guard = func() error {
				if !owner {
					return primary
				}
				return nil
			}
			if kind == "replace-before" {
				os.Rename(name, name+".old")
				os.WriteFile(name, []byte("synthetic bytes"), 0600)
			}
			var input *cloudInputFault
			f.open = func(r *os.Root, n string) (cloudUploadInput, error) {
				if kind == "open" {
					return nil, primary
				}
				if kind == "wrong-descriptor" {
					r.WriteFile("other", []byte("synthetic bytes"), 0600)
					n = "other"
				}
				v, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				input = &cloudInputFault{File: v}
				if kind == "stat" {
					input.stat = func() (fs.FileInfo, error) { return nil, primary }
				}
				input.read = func(b []byte) (int, error) {
					if kind == "read-close" {
						return 0, primary
					}
					if kind == "no-progress" {
						return 0, nil
					}
					n, err := v.Read(b)
					if kind == "cancel-read" {
						cancel()
					}
					if kind == "replace-read" {
						os.Rename(name, name+".old")
						os.WriteFile(name, []byte("replacement"), 0600)
					}
					return n, err
				}
				input.onClose = func() error {
					switch kind {
					case "read-close":
						return closing
					case "mutate-close":
						return os.WriteFile(name, []byte("late"), 0600)
					case "cancel-close":
						cancel()
						return closing
					case "owner-close":
						owner = false
					}
					return nil
				}
				return input, nil
			}
			raw, readErr := io.ReadAll(f)
			closeErr := f.Close()
			err := errors.Join(readErr, closeErr)
			if err == nil {
				t.Fatal(kind, string(raw))
			}
			if kind == "read-close" && (!errors.Is(err, primary) || !errors.Is(err, closing)) {
				t.Fatal(err)
			}
			if kind == "cancel-close" && (!errors.Is(err, context.Canceled) || !errors.Is(err, closing)) {
				t.Fatal(err)
			}
			if input != nil && input.closes != 1 {
				t.Fatal(input.closes)
			}
		})
	}
}

func TestCloudUploadCloseErrorsAndPartialRemoteCount(t *testing.T) {
	for _, kind := range []string{"root", "file-root", "read-file-root", "partial"} {
		t.Run(kind, func(t *testing.T) {
			c, dir, uploads := cloudUploadFixture(t)
			a, b, readErr := errors.New("root close"), errors.New("file close"), errors.New("file read")
			paths := []string{"assets/fixture.png"}
			if kind == "partial" {
				os.WriteFile(filepath.Join(dir, "assets/second.png"), []byte("second"), 0600)
				paths = append(paths, "assets/second.png")
			}
			rootCloses, opens := 0, 0
			op := cloudUploadIO{closeRoot: func(r *os.Root) error { rootCloses++; return errors.Join(r.Close(), a) }, openFile: func(r *os.Root, n string) (cloudUploadInput, error) {
				opens++
				v, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				f := &cloudInputFault{File: v}
				if kind != "root" && (kind != "partial" || opens == 2) {
					f.onClose = func() error { return b }
				}
				if kind == "read-file-root" {
					f.read = func([]byte) (int, error) { return 0, readErr }
				}
				return f, nil
			}}
			n, err := c.uploadCloudAssetsWithIO(context.Background(), paths, cloudaccount.ExportForumBusiness, op)
			if !errors.Is(err, a) || rootCloses != 1 {
				t.Fatal(n, err, rootCloses)
			}
			if kind != "root" && !errors.Is(err, b) {
				t.Fatal(err)
			}
			if kind == "read-file-root" && !errors.Is(err, readErr) {
				t.Fatal(err)
			}
			want := 0
			if kind == "root" || kind == "partial" {
				want = 1
			}
			if n != want || *uploads != want {
				t.Fatal(n, *uploads, want, err)
			}
		})
	}
}

func TestCloudUploadOversizeNeverOpensPayload(t *testing.T) {
	c, dir, uploads := cloudUploadFixture(t)
	name := filepath.Join(dir, "assets/oversize.bin")
	v, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Truncate(11 << 20); err != nil {
		t.Fatal(err)
	}
	v.Close()
	opens, closes := 0, 0
	n, err := c.uploadCloudAssetsWithIO(context.Background(), []string{"assets/oversize.bin"}, cloudaccount.ExportForumBusiness, cloudUploadIO{openFile: func(*os.Root, string) (cloudUploadInput, error) { opens++; return nil, errors.New("must not open") }, closeRoot: func(r *os.Root) error { closes++; return r.Close() }})
	if n != 0 || err != nil || opens != 0 || closes != 1 || *uploads != 0 {
		t.Fatal(n, err, opens, closes, *uploads)
	}
}

func TestCloudUploadRootCloseOwnerAndCancellationChecks(t *testing.T) {
	for _, kind := range []string{"pre-cancel", "cancel-file", "cancel-root", "conf", "data", "workspace", "closed", "cloud-closed", "root"} {
		t.Run(kind, func(t *testing.T) {
			c, dir, uploads := cloudUploadFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			old, data, workspace := Conf, util.DataDir, util.WorkspaceDir
			defer func() {
				Conf = old
				util.DataDir = data
				util.WorkspaceDir = workspace
				c.repoMu.Lock()
				c.repoClosed = false
				c.repoMu.Unlock()
				c.cloudMu.Lock()
				c.cloudClosed = false
				c.cloudMu.Unlock()
			}()
			if kind == "pre-cancel" {
				cancel()
			}
			opens, closes := 0, 0
			op := cloudUploadIO{openRoot: func(n string) (*os.Root, error) { opens++; return os.OpenRoot(n) }, closeRoot: func(r *os.Root) error {
				closes++
				err := r.Close()
				switch kind {
				case "cancel-root":
					cancel()
				case "conf":
					Conf = &AppConf{}
				case "data":
					util.DataDir = t.TempDir()
				case "workspace":
					util.WorkspaceDir = t.TempDir()
				case "cloud-closed":
					c.cloudMu.Lock()
					c.cloudClosed = true
					c.cloudMu.Unlock()
				case "closed":
					c.repoMu.Lock()
					c.repoClosed = true
					c.repoMu.Unlock()
				case "root":
					os.Rename(dir, dir+".old")
					t.Cleanup(func() { os.RemoveAll(dir + ".old") })
					os.Mkdir(dir, 0700)
				}
				return err
			}, openFile: func(r *os.Root, n string) (cloudUploadInput, error) {
				v, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				return &cloudInputFault{File: v, onClose: func() error {
					if kind == "cancel-file" {
						cancel()
					}
					return nil
				}}, nil
			}}
			n, err := c.uploadCloudAssetsWithIO(ctx, []string{"assets/fixture.png"}, cloudaccount.ExportForumBusiness, op)
			if err == nil || opens != closes {
				t.Fatal(n, err, opens, closes)
			}
			want := 1
			if kind == "pre-cancel" || kind == "cancel-file" {
				want = 0
			}
			if n != want || *uploads != want {
				t.Fatal(n, *uploads, want, err)
			}
			if kind == "pre-cancel" && opens != 0 {
				t.Fatal(opens)
			}
			if (kind == "pre-cancel" || kind == "cancel-file" || kind == "cancel-root") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestCloudUploadLocalizedRemoteErrorRetainsCauseAndRootClose(t *testing.T) {
	c, _, uploads := cloudUploadFixture(t, true)
	closing := errors.New("root close")
	n, err := c.uploadCloudAssetsWithIO(context.Background(), []string{"assets/fixture.png"}, cloudaccount.ExportForumBusiness, cloudUploadIO{closeRoot: func(r *os.Root) error { return errors.Join(r.Close(), closing) }})
	var remote *cloudaccount.BusinessError
	if n != 0 || *uploads != 1 || !errors.Is(err, closing) || !errors.As(err, &remote) || !strings.Contains(err.Error(), "upload failed: synthetic refusal") {
		t.Fatal(n, *uploads, err)
	}
}
