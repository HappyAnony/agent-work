// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacefile"
	"wiki-kernel/util"
)

func WorkspaceFileContext(ctx context.Context, q workspacefile.Request, administrator bool) (out workspacefile.Result, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return out, err
	}
	private := filepath.Join(util.ConfDir, "conf.json")
	q.Data = append([]byte(nil), q.Data...)
	err = base.run(ctx, func(c, _ context.Context) (result error) {
		name, e := workspacefile.Name(q.Path, q.Action == "list")
		if e != nil {
			return e
		}
		if q.Action == "read" && !administrator && filepath.Join(base.workspace, filepath.FromSlash(name)) == private {
			return fs.ErrPermission
		}
		root, e := os.OpenRoot(base.workspace)
		if e != nil {
			return e
		}
		closed := false
		closeRoot := func() error {
			if closed {
				return nil
			}
			closed = true
			return root.Close()
		}
		defer func() { result = errors.Join(result, closeRoot()) }()
		identity, e := root.Stat(".")
		if e != nil {
			return e
		}
		guard := func(c context.Context) error {
			if e := base.guard(c); e != nil {
				return e
			}
			info, e := os.Lstat(base.workspace)
			if e != nil {
				return e
			}
			if info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(identity, info) {
				return indexrepair.ErrSourceChanged
			}
			return nil
		}
		s := indexrepair.Source{Data: root, Authorize: func(c context.Context) error {
			if e := guard(c); e != nil {
				return e
			}
			if base.owner.persistenceReadOnly {
				return ErrSettingReadOnly
			}
			return nil
		}}
		if e = guard(c); e != nil {
			return e
		}
		out, e = workspacefile.Execute(c, s, q)
		if e != nil {
			return e
		}
		if e = guard(c); e != nil {
			return e
		}
		if e = closeRoot(); e != nil {
			return e
		}
		if q.Action == "write" || q.Action == "rename" || q.Action == "remove" {
			return IncSyncContext(c)
		}
		return nil
	})
	if err != nil {
		out = workspacefile.Result{}
	}
	return
}

func PutWorkspaceFileRequest(ctx context.Context, request *http.Request) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	if base.owner.persistenceReadOnly {
		return ErrSettingReadOnly
	}
	_, e = runSingleUploadMode(ctx, request, "file", 512<<20, true, func(c context.Context, owner *legacyReindexOwner, upload singleUpload) (struct{}, error) {
		if e := base.guard(c); e != nil {
			return struct{}{}, e
		}
		first := func(key string) string {
			v := upload.Values[key]
			if len(v) > 0 {
				return v[0]
			}
			return ""
		}
		name := first("path")
		if !util.IsValidUploadFileName(filepath.Base(name)) {
			return struct{}{}, indexrepair.ErrSourcePath
		}
		directory, _ := strconv.ParseBool(first("isDir"))
		if !directory && upload.Name == "" {
			return struct{}{}, errors.New("form file is nil")
		}
		modified := time.Now()
		if stamp := first("modTime"); stamp != "" {
			n, e := strconv.ParseInt(stamp, 10, 64)
			if e != nil {
				return struct{}{}, e
			}
			modified = time.UnixMilli(n)
		}
		_, e := WorkspaceFileContext(c, workspacefile.Request{Action: "write", Path: name, Directory: directory, Data: upload.Data, ModTime: modified}, true)
		return struct{}{}, e
	})
	return e
}
