// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/repoview"
	"wiki-kernel/util"
)

func repoViewFile(file *entity.File) *repoview.File {
	if file == nil {
		return nil
	}
	return &repoview.File{ID: file.ID, Path: file.Path, Size: file.Size, Updated: file.Updated}
}
func repoViewFiles(files []*entity.File) []*repoview.File {
	out := make([]*repoview.File, 0, len(files))
	for _, f := range files {
		out = append(out, repoViewFile(f))
	}
	return out
}
func repoViewLogs(logs []*dejavu.Log) []*repoview.Log {
	out := make([]*repoview.Log, 0, len(logs))
	for _, l := range logs {
		if l == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, &repoview.Log{ID: l.ID, Memo: l.Memo, Created: l.Created, HCreated: l.HCreated, Files: repoViewFiles(l.Files), Count: l.Count, Size: l.Size, HSize: l.HSize, SystemID: l.SystemID, SystemName: l.SystemName, SystemOS: l.SystemOS, Tag: l.Tag, HTagUpdated: l.HTagUpdated})
	}
	return out
}

func (c *AppConf) executeRepositoryRead(ctx context.Context, v conf.AppData, q repoview.Request, p metadataRepoPaths) (out repoview.Result, err error) {
	var identity fs.FileInfo
	guard := func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if !c.restoreScope(p) {
			return cloudrepo.ErrDependency
		}
		if identity != nil {
			current, e := os.Lstat(p.Repo)
			if e != nil {
				return e
			}
			if current.Mode()&fs.ModeSymlink != 0 || !os.SameFile(identity, current) {
				return indexrepair.ErrSourceChanged
			}
		}
		return nil
	}
	if e := guard(ctx); e != nil {
		return out, e
	}
	if v.Repo == nil || v.Sync == nil || v.System == nil {
		return out, cloudrepo.ErrDependency
	}
	if len(v.Repo.Key) == 0 {
		return out, errors.New(c.business.Language(v.Lang, 26, true, v.CloudRegion))
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, transport := range transports {
			transport.CloseIdleConnections()
		}
	}()
	repo, e := newMetadataRepository(ctx, v, p)
	if e != nil {
		return out, e
	}
	defer repo.CloseMetadata()
	identity, e = os.Lstat(p.Repo)
	if e != nil {
		return out, e
	}
	if !identity.IsDir() || identity.Mode()&fs.ModeSymlink != 0 {
		return out, indexrepair.ErrSourcePath
	}
	root, e := os.OpenRoot(p.Repo)
	if e != nil {
		return out, e
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = repoview.Result{}
		}
	}()
	actual, e := root.Stat(".")
	if e != nil {
		return out, e
	}
	if !os.SameFile(identity, actual) {
		return out, indexrepair.ErrSourceChanged
	}
	source := indexrepair.Source{Data: root}
	relative := func(name string) (string, error) {
		if e := guard(ctx); e != nil {
			return "", e
		}
		rel, e := filepath.Rel(p.Repo, name)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return "", indexrepair.ErrSourcePath
		}
		return filepath.ToSlash(rel), nil
	}
	if e = repo.BindMetadataReadStorage(ctx, dejavu.MetadataReadStorage{
		ReadFile: func(name string) ([]byte, error) {
			rel, e := relative(name)
			if e != nil {
				return nil, e
			}
			return source.Read(ctx, rel)
		},
		ReadDir: func(name string) ([]fs.DirEntry, error) {
			rel, e := relative(name)
			if e != nil {
				return nil, e
			}
			return source.ReadDir(ctx, rel)
		},
		Stat: func(name string) (fs.FileInfo, error) {
			rel, e := relative(name)
			if e != nil {
				return nil, e
			}
			if rel != "." {
				if e = source.Check(rel, false); e != nil {
					return nil, e
				}
			}
			return root.Lstat(rel)
		},
	}); e != nil {
		return out, e
	}
	repo.BindAdmission(func() error { return guard(ctx) })
	ports := repoview.Ports{
		IndexFiles: func(c context.Context, id string) ([]*repoview.File, error) {
			if e := guard(c); e != nil {
				return nil, e
			}
			index, e := repo.GetIndex(id)
			if e != nil {
				return nil, e
			}
			files, e := repo.GetFiles(index)
			if e != nil {
				return nil, e
			}
			return repoViewFiles(files), guard(c)
		},
		File: func(c context.Context, id string) (repoview.File, error) {
			if e := guard(c); e != nil {
				return repoview.File{}, e
			}
			file, e := repo.GetFile(id)
			if e != nil {
				return repoview.File{}, e
			}
			raw, e := repo.OpenFile(file)
			if e != nil {
				return repoview.File{}, e
			}
			value := repoViewFile(file)
			if value == nil {
				return repoview.File{}, repoview.ErrRequest
			}
			value.Data = raw
			return *value, guard(c)
		},
		Logs: func(c context.Context, page int) ([]*repoview.Log, int, int, error) {
			if e := guard(c); e != nil {
				return nil, 0, 0, e
			}
			logs, pages, total, e := repo.GetIndexLogs(page, 32)
			if errors.Is(e, dejavu.ErrNotFoundIndex) {
				return []*repoview.Log{}, pages, total, nil
			}
			if e != nil {
				return nil, 0, 0, e
			}
			return repoViewLogs(logs), pages, total, guard(c)
		},
		Tags: func(c context.Context) ([]*repoview.Log, error) {
			if e := guard(c); e != nil {
				return nil, e
			}
			logs, e := repo.GetTagLogs()
			if e != nil {
				return nil, e
			}
			return repoViewLogs(logs), guard(c)
		},
		Diff: func(c context.Context, left, right string) (repoview.DiffInput, error) {
			if e := guard(c); e != nil {
				return repoview.DiffInput{}, e
			}
			diff, e := repo.DiffIndex(left, right)
			if e != nil {
				return repoview.DiffInput{}, e
			}
			if diff == nil || diff.LeftIndex == nil || diff.RightIndex == nil {
				return repoview.DiffInput{}, repoview.ErrRequest
			}
			return repoview.DiffInput{Left: repoview.DiffIndex{ID: diff.LeftIndex.ID, Created: diff.LeftIndex.Created}, Right: repoview.DiffIndex{ID: diff.RightIndex.ID, Created: diff.RightIndex.Created}, AddsLeft: repoViewFiles(diff.AddsLeft), UpdatesLeft: repoViewFiles(diff.UpdatesLeft), UpdatesRight: repoViewFiles(diff.UpdatesRight), RemovesRight: repoViewFiles(diff.RemovesRight)}, guard(c)
		},
		MIME: mime.TypeByExtension, Displayable: util.IsDisplayableAsset,
		StoreAsset: func(op context.Context, name string, raw []byte) (result string, err error) {
			if e := guard(op); e != nil {
				return "", e
			}
			if e := os.MkdirAll(p.Temp, 0755); e != nil {
				return "", e
			}
			root, e := os.OpenRoot(p.Temp)
			if e != nil {
				return "", e
			}
			defer func() {
				err = errors.Join(err, root.Close())
				if err != nil {
					result = ""
				}
			}()
			source := indexrepair.Source{Data: root, Authorize: guard}
			target := path.Join("repo", "diff", name)
			before, e := source.Read(op, target)
			if e != nil && !indexrepair.IsSourceNotExist(e) {
				return "", e
			}
			if e = source.Rewrite(op, target, before, raw); e != nil {
				return "", e
			}
			return target, guard(op)
		},
	}
	if q.Kind == "preview" || q.Kind == "diff" {
		options, e := v.ParserOptions(c.customEmojis.Snapshot())
		if e != nil {
			return out, e
		}
		ports.Parser = nativeparser.NewEditor(options)
	}
	out, err = repoview.Run(ctx, q, ports)
	if err == nil {
		err = guard(ctx)
	}
	if err != nil {
		return repoview.Result{}, err
	}
	if q.Kind == "preview" && out.Preview.Large {
		c.publishSyncUI(ctx, func() { util.PushMsg(c.business.Language(v.Lang, 36, true, v.CloudRegion), 5000) })
	}
	return
}

func ReadRepositoryContext(ctx context.Context, q repoview.Request) (out repoview.Result, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	service, e := o.owner.CloudRepoService()
	if e != nil {
		return out, e
	}
	err = o.run(ctx, func(op, life context.Context) error { var e error; out, e = service.ReadRepository(op, q); return e })
	if err != nil {
		return repoview.Result{}, err
	}
	return
}
func GetRepoFileContext(ctx context.Context, id string) ([]byte, string, error) {
	v, e := ReadRepositoryContext(ctx, repoview.Request{Kind: "file", ID: id})
	return v.Data, v.Path, e
}
func OpenRepoSnapshotDocContext(ctx context.Context, id string) (string, string, bool, int64, error) {
	v, e := ReadRepositoryContext(ctx, repoview.Request{Kind: "preview", ID: id})
	return v.Preview.Title, v.Preview.Content, v.Preview.Text, v.Preview.Updated, e
}
func DiffRepoSnapshotsContext(ctx context.Context, left, right string) (*LeftRightDiff, error) {
	v, e := ReadRepositoryContext(ctx, repoview.Request{Kind: "diff", Left: left, Right: right})
	return v.Diff, e
}
func GetRepoSnapshotsContext(ctx context.Context, page int) ([]*Snapshot, int, int, error) {
	v, e := ReadRepositoryContext(ctx, repoview.Request{Kind: "logs", Page: page})
	return v.Snapshots, v.Pages, v.Total, e
}
func GetTagSnapshotsContext(ctx context.Context) ([]*Snapshot, error) {
	v, e := ReadRepositoryContext(ctx, repoview.Request{Kind: "tags"})
	return v.Snapshots, e
}
