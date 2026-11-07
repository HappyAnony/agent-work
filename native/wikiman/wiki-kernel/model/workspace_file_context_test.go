//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/workspacefile"
	"wiki-kernel/util"
)

func TestWorkspaceFileActualSourceAndPrivateBoundary(t *testing.T) {
	old, oldWorkspace, oldData, oldConf := Conf, util.WorkspaceDir, util.DataDir, util.ConfDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.ConfDir = filepath.Join(root, "conf")
	os.MkdirAll(util.DataDir, 0700)
	os.MkdirAll(util.ConfDir, 0700)
	owner := settingOwner(t, filepath.Join(util.ConfDir, "conf.json"), false)
	owner.Sync = conf.NewSync()
	Conf = owner
	t.Cleanup(func() {
		owner.CloseCloudRepo(context.Background())
		Conf = old
		util.WorkspaceDir = oldWorkspace
		util.DataDir = oldData
		util.ConfDir = oldConf
	})
	ctx := context.Background()
	stamp := time.UnixMilli(1234567890000)
	if _, err := WorkspaceFileContext(ctx, workspacefile.Request{Action: "write", Path: "/data/files/a.txt", Data: []byte("first"), ModTime: stamp}, true); err != nil {
		t.Fatal(err)
	}
	got, err := WorkspaceFileContext(ctx, workspacefile.Request{Action: "read", Path: "data/files/a.txt"}, true)
	if err != nil || string(got.Data) != "first" {
		t.Fatal(err, string(got.Data))
	}
	info, _ := os.Stat(filepath.Join(root, "data/files/a.txt"))
	if !info.ModTime().Equal(stamp) {
		t.Fatal(info.ModTime())
	}
	os.WriteFile(owner.confPath, []byte("private"), 0600)
	if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "read", Path: "conf/conf.json"}, false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside", "data/../../outside"} {
		if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "write", Path: name, Data: []byte("bad")}, true); err == nil {
			t.Fatal("escape admitted", name)
		}
	}
	owner.persistenceReadOnly = true
	if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "write", Path: "data/files/a.txt", Data: []byte("bad")}, true); !errors.Is(err, ErrSettingReadOnly) {
		t.Fatal(err)
	}
	owner.persistenceReadOnly = false
	request := singleUploadRequest(t, "b.txt", []byte("multipart"), map[string]string{"path": "/data/files/b.txt", "modTime": "1234567890000"})
	if err = PutWorkspaceFileRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	directoryRequest := httptest.NewRequest("POST", "/putFile", strings.NewReader("path=data%2Fform-directory&isDir=true"))
	directoryRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err = PutWorkspaceFileRequest(ctx, directoryRequest); err != nil {
		t.Fatal("directory-only form", err)
	}
	if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "rename", Path: "data/files/b.txt", NewPath: "data/files/c.txt"}, true); err != nil {
		t.Fatal(err)
	}
	listing, err := WorkspaceFileContext(ctx, workspacefile.Request{Action: "list", Path: "data/files"}, true)
	if err != nil || len(listing.Entries) != 2 {
		t.Fatal(err, listing.Entries)
	}
	external := filepath.Join(t.TempDir(), "host.txt")
	os.WriteFile(external, []byte("host"), 0600)
	if err = CopyWorkspaceFilesContext(ctx, []string{external, external + "-missing"}, "data/files"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "data/files/host.txt")); !bytes.Equal(raw, []byte("host")) {
		t.Fatal(string(raw))
	}
	if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "remove", Path: "data/files/c.txt"}, true); err != nil {
		t.Fatal(err)
	}
	os.Symlink(t.TempDir(), filepath.Join(root, "data/link"))
	if _, err = WorkspaceFileContext(ctx, workspacefile.Request{Action: "write", Path: "data/link/out.txt", Data: []byte("bad")}, true); err == nil {
		t.Fatal("symlink admitted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = WorkspaceFileContext(cancelled, workspacefile.Request{Action: "read", Path: "data/files/a.txt"}, true); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
