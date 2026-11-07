// SPDX-License-Identifier: AGPL-3.0-or-later
// Original snapshot read projections with explicit repository and device ports.
package repoview

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/88250/lute"
	"github.com/siyuan-note/dataparser"
	"io/fs"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/docview"
)

var ErrDependency = errors.New("repository_view_unbound")
var ErrRequest = errors.New("repository_view_request_invalid")

type File struct {
	ID, Path      string
	Size, Updated int64
	Data          []byte
}
type Log struct {
	ID          string  `json:"id"`
	Memo        string  `json:"memo"`
	Created     int64   `json:"created"`
	HCreated    string  `json:"hCreated"`
	Files       []*File `json:"files"`
	Count       int     `json:"count"`
	Size        int64   `json:"size"`
	HSize       string  `json:"hSize"`
	SystemID    string  `json:"systemID"`
	SystemName  string  `json:"systemName"`
	SystemOS    string  `json:"systemOS"`
	Tag         string  `json:"tag"`
	HTagUpdated string  `json:"hTagUpdated"`
}
type Snapshot struct {
	*Log
	TypesCount []*TypeCount `json:"typesCount"`
}
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}
type DiffIndex struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
}
type DiffFile struct {
	FileID  string `json:"fileID"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	HSize   string `json:"hSize"`
	Updated int64  `json:"updated"`
}
type Diff struct {
	LeftIndex    *DiffIndex  `json:"leftIndex"`
	RightIndex   *DiffIndex  `json:"rightIndex"`
	AddsLeft     []*DiffFile `json:"addsLeft"`
	UpdatesLeft  []*DiffFile `json:"updatesLeft"`
	UpdatesRight []*DiffFile `json:"updatesRight"`
	RemovesRight []*DiffFile `json:"removesRight"`
}
type DiffInput struct {
	Left, Right                                       DiffIndex
	AddsLeft, UpdatesLeft, UpdatesRight, RemovesRight []*File
}
type Preview struct {
	Title, Content string
	Text           bool
	Large          bool
	Updated        int64
}
type Request struct {
	Kind, ID, Left, Right, Path string
	Page                        int
}
type Result struct {
	Data         []byte
	Path         string
	Preview      Preview
	Diff         *Diff
	Snapshots    []*Snapshot
	Pages, Total int
}
type Ports struct {
	File        func(context.Context, string) (File, error)
	Logs        func(context.Context, int) ([]*Log, int, int, error)
	Tags        func(context.Context) ([]*Log, error)
	Diff        func(context.Context, string, string) (DiffInput, error)
	IndexFiles  func(context.Context, string) ([]*File, error)
	Parser      *lute.Lute
	MIME        func(string) string
	Displayable func(string) bool
	StoreAsset  func(context.Context, string, []byte) (string, error)
}

func Run(ctx context.Context, q Request, p Ports) (out Result, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("repository_view_panic:%T", v)
		}
		if err != nil {
			out = Result{}
		}
	}()
	if ctx == nil {
		return out, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	switch q.Kind {
	case "snapshot-file":
		if p.IndexFiles == nil || p.File == nil {
			return out, ErrDependency
		}
		if !fs.ValidPath(q.Path) || q.Path == "." {
			return out, ErrRequest
		}
		files, e := p.IndexFiles(ctx, q.ID)
		if e != nil {
			return out, e
		}
		for _, file := range files {
			if file == nil {
				return out, ErrRequest
			}
			if file.Path == "/"+q.Path {
				v, e := p.File(ctx, file.ID)
				if e != nil {
					return out, e
				}
				out.Data = append([]byte(nil), v.Data...)
				out.Path = file.Path
				break
			}
		}
	case "file", "preview":
		if p.File == nil {
			return out, ErrDependency
		}
		file, e := p.File(ctx, q.ID)
		if e != nil {
			return out, e
		}
		if q.Kind == "file" {
			out.Data = append([]byte(nil), file.Data...)
			out.Path = file.Path
			break
		}
		out.Preview, e = preview(ctx, file, p)
		if e != nil {
			return out, e
		}
	case "diff":
		if p.Diff == nil || p.File == nil || p.Parser == nil {
			return out, ErrDependency
		}
		input, e := p.Diff(ctx, q.Left, q.Right)
		if e != nil {
			return out, e
		}
		left, right := input.Left, input.Right
		out.Diff = &Diff{LeftIndex: &left, RightIndex: &right}
		// Original UI labels deliberately map repository removals to AddsLeft.
		groups := []struct {
			in  []*File
			out *[]*DiffFile
		}{{input.RemovesRight, &out.Diff.AddsLeft}, {input.AddsLeft, &out.Diff.RemovesRight}, {input.UpdatesLeft, &out.Diff.UpdatesLeft}, {input.UpdatesRight, &out.Diff.UpdatesRight}}
		for _, group := range groups {
			*group.out = []*DiffFile{}
			for _, file := range group.in {
				if e = ctx.Err(); e != nil {
					return out, e
				}
				if file == nil {
					return out, ErrRequest
				}
				title := path.Base(file.Path)
				if strings.HasSuffix(file.Path, ".docj") {
					content, e := p.File(ctx, file.ID)
					if e != nil {
						return out, e
					}
					tree, e := dataparser.ParseJSONWithoutFix(content.Data, p.Parser.ParseOptions)
					if e != nil {
						return out, e
					}
					if tree == nil || tree.Root == nil {
						return out, ErrRequest
					}
					title = tree.Root.IALAttr("title")
				}
				if title == "" {
					continue
				}
				*group.out = append(*group.out, &DiffFile{FileID: file.ID, Title: title, Path: file.Path, HSize: humanize.BytesCustomCeil(uint64(file.Size), 2), Updated: file.Updated})
			}
		}
	case "logs", "tags":
		var logs []*Log
		var e error
		if q.Kind == "logs" {
			if p.Logs == nil || q.Page < 1 {
				return out, ErrDependency
			}
			logs, out.Pages, out.Total, e = p.Logs(ctx, q.Page)
		} else {
			if p.Tags == nil {
				return out, ErrDependency
			}
			logs, e = p.Tags(ctx)
		}
		if e != nil {
			return out, e
		}
		out.Snapshots = []*Snapshot{}
		for _, log := range logs {
			if log == nil {
				return out, ErrRequest
			}
			copy := *log
			copy.Files = nil
			out.Snapshots = append(out.Snapshots, &Snapshot{Log: &copy, TypesCount: Types(log.Files)})
		}
	default:
		return out, ErrRequest
	}
	return out, ctx.Err()
}

func preview(ctx context.Context, file File, p Ports) (out Preview, err error) {
	out.Updated = file.Updated
	if strings.HasSuffix(file.Path, ".docj") {
		v, e := docview.RenderHistory(ctx, file.Data, "", false, false, p.Parser)
		if e != nil {
			return out, e
		}
		out.Title, out.Content, out.Text = v.Title, v.Content, v.Large
		out.Large = v.Large
		return out, ctx.Err()
	}
	out.Text, out.Title = true, file.Path
	if p.MIME == nil {
		return out, ErrDependency
	}
	mime := p.MIME(path.Ext(file.Path))
	if strings.HasPrefix(mime, "text/") || strings.Contains(mime, "json") {
		out.Content = string(file.Data)
		return out, ctx.Err()
	}
	if offset := strings.Index(file.Path, "assets/"); offset >= 0 {
		asset := file.Path[offset:]
		if !fs.ValidPath(asset) || path.Clean(asset) != asset || strings.Contains(asset, "\\") {
			return out, ErrRequest
		}
		if p.Displayable == nil {
			return out, ErrDependency
		}
		if p.Displayable(asset) {
			if p.StoreAsset == nil {
				return out, ErrDependency
			}
			out.Content, err = p.StoreAsset(ctx, asset, append([]byte(nil), file.Data...))
		}
	} else {
		out.Content = file.Path
	}
	return out, err
}

// Types retains the original count ordering and ten-item compatibility shape.
func Types(files []*File) (out []*TypeCount) {
	for _, file := range files {
		if file == nil {
			continue
		}
		ext := path.Ext(file.Path)
		if ext == "." {
			ext = ""
		}
		if ext == "" {
			ext = "NoExt"
		}
		found := false
		for _, v := range out {
			if v.Type == ext {
				v.Count++
				found = true
				break
			}
		}
		if !found {
			out = append(out, &TypeCount{Type: ext, Count: 1})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > 10 {
		out = append(out[:10], &TypeCount{Type: "Other", Count: 0})
	}
	return
}
