// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/siyuan-note/filelock"
	stdio "io"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/gabriel-vasile/mimetype"
	"wiki-kernel/av"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

// One compatibility commit callback owns this detached AV graph, source versions
// and render recursion. No native write grant, process selector or worker lives here.
type commitAVContext struct {
	readOnly bool
	// Optional read-only export sources share the enclosing first-read snapshot.
	sourceReader func(string) ([]byte, error)
	treeReader   func([]string) (map[string]*parse.Tree, error)
	ctx          context.Context
	commit       *legacyCommitContext
	versions     map[string][]byte
	cached       map[string]*av.AttributeView
	active       map[string]bool
	depth        int
	now          time.Time
}

func (c *legacyCommitContext) avContext(ctx context.Context) *commitAVContext {
	return &commitAVContext{ctx: ctx, commit: c, versions: map[string][]byte{}, cached: map[string]*av.AttributeView{}, active: map[string]bool{}, now: time.Now()}
}
func (s *commitAVContext) guard() error { return s.commit.r.guard(s.ctx) }
func (s *commitAVContext) bind(a *av.AttributeView) {
	a.ReadAttributeView = s.read
	a.DurationFormatter = s.commit.duration
	a.ErrorSink = nil
	if a.RenderedViewables == nil {
		a.RenderedViewables = map[string]av.Viewable{}
	}
}

// Bridge the original in-process file lock while borrowing only the fixed root.
func (s *commitAVContext) source(name string) (repairIOResult []byte, repairIOErr error) {
	if e := s.guard(); e != nil {
		return nil, e
	}
	if s.sourceReader != nil {
		if !s.readOnly {
			return nil, errors.New("AV export source requires read-only scope")
		}
		raw, err := s.sourceReader(name)
		if err != nil {
			return nil, err
		}
		return raw, s.guard()
	}
	root, closeIO, e := s.commit.r.openRepairIOChecked(s.ctx, false)
	if e != nil {
		return nil, e
	}
	defer func() {
		repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
		if repairIOErr != nil {
			repairIOResult = nil
		}
	}()
	filename := filepath.Join(s.commit.r.data, filepath.FromSlash(name))
	filelock.Lock(filename)
	defer filelock.Unlock(filename)
	return root.Source.Read(s.ctx, name)
}

func (s *commitAVContext) read(id string) (*av.AttributeView, error) { return s.load(id, true) }
func (s *commitAVContext) load(id string, upgrade bool) (*av.AttributeView, error) {
	if e := s.guard(); e != nil {
		return nil, e
	}
	if !ast.IsNodeIDPattern(id) {
		return nil, avrender.ErrShape
	}
	if a := s.cached[id]; a != nil {
		return a, nil
	}
	raw, e := s.source("storage/av/" + id + ".json")
	if e != nil {
		return nil, e
	}
	a, e := av.DecodeAttributeView(raw)
	if e != nil {
		return nil, e
	}
	if a == nil || a.ID != id {
		return nil, avrender.ErrShape
	}
	// Old-format adaptation is detached; only an explicit later save may publish it.
	if upgrade {
		if e = avmodel.UpgradeInMemory(a, avmodel.UpgradeEnvironment{Now: func() time.Time { return s.now }, NewID: ast.NewNodeID}); e != nil {
			return nil, e
		}
	}
	if upgrade {
		if e = validateUpdatedAttributeView(a); e != nil {
			return nil, e
		}
	} else {
		// Live initialization must allow the original check to repair value type
		// mismatches before the stricter render/save validation runs.
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil {
				return nil, avrender.ErrShape
			}
			for _, v := range kv.Values {
				if v == nil {
					return nil, avrender.ErrShape
				}
			}
		}
		for _, v := range a.Views {
			if v == nil {
				return nil, avrender.ErrShape
			}
		}
	}
	if a.GetBlockKeyValues() == nil {
		return nil, avrender.ErrShape
	}
	s.versions[id] = append([]byte(nil), raw...)
	s.bind(a)
	s.cached[id] = a
	return a, s.guard()
}
func (s *commitAVContext) clone(a *av.AttributeView) (*av.AttributeView, error) {
	copy, e := cloneAttributeViewPreview(a)
	if e != nil {
		return nil, e
	}
	s.bind(copy)
	return copy, s.guard()
}
func (s *commitAVContext) save(a *av.AttributeView) (repairIOErr error) {
	if s.readOnly {
		return errors.New("AV read-only scope cannot save")
	}
	if e := s.guard(); e != nil {
		return e
	}
	before, ok := s.versions[a.ID]
	if !ok {
		return errors.New("AV source revision unbound")
	}
	if e := validateUpdatedAttributeView(a); e != nil {
		return e
	}
	if a.GetBlockKeyValues() == nil {
		return avrender.ErrShape
	}
	raw, e := av.EncodeAttributeView(a, s.commit.r.singleLine)
	if e != nil {
		return e
	}
	raw, e = docast.SerializeAttributeView(before, raw)
	if e != nil {
		return e
	}
	io, closeIO, e := s.commit.r.openRepairIOChecked(s.ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	if e = io.Source.Rewrite(s.ctx, "storage/av/"+a.ID+".json", before, raw); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		delete(s.versions, a.ID)
		delete(s.cached, a.ID)
		return e
	}
	s.versions[a.ID] = append([]byte(nil), raw...)
	s.cached[a.ID] = a
	if s.commit.r.warningSize > 0 && len(raw) > s.commit.r.warningSize*1024*1024 && s.commit.avWarning != nil {
		s.commit.avWarning(fmt.Sprintf(s.commit.r.language(268), a.Name+" "+a.ID+".json", s.commit.r.warningSize))
	}
	return nil
}
func (s *commitAVContext) sources(id string) ([]string, error) {
	if e := s.guard(); e != nil {
		return nil, e
	}
	return av.ReadSourceAVIDs(s.ctx, id, func() ([]byte, error) { return s.source("storage/av/relations.msgpack") })
}

func (s *commitAVContext) title(id string) (string, error) {
	a, e := s.read(id)
	if e != nil {
		return "", e
	}
	return a.Name, nil
}
func (s *commitAVContext) text(n *ast.Node, id string) (string, string, error) {
	if e := s.guard(); e != nil {
		return "", "", e
	}
	return attributeViewBlockTextWithServices(n, id, func(n *ast.Node) (string, error) {
		switch n.Type {
		case ast.NodeBlockQueryEmbed:
			return "Query Embed Block...", nil
		case ast.NodeIFrame:
			return "IFrame...", nil
		case ast.NodeThematicBreak:
			return "Thematic Break...", nil
		case ast.NodeVideo:
			return "Video...", nil
		case ast.NodeAudio:
			return "Audio...", nil
		case ast.NodeAttributeView:
			t, e := s.title(n.AttributeViewID)
			if e != nil {
				return "", e
			}
			if t == "" {
				t = "Database..."
			}
			return t, nil
		}
		if n.Type != ast.NodeDocument && n.IsContainerBlock() {
			n = anchor.FirstLeaf(n)
		}
		text, e := docquery.BlockText(s.ctx, n, nil, false, func(ctx context.Context, n *ast.Node, exclude []string) (string, error) {
			return indexdb.StaticContent(ctx, n, exclude, false, false, false, indexdb.ContentServices{AttributeView: func(ctx context.Context, id string, full bool) (string, error) { return s.title(id) }})
		})
		if e == nil && utf8.RuneCountInString(text) > 1024 {
			text = string([]rune(text)[:1024]) + "..."
		}
		return text, e
	}, func() string { return s.commit.avUntitled })
}
func (s *commitAVContext) trees(ids []string) (repairIOResult map[string]*parse.Tree, repairIOErr error) {
	if e := s.guard(); e != nil {
		return nil, e
	}
	if s.treeReader != nil {
		if !s.readOnly {
			return nil, errors.New("AV export trees require read-only scope")
		}
		trees, err := s.treeReader(ids)
		if err != nil {
			return nil, err
		}
		return trees, s.guard()
	}
	io, closeIO, e := s.commit.r.openRepairIOChecked(s.ctx, false)
	if e != nil {
		return nil, e
	}
	defer func() {
		repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
		if repairIOErr != nil {
			repairIOResult = nil
		}
	}()
	// Query-only document loads must not replace an admitted transaction revision.
	if s.readOnly {
		io.Remember = nil
	}
	ret := map[string]*parse.Tree{}
	for _, id := range ids {
		bt, e := s.commit.r.referenceLookup(s.ctx, id)
		if e != nil {
			return nil, e
		}
		if bt == nil {
			continue
		}
		t, e := io.Load(s.ctx, bt.BoxID, bt.Path, s.commit.parser(), false)
		if e != nil {
			return nil, e
		}
		ret[id] = t
	}
	return ret, s.guard()
}
func (s *commitAVContext) attrs(ids []string) (map[string]map[string]string, error) {
	trees, e := s.trees(ids)
	if e != nil {
		return nil, e
	}
	ret := map[string]map[string]string{}
	for id, t := range trees {
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.ID == id {
				ret[id] = parse.IAL2MapUnEsc(n.KramdownIAL)
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
	}
	return ret, s.guard()
}
func (s *commitAVContext) render(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
	if v == nil {
		return nil, avrender.ErrShape
	}
	return s.renderAs(a, v, q, v.LayoutType)
}
func (s *commitAVContext) renderAs(a *av.AttributeView, v *av.View, q string, layout av.LayoutType) (av.Viewable, error) {
	if a == nil || v == nil {
		return nil, avrender.ErrShape
	}
	if (layout == av.LayoutTypeTable && v.Table == nil) || (layout == av.LayoutTypeGallery && v.Gallery == nil) || (layout == av.LayoutTypeKanban && v.Kanban == nil) {
		return nil, avrender.ErrShape
	}
	if e := s.guard(); e != nil {
		return nil, e
	}
	key := a.ID + "/" + v.ID
	if s.active[key] || s.depth >= 7 {
		return nil, avrender.ErrLimit
	}
	s.active[key] = true
	s.depth++
	defer func() { delete(s.active, key); s.depth-- }()
	s.bind(a)
	templates := s.templates()
	env := avrender.Environment{Trees: s.trees, Cover: avrender.CoverServices{Parser: s.commit.parser(), IsImage: func(p string) (repairIOResult bool, repairIOErr error) {
		ext := strings.ToLower(path.Ext(p))
		if ext != "" {
			return avrender.IsImageExtension(ext), nil
		}
		io, closeIO, e := s.commit.r.openRepairIOChecked(s.ctx, false)
		if e != nil {
			return false, e
		}
		defer func() {
			repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
			if repairIOErr != nil {
				repairIOResult = false
			}
		}()
		if e = io.Source.Check(p, false); e != nil {
			return false, e
		}
		f, e := io.Source.Data.Open(p)
		if e != nil {
			return false, e
		}
		defer func() {
			repairIOErr = joinRepairCloseError(repairIOErr, f.Close())
			if repairIOErr != nil {
				repairIOResult = false
			}
		}()
		info, e := f.Stat()
		if e != nil {
			return false, e
		}
		if !info.Mode().IsRegular() {
			return false, avrender.ErrShape
		}
		mime, e := mimetype.DetectReader(stdio.LimitReader(f, 3072))
		if e != nil {
			return false, e
		}
		return avrender.IsImageExtension(mime.Extension()), s.guard()
	}},
		RemoveMissingField: func(a *av.AttributeView, v *av.View, id string) error {
			if avrender.RemoveMissingField(v, id) && !s.readOnly {
				return s.save(a)
			}
			return nil
		},
		FilterItems: func(items map[string][]*av.KeyValues) error {
			for row, values := range items {
				b := av.GetKeyBlockValue(values)
				if b == nil || b.Block == nil {
					delete(items, row)
					continue
				}
				if b.IsDetached {
					continue
				}
				if b.Block.ID == "" {
					delete(items, row)
					continue
				}
				bt, e := s.commit.r.referenceLookup(s.ctx, b.Block.ID)
				if e != nil {
					return e
				}
				if bt == nil {
					delete(items, row)
				}
			}
			return s.guard()
		}, BlockAttrs: s.attrs,
	}
	env.AutoValues = func(a *av.AttributeView, c av.Collection, ials map[string]map[string]string) error {
		avrender.FillBasicAutoValues(a, c, ials, func() time.Time { return s.now })
		return avrender.FillRelatedValues(a, c, avrender.RelatedEnvironment{Read: s.read, Render: s.render, Cache: s.cached, Duration: s.commit.duration, Fallback: func(a *av.AttributeView, v *av.View, c av.Collection) error {
			return avrender.FillTemplateValues(a, v, c, ials, templates)
		}})
	}
	env.Templates = func(a *av.AttributeView, v *av.View, c av.Collection, ials map[string]map[string]string) error {
		return avrender.FillTemplateValues(a, v, c, ials, templates)
	}
	var out av.Viewable
	var e error
	switch layout {
	case av.LayoutTypeTable:
		out, e = avrender.Table(a, v, q, env)
	case av.LayoutTypeGallery:
		out, e = avrender.Gallery(a, v, q, env)
	case av.LayoutTypeKanban:
		out, e = avrender.Kanban(a, v, q, env)
	default:
		return nil, avrender.ErrUnsupported
	}
	if e != nil {
		return nil, e
	}
	if out == nil {
		return nil, avrender.ErrShape
	}
	a.RenderedViewables[out.GetID()] = out
	return out, s.guard()
}
func (s *commitAVContext) groups(a *av.AttributeView) error {
	for _, v := range a.Views {
		if e := s.guard(); e != nil {
			return e
		}
		if v.GetGroupKey(a) == nil {
			continue
		}
		if e := avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: func() time.Time { return s.now }, Read: s.read, Render: s.render, NewView: avrender.NewGroupView}); e != nil {
			return e
		}
	}
	return s.guard()
}
func (s *commitAVContext) flush(ids []string, reload func(string) error) error {
	return flushRelatedAttributeViews(s.ctx, ids, s.read, s.groups, s.save, reload, false)
}
func (s *commitAVContext) refresh(nodes map[string]*ast.Node, reload func(string) error) error {
	e := &attributeViewTextEnvironment{ctx: s.ctx, read: s.read, clone: s.clone, text: s.text, stage: s.save, reload: reload, related: func(id string) error {
		return collectRelatedAttributeViews(s.ctx, id, s.sources, func(ids []string) error { return s.flush(ids, reload) })
	}}
	return updateAttributeViewBlockTextWithEnvironment(nodes, e)
}

// One captured function environment for rendering and default-field dependency parsing.
func (s *commitAVContext) templates() avrender.TemplateEnvironment {
	templates := avrender.TemplateEnvironment{Context: s.ctx, Now: func() time.Time { return s.now }, Name: func(a *av.AttributeView) string {
		if strings.TrimSpace(a.Name) == "" {
			return s.commit.avUntitled
		}
		return strings.TrimSpace(a.Name)
	}, Funcs: avrender.BuiltInTemplateFuncs(func() time.Time { return s.now }, s.commit.parser(), indexdb.TemplateFunctions(s.ctx, s.commit.r.main.DB))}
	templates.Funcs["getHPathByID"] = func(id string) (string, error) {
		bt, e := s.commit.r.referenceLookup(s.ctx, id)
		if e != nil || bt == nil {
			return "", e
		}
		return bt.HPath, nil
	}
	templates.Funcs["statBlock"] = func(id string) (*avrender.BlockStatResult, error) {
		trees, e := s.trees([]string{id})
		if e != nil {
			return nil, e
		}
		t := trees[id]
		if t == nil {
			return nil, ErrBlockNotFound
		}
		var n *ast.Node
		ast.Walk(t.Root, func(v *ast.Node, enter bool) ast.WalkStatus {
			if enter && v.ID == id {
				n = v
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if n == nil {
			return nil, ErrBlockNotFound
		}
		if n.Type == ast.NodeDocument {
			return avrender.StatTree(t, s.commit.parser(), s.read)
		}
		r, w, l, i, f := n.Stat()
		return &avrender.BlockStatResult{RuneCount: r, WordCount: w, LinkCount: l, ImageCount: i, RefCount: f, BlockCount: 1}, nil
	}
	return templates
}
