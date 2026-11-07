package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/indexdb"
)

func (r *resources) readNativeAV(ctx context.Context, w *WorkspaceContext, id string) (name string, views []avquery.View, err error) {
	if !documentID.MatchString(id) {
		return "", nil, ErrDocument
	}
	raw, err := readNativeFile(ctx, w, "storage/av/"+id+".json", 16<<20)
	if err != nil {
		return "", nil, err
	}
	var doc struct {
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Views []avquery.View `json:"views"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return "", nil, err
	}
	if doc.ID != "" && doc.ID != id {
		return "", nil, ErrDocument
	}
	return doc.Name, doc.Views, nil
}
func (l *Lease) NativeAttributeViewName(ctx context.Context, id string) (name string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		name, _, e = l.workspace.resources.readNativeAV(ctx, l.workspace, id)
		return e
	})
	return
}
func (l *Lease) SearchNativeAttributeViews(ctx context.Context, keyword string, exclude []string) (ret []*avquery.Result, err error) {
	return l.searchNativeAttributeViewsWithIO(ctx, keyword, exclude, nativeAVQueryIO{})
}

func (l *Lease) searchNativeAttributeViewsWithIO(ctx context.Context, keyword string, exclude []string, op nativeAVQueryIO) (ret []*avquery.Result, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		ret = []*avquery.Result{}
		if w.data == nil {
			return ErrDocument
		}
		manifest, e := confirmedNativeAVQueryManifest(ctx, w, op)
		if e != nil {
			if errors.Is(e, fs.ErrNotExist) {
				return nil
			}
			return e
		}
		manifestEntries := nativeAVQueryEntries(manifest)
		blocks, ok := manifestEntries["storage/av/blocks.msgpack"]
		if !ok {
			return confirmFinalNativeAVQueryManifest(ctx, w, manifest, op)
		}
		raw, e := readNativeAVQueryFile(ctx, w, blocks, 16<<20, op)
		if e != nil {
			return e
		}
		relations := map[string][]string{}
		if e = msgpack.Unmarshal(raw, &relations); e != nil {
			return e
		}
		if len(relations) > 100000 {
			return ErrBudget
		}
		type avDocument struct {
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Views []avquery.View `json:"views"`
		}
		documents := map[string]avDocument{}
		var metadata []avquery.Entry
		for _, entry := range manifest[1:] {
			if entry.info.IsDir() {
				continue
			}
			base := strings.TrimPrefix(entry.name, "storage/av/")
			id := strings.TrimSuffix(base, ".json")
			if !documentID.MatchString(id) {
				continue
			}
			if relations[id] == nil {
				continue
			}
			doc, exists := documents[id]
			if !exists {
				filename := "storage/av/" + id + ".json"
				source, found := manifestEntries[filename]
				if !found {
					_, e = op.fileInfo(ctx, w, filename)
					if e == nil {
						e = errNativeAVQuerySourceChanged
					}
					return e
				}
				raw, e = readNativeAVQueryFile(ctx, w, source, 16<<20, op)
				if e != nil {
					return e
				}
				if e = json.Unmarshal(raw, &doc); e != nil {
					return e
				}
				if doc.ID != "" && doc.ID != id {
					return ErrDocument
				}
				documents[id] = doc
			}
			metadata = append(metadata, avquery.Entry{ID: id, Name: doc.Name, Updated: entry.info.ModTime().UnixMilli()})
		}
		if !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		ret, e = avquery.Search(ctx, keyword, append([]string(nil), exclude...), avquery.Services{Strict: true, Entries: metadata, Relations: relations, Views: func(ctx context.Context, id string) ([]avquery.View, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			doc, ok := documents[id]
			if !ok {
				return nil, ErrDocument
			}
			return append([]avquery.View(nil), doc.Views...), nil
		}, Resolve: func(ctx context.Context, ids []string) (avquery.Location, bool, error) {
			for _, id := range ids {
				if !documentID.MatchString(id) {
					return avquery.Location{}, false, ErrDocument
				}
				tree, e := r.nativeTreeByBlock(ctx, w, id)
				if e != nil {
					if e == ErrDocument {
						continue
					}
					return avquery.Location{}, false, e
				}
				var found *ast.Node
				ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
					if enter && n.ID == id {
						found = n
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
				if found == nil || found.Type != ast.NodeAttributeView || found.AttributeViewID == "" {
					continue
				}
				hpath := tree.HPath
				if r.configurationBusiness == nil {
					return avquery.Location{}, false, conf.ErrBusinessUnbound
				}
				notebooks, e := r.configurationBusiness.Notebooks(ctx)
				if e != nil {
					return avquery.Location{}, false, e
				}
				for _, box := range notebooks {
					if box.ID == tree.Box {
						hpath = box.Name + hpath
						break
					}
				}
				return avquery.Location{BlockID: id, HPath: hpath}, true, nil
			}
			return avquery.Location{}, false, nil
		}})
		if e != nil {
			return e
		}
		if e = confirmFinalNativeAVQueryManifest(ctx, w, manifest, op); e != nil {
			ret = nil
			return e
		}
		return nil
	})
	return
}
