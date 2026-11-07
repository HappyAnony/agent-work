// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"html"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/av"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

type workspaceReadOwner struct {
	base          *legacyReindexOwner
	r             *indexRepairContext
	parser        *lute.Lute
	settings      conf.AppData
	parserOptions nativeparser.EditorOptions
}

func captureWorkspaceRead(ctx context.Context) (*workspaceReadOwner, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return nil, e
	}
	r.queryOnly = true
	o := &workspaceReadOwner{base: base, r: r}
	e = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		var err error
		o.settings, err = v.Clone()
		if err != nil {
			return err
		}
		options, e := v.ParserOptions(base.owner.customEmojis.Snapshot())
		if e != nil {
			return e
		}
		o.parser = nativeparser.NewEditor(options)
		o.parserOptions = options
		return base.guard(ctx)
	})
	return o, e
}

func (o *workspaceReadOwner) load(ctx context.Context, id string) (*parse.Tree, error) {
	bt, e := o.r.referenceLookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if bt == nil {
		return nil, ErrBlockNotFound
	}
	return o.r.loadRepairTree(ctx, bt.BoxID, bt.Path, o.parser)
}

func (o *workspaceReadOwner) avScope(ctx context.Context) (*commitAVContext, error) {
	env := legacyTransactionEnvironmentContext(ctx)
	env.ctx, env.parser = ctx, func() *lute.Lute { return o.parser }
	if e := bindLegacyCommit(ctx, env, o.r); e != nil {
		return nil, e
	}
	s := env.commitContext.avContext(ctx)
	s.readOnly = true
	return s, nil
}

func (o *workspaceReadOwner) funcs(ctx context.Context) template.FuncMap {
	f := avrender.BuiltInTemplateFuncs(time.Now, o.parser, indexdb.TemplateFunctions(ctx, o.r.main.DB))
	f["getHPathByID"] = func(id string) (string, error) {
		bt, e := o.r.referenceLookup(ctx, id)
		if e != nil {
			return "", e
		}
		if bt == nil {
			return "", ErrBlockNotFound
		}
		return bt.HPath, nil
	}
	f["statBlock"] = func(id string) (*docquery.StatResult, error) {
		return docquery.Statistics(ctx, docquery.StatRequest{Mode: "block", IDs: []string{id}}, docquery.StatServices{
			Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
				out := map[string]*parse.Tree{}
				for _, id := range ids {
					t, e := o.load(c, id)
					if e != nil {
						return nil, e
					}
					out[id] = t
				}
				return out, c.Err()
			}, Parser: func(c context.Context) (*lute.Lute, error) { return o.parser, o.base.guard(c) },
			AttributeView: func(c context.Context, id string) (*av.AttributeView, error) {
				s, e := o.avScope(c)
				if e != nil {
					return nil, e
				}
				return s.read(id)
			},
		})
	}
	return f
}

func templateData(t *parse.Tree, id string) (map[string]string, error) {
	n := docview.GetNodeInTree(t, id)
	if n == nil {
		return nil, ErrBlockNotFound
	}
	title := html.UnescapeString(n.IALAttr("name"))
	if n.Type == ast.NodeDocument {
		title = n.IALAttr("title")
	}
	return map[string]string{"id": id, "title": title, "name": html.UnescapeString(n.IALAttr("name")), "alias": html.UnescapeString(n.IALAttr("alias"))}, nil
}

func RenderGoTemplateContext(ctx context.Context, source string) (out string, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return "", e
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		out, e = doclife.RenderCreationText(op, source, false, o.funcs(op), nil)
		return e
	})
	if err != nil {
		return "", err
	}
	return
}

func RenderDynamicIconContentTemplateContext(ctx context.Context, source, id string) (out string, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return "", e
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		t, e := o.load(op, id)
		if e != nil {
			return e
		}
		data, e := templateData(t, id)
		if e != nil {
			return e
		}
		out, e = doclife.RenderCreationText(op, source, true, o.funcs(op), data)
		return e
	})
	if err != nil {
		return "", err
	}
	return
}

func RenderTemplateContext(ctx context.Context, p, id string, preview bool) (tree *parse.Tree, dom string, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return nil, "", e
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		rel, e := filepath.Rel(o.base.workspace, p)
		if e != nil || !filepath.IsLocal(rel) {
			return errors.New("template_path_invalid")
		}
		root, e := os.OpenRoot(o.base.workspace)
		if e != nil {
			return e
		}
		defer root.Close()
		raw, e := (ownedResourceFiles{owner: o.base, root: root}).read(op, rel)
		if e != nil {
			return e
		}
		t, e := o.load(op, id)
		if e != nil {
			return e
		}
		data, e := templateData(t, id)
		if e != nil {
			return e
		}
		text, e := doclife.RenderCreationText(op, string(raw), true, o.funcs(op), data)
		if e != nil {
			return e
		}
		services := doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now, RefText: func(c context.Context, id string) (string, error) {
			return indexdb.ReferenceText(c, id, o.parser, func(c context.Context, id string) (indexdb.BlockRecord, bool, error) {
				rows, e := o.r.main.Blocks(c, []string{id})
				if e != nil {
					return indexdb.BlockRecord{}, false, e
				}
				if len(rows) == 0 {
					return indexdb.BlockRecord{}, false, nil
				}
				return *rows[0], true, nil
			})
		}}
		if preview {
			services.PreviewAV = func(c context.Context, id, view string) ([]string, error) {
				s, e := o.avScope(c)
				if e != nil {
					return nil, e
				}
				a, e := s.read(id)
				if e != nil {
					return nil, e
				}
				v, e := a.GetCurrentView(view)
				if e != nil {
					return nil, e
				}
				return avrender.TableColumnNames(a, v)
			}
		} else {
			services.CloneAV = func(c context.Context, id string) (string, error) {
				s, e := o.avScope(c)
				if e != nil {
					return "", e
				}
				a, e := s.read(id)
				if e != nil {
					return "", e
				}
				copy := a.Clone()
				if copy == nil {
					return "", errors.New("template_av_clone_invalid")
				}
				raw, e := gulu.JSON.MarshalJSON(copy)
				if e != nil {
					return "", e
				}
				// Only this explicit mutation acquires a write binding. Read helpers
				// above cannot migrate source documents or persist AV upgrades.
				writeR, e := captureIndexRepair(c)
				if e != nil {
					return "", e
				}
				if writeR.owner != o.base.owner || writeR.data != o.base.data {
					return "", conf.ErrBusinessUnbound
				}
				env := legacyTransactionEnvironmentContext(c)
				env.ctx = c
				env.parser = func() *lute.Lute { return o.parser }
				if e = bindLegacyCommit(c, env, writeR); e != nil {
					return "", e
				}
				e = bindAVOperation(env.commitContext, c).createDuplicateSource(copy.ID, raw)
				return copy.ID, e
			}
		}
		tree, dom, e = doclife.RenderCreationBody(op, text, o.parser, services)
		return e
	})
	if err != nil {
		return nil, "", err
	}
	return
}

func DocSaveAsTemplateContext(ctx context.Context, id, name string, overwrite bool) (code int, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return 0, e
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		t, e := o.load(op, id)
		if e != nil {
			return e
		}
		raw, e := doclife.TemplateMarkdown(op, t, id, o.parser)
		if e != nil {
			return e
		}
		root, e := os.OpenRoot(o.base.data)
		if e != nil {
			return e
		}
		defer root.Close()
		name = filepath.Join("templates", doclife.TemplateFilename(name))
		p := filepath.Join(o.base.data, name)
		filelock.Lock(p)
		defer filelock.Unlock(p)
		o.base.owner.m.Lock()
		ro := o.base.owner.persistenceReadOnly
		o.base.owner.m.Unlock()
		if ro {
			return ErrSettingReadOnly
		}
		if e = o.base.guard(op); e != nil {
			return e
		}
		if _, e = root.Stat(name); e == nil && !overwrite {
			code = 1
			return nil
		} else if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = root.MkdirAll("templates", 0755); e != nil {
			return e
		}
		f, e := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		return errors.Join(e, f.Close())
	})
	return
}
