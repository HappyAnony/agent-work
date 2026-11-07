package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/88250/lute/parse"
	"io/fs"
	"os"
	"strings"
	"time"
	"wiki-kernel/conf"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
)

type CreationEffect struct{ Kind, Box, Path, ID, Target string }
type NativeCreationPlan struct {
	Result         doclife.CreationResult
	Trees          map[string]*parse.Tree
	AttributeViews map[string]*av.AttributeView
	SourceHashes   map[string][32]byte
	Effects        []CreationEffect
}

// Prepare actual owner-local candidates only. Flush/sort/tag/mirror callbacks
// record intent; no source, AV, index or notification publication occurs here.
func (l *Lease) PlanNativeDocumentCreation(ctx context.Context, q doclife.CreationRequest) (ret NativeCreationPlan, err error) {
	q.Tree = nil
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		base, hashes, guard, e := r.documentLifecycleServices(ctx, w, bundle, q.Box)
		if e != nil {
			return e
		}
		trees := map[string]*parse.Tree{}
		var order []string
		ret = NativeCreationPlan{Trees: trees, SourceHashes: hashes, AttributeViews: map[string]*av.AttributeView{}}
		record := func(kind, box, p, id, target string) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			ret.Effects = append(ret.Effects, CreationEffect{kind, box, p, id, target})
			return nil
		}
		oldLookup, oldLoad, oldExists := base.Lookup, base.Load, base.Exists
		base.Lookup = func(c context.Context, id string) (doclife.Location, bool, error) {
			if t := trees[id]; t != nil {
				return doclife.Location{Box: t.Box, Path: t.Path, HPath: t.HPath, ID: t.ID, Document: true}, true, c.Err()
			}
			return oldLookup(c, id)
		}
		base.Load = func(c context.Context, box, p string) (*parse.Tree, error) {
			for _, id := range order {
				t := trees[id]
				if t.Box == box && t.Path == p {
					return t, c.Err()
				}
			}
			return oldLoad(c, box, p)
		}
		base.Exists = func(c context.Context, box, p string) (bool, error) {
			for _, id := range order {
				t := trees[id]
				if t.Box == box && t.Path == p {
					return true, c.Err()
				}
			}
			return oldExists(c, box, p)
		}
		load := func(c context.Context, id string) (*parse.Tree, error) {
			if t := trees[id]; t != nil {
				return t, c.Err()
			}
			loc, ok, e := oldLookup(c, id)
			if e != nil {
				return nil, e
			}
			if !ok {
				return nil, ErrDocument
			}
			if q.Mode == "duplicate" {
				if q.Box != "" && q.Box != loc.Box {
					return nil, ErrDocument
				}
				q.Box = loc.Box
			}
			return oldLoad(c, loc.Box, loc.Path)
		}
		stage := func(c context.Context, t *parse.Tree, mode string) error {
			if e := c.Err(); e != nil {
				return e
			}
			if t == nil || t.Root == nil || t.ID != t.Root.ID || !documentID.MatchString(t.ID) {
				return ErrDocument
			}
			if _, ok := trees[t.ID]; !ok {
				order = append(order, t.ID)
			}
			trees[t.ID] = t
			return record(mode, t.Box, t.Path, t.ID, "")
		}
		db, e := bundle.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		find := func(c context.Context, mode, box, value, parent string) (doclife.Location, bool, error) {
			for _, id := range order {
				t := trees[id]
				if t.Box != box {
					continue
				}
				ok := mode == "path" && t.Path == value || mode == "hpath" && t.HPath == value || mode == "preferred" && t.HPath == value && t.ID == parent
				if ok {
					return doclife.Location{Box: box, Path: t.Path, HPath: t.HPath, ID: t.ID, Document: true}, true, c.Err()
				}
			}
			kind := ""
			switch mode {
			case "path":
				kind = "root-path"
			case "hpath":
				kind = "root-hpath"
			case "preferred":
				kind = "preferred"
			default:
				return doclife.Location{}, false, doclife.ErrDependency
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(c, indexdb.BlockTreeFilter{Kind: kind, BoxID: box, Value: value, ParentID: parent})
			if e != nil || len(rows) == 0 {
				return doclife.Location{}, false, e
			}
			v := rows[0]
			return doclife.Location{Box: v.BoxID, Path: v.Path, HPath: v.HPath, ID: v.RootID, Document: true}, true, nil
		}
		main, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		parser, e := base.Parser(ctx)
		if e != nil {
			return e
		}
		now := time.Now
		funcs := avrender.BuiltInTemplateFuncs(now, parser, indexdb.TemplateFunctions(ctx, main))
		funcs["getHPathByID"] = func(id string) (string, error) {
			loc, ok, e := base.Lookup(ctx, id)
			if e != nil {
				return "", e
			}
			if !ok {
				return "", ErrDocument
			}
			return loc.HPath, ctx.Err()
		}
		funcs["statBlock"] = func(id string) (*docquery.StatResult, error) {
			return docquery.Statistics(ctx, docquery.StatRequest{Mode: "block", IDs: []string{id}}, docquery.StatServices{
				Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
					ret := map[string]*parse.Tree{}
					for _, id := range ids {
						t, e := load(c, id)
						if e != nil {
							return nil, e
						}
						ret[id] = t
					}
					return ret, c.Err()
				}, Parser: base.Parser,
				AttributeView: func(c context.Context, id string) (*av.AttributeView, error) {
					if a := ret.AttributeViews[id]; a != nil {
						return a, c.Err()
					}
					raw, _, e := readNativeAVSource(c, w, id)
					if e != nil {
						return nil, e
					}
					hashes["data/storage/av/"+id+".json"] = sha256.Sum256(raw)
					return decodeNativeAV(raw, id, now())
				},
			})
		}
		services := doclife.CreationServices{Strict: true, Box: base.Box, Find: find, Load: load, Parser: base.Parser,
			Create: func(c context.Context, box, p, title, dom string) (*parse.Tree, error) {
				plan, e := doclife.Prepare(c, doclife.Request{Action: "create-dom", Box: box, Path: p, Title: title, DOM: dom}, base)
				if e != nil {
					return nil, e
				}
				if e = stage(c, plan.Tree, "create"); e != nil {
					return nil, e
				}
				return plan.Tree, nil
			},
			Flush: func(c context.Context) error { return record("flush", q.Box, "", "", "") },
			Attrs: func(c context.Context, id string, attrs map[string]string) error {
				t, e := load(c, id)
				if e != nil {
					return e
				}
				for k, v := range attrs {
					if k == "tags" {
						v = markup.NormalizeTagValue(v)
					}
					if v == "" {
						t.Root.RemoveIALAttr(k)
					} else {
						t.Root.SetIALAttr(k, v)
					}
				}
				return stage(c, t, "attributes")
			}, Stage: stage,
			Sort: func(c context.Context, box, mode, p, id string) error { return record("sort-"+mode, box, p, id, "") }, Sync: func(c context.Context) error { return record("sync", q.Box, "", "", "") },
			Daily: func(c context.Context, box string) (doclife.DailySettings, error) {
				if !documentID.MatchString(box) {
					return doclife.DailySettings{}, ErrDocument
				}
				p := box + "/.siyuan/conf.json"
				raw, e := readNativeFile(c, w, p, 1<<20)
				if os.IsNotExist(e) {
					p = box + "/.wiki/conf.json"
					raw, e = readNativeFile(c, w, p, 1<<20)
				}
				if os.IsNotExist(e) {
					cfg := conf.NewBoxConf()
					return doclife.DailySettings{SavePath: cfg.DailyNoteSavePath, TemplatePath: cfg.DailyNoteTemplatePath}, c.Err()
				}
				if e != nil {
					return doclife.DailySettings{}, e
				}
				hashes["data/"+p] = sha256.Sum256(raw)
				cfg := conf.NewBoxConf()
				if e = json.Unmarshal(raw, cfg); e != nil {
					return doclife.DailySettings{}, e
				}
				return doclife.DailySettings{SavePath: cfg.DailyNoteSavePath, TemplatePath: cfg.DailyNoteTemplatePath}, nil
			},
			RenderPath: func(c context.Context, s string) (string, error) {
				return doclife.RenderCreationText(c, s, false, funcs, nil)
			},
			Template: func(c context.Context, p, id string) (*parse.Tree, string, error) {
				// The original notebook settings API stores template-root-relative
				// names with a leading slash. This is never a host absolute path.
				p = strings.TrimPrefix(p, "/")
				if !fs.ValidPath(p) {
					return nil, "", ErrDocument
				}
				raw, e := readNativeFile(c, w, "templates/"+p, 4<<20)
				if e != nil {
					return nil, "", e
				}
				hashes["data/templates/"+p] = sha256.Sum256(raw)
				t, e := load(c, id)
				if e != nil {
					return nil, "", e
				}
				model := map[string]string{"id": id, "title": t.Root.IALAttr("title"), "name": t.Root.IALAttr("name"), "alias": t.Root.IALAttr("alias")}
				text, e := doclife.RenderCreationText(c, string(raw), true, funcs, model)
				if e != nil {
					return nil, "", e
				}
				return doclife.RenderCreationBody(c, text, parser, doclife.TemplateBodyServices{NewID: base.NewID, Now: now,
					RefText: func(c context.Context, id string) (string, error) {
						return indexdb.ReferenceText(c, id, parser, r.nativeBlock)
					},
					CloneAV: func(c context.Context, id string) (string, error) {
						raw, _, e := readNativeAVSource(c, w, id)
						if e != nil {
							return "", e
						}
						hashes["data/storage/av/"+id+".json"] = sha256.Sum256(raw)
						a, e := decodeNativeAV(raw, id, now())
						if e != nil {
							return "", e
						}
						copy := a.Clone()
						if copy == nil {
							return "", ErrDocument
						}
						ret.AttributeViews[copy.ID] = copy
						if e = record("template-av", q.Box, "", copy.ID, id); e != nil {
							return "", e
						}
						return copy.ID, nil
					},
				})
			},
			Mirror: func(c context.Context, id, block string) error {
				raw, _, e := readNativeAVSource(c, w, id)
				if e != nil {
					return e
				}
				hashes["data/storage/av/"+id+".json"] = sha256.Sum256(raw)
				return record("mirror", q.Box, "", block, id)
			},
			Progress: func(c context.Context, start bool) error {
				if !start {
					return nil
				}
				return record("progress", q.Box, "", "", "")
			}, Now: now, NewID: base.NewID, Language: base.Language, Untitled: func() string { return base.PlainLanguage(16) },
		}
		ret.Result, e = doclife.CreatePeripheral(ctx, q, services)
		if e == nil {
			e = guard.Confirm(ctx)
		}
		return e
	})
	if err != nil {
		return NativeCreationPlan{}, err
	}
	return
}
