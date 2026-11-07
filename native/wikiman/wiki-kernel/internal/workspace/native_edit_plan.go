// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/docquery"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
)

type NativeEditPlan struct {
	Changes       []sourcetx.Change
	ReadGuards    *sourcetx.ReadGuards
	Operations    []*editcore.Operation
	Notices       []NativeEditNotice
	DirectSources map[string]string
}

type NativeEditNotice struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Data any    `json:"data,omitempty"`
}

type nativeEditFile struct {
	before, after []byte
	existed       bool
}

type nativeEdit struct {
	ctx         context.Context
	w           *WorkspaceContext
	r           *resources
	bundle      *indexdb.Bundle
	parser      *lute.Lute
	indexer     *lute.Lute
	now         time.Time
	tx          *editcore.DocumentTransaction
	before      map[string][]byte
	normalized  map[string][]byte
	anchorText  map[[2]string]string
	dirty       map[string]bool
	avs         map[string]*av.AttributeView
	avBefore    map[string][]byte
	avDirty     map[string]bool
	files       map[string]nativeEditFile
	related     map[string]bool
	notices     []NativeEditNotice
	content     indexdb.ContentServices
	bound       *editcore.AttributeViewBoundEnvironment
	cell        *editcore.AttributeViewCellEnvironment
	defaults    *editcore.AttributeViewDefaultEnvironment
	direct      map[string]string
	ignoreLines []string
}

// PlanNativeEdits runs original operations against one private AST/AV/SQL view.
// It returns only checked candidates; source publication belongs to sourcetx.
func (l *Lease) PlanNativeEdits(ctx context.Context, operations []*editcore.Operation) (NativeEditPlan, error) {
	return l.planNativeEdits(ctx, operations, nil, nil)
}

// PlanNativeDocumentDeleteEffects reuses the original AV binding algorithm on
// the disappearing document roots. The lifecycle transaction owns publication.
func (l *Lease) PlanNativeDocumentDeleteEffects(ctx context.Context, roots []string) (NativeEditPlan, error) {
	if len(roots) == 0 {
		return NativeEditPlan{}, ErrDocument
	}
	return l.planNativeEdits(ctx, nil, slices.Clone(roots), nil)
}

func (l *Lease) planNativeEdits(ctx context.Context, operations []*editcore.Operation, deleteRoots []string, tag *nativeTagRename) (out NativeEditPlan, err error) {
	defer func() {
		if v := recover(); v != nil {
			out = NativeEditPlan{}
			err = fmt.Errorf("original_edit_failed: %T", v)
		}
	}()
	if len(deleteRoots) == 0 && tag == nil {
		if err = editcore.ValidateOperations(ctx, operations); err != nil {
			return out, err
		}
	}
	raw, err := json.Marshal(operations)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &operations); err != nil {
		return out, err
	}
	projection, err := l.PrepareNativeReads(ctx, false)
	if err != nil {
		return out, err
	}
	err = l.WithNativeIndexes(ctx, func(original *indexdb.Bundle) (err error) {
		r, w := l.workspace.resources, l.workspace
		before, err := r.readSourceSnapshot(ctx, w)
		if err != nil {
			return err
		}
		if before.hash != projection.SourceHash {
			return ErrStale
		}
		candidate, err := original.CloneReadProjection(ctx)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, candidate.Close()) }()
		// The serial owner gate excludes other readers. Restoring the original
		// pointer before release prevents even a failed plan publishing its SQL.
		r.native = candidate
		defer func() { r.native = original }()
		opts, err := r.configuredParserOptions(ctx, w)
		if err != nil {
			return err
		}
		p := &nativeEdit{ctx: ctx, w: w, r: r, bundle: candidate, parser: nativeparser.NewEditor(opts), indexer: nativeparser.NewConfiguredIndex(opts), now: time.Now(), before: map[string][]byte{}, normalized: map[string][]byte{}, anchorText: map[[2]string]string{}, dirty: map[string]bool{}, avs: map[string]*av.AttributeView{}, avBefore: map[string][]byte{}, avDirty: map[string]bool{}, files: map[string]nativeEditFile{}, related: map[string]bool{}}
		ignore, e := readNativeFile(ctx, w, ".wiki/indexignore", 1<<20)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		p.ignoreLines = strings.Split(strings.ReplaceAll(string(ignore), "\r\n", "\n"), "\n")
		p.content = r.nativeContent
		p.content.AttributeView = func(ctx context.Context, id string, full bool) (string, error) {
			a, e := p.readAV(id)
			if e != nil {
				return "", e
			}
			if full {
				return avrender.SearchText(ctx, a)
			}
			return a.Name, ctx.Err()
		}
		p.tx = &editcore.DocumentTransaction{Trees: map[string]*parse.Tree{}, Nodes: map[string]*ast.Node{}, LuteEngine: p.parser, DoOperations: operations}
		p.bind()
		p.direct = map[string]string{}
		if tag != nil {
			if e := p.renameTag(*tag, before.files); e != nil {
				return e
			}
		}
		for _, op := range operations {
			if e := p.captureTarget(op); e != nil {
				return e
			}
		}
		large, failure := p.tx.ProcessLargeInsert()
		if failure != nil {
			return failure
		}
		if !large {
			p.tx.ProcessUndoInsertWithFoldedHeading()
			for _, op := range operations {
				if e := p.captureTarget(op); e != nil {
					return e
				}
				if err = p.run(op); err != nil {
					return err
				}
			}
		}
		for _, id := range deleteRoots {
			n, t, e := p.node(id)
			if e != nil {
				return e
			}
			if n == nil || n != t.Root {
				return ErrDocument
			}
			if e = editcore.DeleteAttributeViewBindings(n, t, p.bound); e != nil {
				return e
			}
		}
		if err = p.finish(); err != nil {
			return err
		}
		for id := range p.dirty {
			t := p.tx.Trees[id]
			if _, e := docview.PrepareWriteTree(ctx, t, func() *ast.Node { return docview.NewParagraph("") }); e != nil {
				return e
			}
			if e := p.stage(t); e != nil {
				return e
			}
			encoded, e := docview.EncodeWriteTree(t, p.parser, p.singleLine())
			if e != nil {
				return e
			}
			inputs := make([][]byte, 0, len(p.before))
			for _, raw := range p.before {
				inputs = append(inputs, raw)
			}
			originals := make([][]byte, 0, len(p.normalized))
			for _, raw := range p.normalized {
				originals = append(originals, raw)
			}
			encoded, err = editcore.SerializeDocument(inputs, encoded, docast.DocumentPreservation{Originals: originals, AnchorText: p.anchorText})
			if err != nil {
				return err
			}
			name, e := treeRelative(t.Box, t.Path)
			if e != nil {
				return e
			}
			if e = p.putFile(name, encoded); e != nil {
				return e
			}
		}
		if e := p.validateFinalIdentities(original); e != nil {
			return e
		}
		for id := range p.avDirty {
			encoded, e := av.EncodeAttributeView(p.avs[id], p.singleLine())
			if e != nil {
				return e
			}
			encoded, e = editcore.SerializeAttributeView(p.avBefore[id], encoded)
			if e != nil {
				return e
			}
			if e = p.putFile("storage/av/"+id+".json", encoded); e != nil {
				return e
			}
		}
		guards := &sourcetx.ReadGuards{Files: map[string]string{}, Directories: map[string][]string{}}
		prefix := "pages/" + w.paths.PageID + "/data/"
		for name, hash := range before.files {
			guards.Files[prefix+name] = hash
		}
		for _, dir := range before.directories {
			children := make([]string, 0, len(dir.entries))
			for _, entry := range dir.entries {
				children = append(children, path.Base(entry.name))
			}
			slices.Sort(children)
			guards.Directories[path.Join(prefix, dir.name)] = children
		}
		changes := []sourcetx.Change{}
		for name, f := range p.files {
			if slices.Equal(f.before, f.after) && f.existed {
				continue
			}
			hash := ""
			if f.existed {
				hash = sourcetx.Hash(f.before)
			}
			changes = append(changes, sourcetx.Change{Path: prefix + name, ExpectedHash: hash, Data: f.after})
		}
		slices.SortFunc(changes, func(a, b sourcetx.Change) int { return strings.Compare(a.Path, b.Path) })
		after, e := r.readSourceSnapshot(ctx, w)
		if e != nil {
			return e
		}
		if before.hash != after.hash {
			return ErrStale
		}
		out = NativeEditPlan{Changes: changes, ReadGuards: guards, Operations: operations, Notices: p.notices, DirectSources: p.direct}
		return ctx.Err()
	})
	if err != nil {
		return NativeEditPlan{}, err
	}
	return out, nil
}

// A move may temporarily stage both trees in either order. Validate the
// final ownership against the original projection before source publication.
func (p *nativeEdit) validateFinalIdentities(original *indexdb.Bundle) error {
	db, e := original.Database(indexdb.BlockTree)
	if e != nil {
		return e
	}
	owners := map[string]string{}
	ids := []string{}
	for id := range p.dirty {
		for _, row := range indexdb.ProjectBlockTree(p.tx.Trees[id]) {
			if _, duplicate := owners[row.ID]; duplicate {
				return ErrDocument
			}
			owners[row.ID] = id
			ids = append(ids, row.ID)
		}
	}
	old, e := (indexdb.BlockTrees{DB: db}).Query(p.ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
	if e != nil {
		return e
	}
	for _, row := range old {
		if !p.dirty[row.RootID] && owners[row.ID] != row.RootID {
			return ErrDocument
		}
	}
	return p.ctx.Err()
}

func (p *nativeEdit) singleLine() bool {
	return p.r.configuration.FileTree == nil || p.r.configuration.FileTree.UseSingleLineSave
}

func (p *nativeEdit) captureTarget(op *editcore.Operation) error {
	if strings.Contains(op.Action, "AttrView") && op.Action != "insertAttrViewTable" {
		id := op.AvID
		if op.Action == "setAttrViewName" {
			id = op.ID
		}
		if !documentID.MatchString(id) {
			return ErrDocument
		}
		name := "storage/av/" + id + ".json"
		raw, e := readNativeFile(p.ctx, p.w, name, 16<<20)
		if e != nil {
			return e
		}
		p.direct[name] = sourcetx.Hash(raw)
		return nil
	}
	ids := []string{op.ID, op.ParentID, op.PreviousID, op.NextID}
	ids = append(ids, op.BlockIDs...)
	for _, id := range ids {
		if id == "" {
			continue
		}
		loc, e := p.lookup(id)
		if e != nil {
			return e
		}
		if loc == nil {
			continue
		}
		name, e := treeRelative(loc.BoxID, loc.Path)
		if e != nil {
			return e
		}
		if _, ok := p.direct[name]; ok {
			continue
		}
		raw, e := readNativeFile(p.ctx, p.w, name, 16<<20)
		if e != nil {
			return e
		}
		p.direct[name] = sourcetx.Hash(raw)
	}
	return p.ctx.Err()
}

func (p *nativeEdit) lookup(id string) (*editcore.BlockLocation, error) {
	if e := p.ctx.Err(); e != nil {
		return nil, e
	}
	if !documentID.MatchString(id) {
		return nil, nil
	}
	db, e := p.bundle.Database(indexdb.BlockTree)
	if e != nil {
		return nil, e
	}
	rows, e := (indexdb.BlockTrees{DB: db}).Query(p.ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if e != nil || len(rows) == 0 {
		return nil, e
	}
	v := editcore.BlockLocation(rows[0])
	return &v, nil
}

func (p *nativeEdit) load(box, name string, _ *lute.Lute) (*parse.Tree, error) {
	id := strings.TrimSuffix(path.Base(name), ".docj")
	if t := p.tx.Trees[id]; t != nil {
		if t.Box != box || t.Path != name {
			return nil, ErrDocument
		}
		return t, p.ctx.Err()
	}
	rel, e := treeRelative(box, name)
	if e != nil {
		return nil, e
	}
	raw, e := readNativeFile(p.ctx, p.w, rel, 16<<20)
	if e != nil {
		return nil, e
	}
	t, e := p.r.parseOriginalTree(p.ctx, p.w, cachedDocument{raw: raw, path: name}, box, id)
	if e != nil {
		return nil, e
	}
	if _, e = locateOriginalTree(p.ctx, p.w, t, box); e != nil {
		return nil, e
	}
	// Match the same original write normalization when comparing untouched
	// subtrees. Legacy Spec migration can normalize render-only table/inline
	// details even when the operation did not visit that subtree.
	baseline, e := p.r.parseOriginalTree(p.ctx, p.w, cachedDocument{raw: raw, path: name}, box, id)
	if e != nil {
		return nil, e
	}
	if _, e = docview.PrepareWriteTree(p.ctx, baseline, func() *ast.Node { return docview.NewParagraph("") }); e != nil {
		return nil, e
	}
	if p.normalized[id], e = docview.EncodeWriteTree(baseline, p.parser, p.singleLine()); e != nil {
		return nil, e
	}
	p.before[id] = raw
	p.tx.Trees[id] = t
	return t, nil
}

func (p *nativeEdit) node(id string) (*ast.Node, *parse.Tree, error) {
	loc, e := p.lookup(id)
	if e != nil || loc == nil {
		return nil, nil, e
	}
	t, e := p.load(loc.BoxID, loc.Path, p.parser)
	if e != nil {
		return nil, nil, e
	}
	return docview.GetNodeInTree(t, id), t, p.ctx.Err()
}

func (p *nativeEdit) stage(t *parse.Tree) error {
	if t == nil || t.Root == nil || !documentID.MatchString(t.ID) {
		return ErrDocument
	}
	p.tx.Trees[t.ID] = t
	p.dirty[t.ID] = true
	db, e := p.bundle.Database(indexdb.BlockTree)
	if e != nil {
		return e
	}
	if e = (indexdb.BlockTrees{DB: db}).Replace(p.ctx, t.ID, indexdb.ProjectBlockTree(t)); e != nil {
		return e
	}
	// The original Markdown index renderer measures tables and writes the
	// widths back into its AST. Keep this disposable projection separate so
	// those render measurements cannot turn untouched source into a write.
	image, e := docview.EncodeWriteTree(t, p.parser, true)
	if e != nil {
		return e
	}
	projected, e := dataparser.ParseJSONWithoutFix(image, p.indexer.ParseOptions)
	if e != nil {
		return e
	}
	if projected == nil || projected.Root == nil {
		return ErrDocument
	}
	projected.ID, projected.Box, projected.Path, projected.HPath = t.ID, t.Box, t.Path, t.HPath
	ast.Walk(projected.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			n.Box, n.Path = t.Box, t.Path
		}
		return ast.WalkContinue
	})
	record, e := indexdb.ProjectTree(p.ctx, projected, p.indexer, indexdb.TreeServices{Content: p.content, LocalDefinitions: true, PrepareOCR: p.r.nativeOCR, IndexAssetPath: p.r.configuration.Search.IndexAssetPath, AssetHash: func(c context.Context, name, box, doc string) (string, error) {
		return originalAssetHash(c, p.w, name, box, doc)
	}}, p.r.nativeDefinition)
	if e != nil {
		return e
	}
	record.IgnoreLines = p.ignoreLines
	return p.bundle.Transaction(p.ctx, indexdb.Main, func(tx *sql.Tx) error {
		_, e := indexdb.WriteTree(p.ctx, tx, record, true, p.r.configuration.Search.CaseSensitive)
		return e
	})
}

func (p *nativeEdit) readFile(name string) ([]byte, error) {
	if f, ok := p.files[name]; ok {
		return slices.Clone(f.after), p.ctx.Err()
	}
	return readNativeFile(p.ctx, p.w, name, 64<<20)
}

func (p *nativeEdit) putFile(name string, after []byte) error {
	f, ok := p.files[name]
	if !ok {
		before, e := readNativeFile(p.ctx, p.w, name, 64<<20)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		f = nativeEditFile{before: before, existed: e == nil}
	}
	f.after = slices.Clone(after)
	p.files[name] = f
	return p.ctx.Err()
}

func (p *nativeEdit) attrs(n *ast.Node, t *parse.Tree, values map[string]string) error {
	if n == nil || t == nil {
		return ErrDocument
	}
	n.KramdownIAL = nil
	for key, value := range values {
		n.SetIALAttr(key, value)
	}
	p.tx.Nodes[n.ID] = n
	return p.stage(t)
}

func (p *nativeEdit) notice(kind, id string, data any) error {
	p.notices = append(p.notices, NativeEditNotice{kind, id, data})
	return p.ctx.Err()
}

func (p *nativeEdit) readAV(id string) (*av.AttributeView, error) {
	if !documentID.MatchString(id) {
		return nil, ErrDocument
	}
	if a := p.avs[id]; a != nil {
		return a, p.ctx.Err()
	}
	raw, e := p.readFile("storage/av/" + id + ".json")
	if e != nil {
		return nil, e
	}
	a, e := decodeNativeAV(raw, id, p.now)
	if e != nil {
		return nil, e
	}
	a.DurationFormatter, e = p.r.configurationBusiness.Duration(p.r.configuration.Lang)
	if e != nil {
		return nil, e
	}
	a.ReadAttributeView = p.readAV
	p.avs[id], p.avBefore[id] = a, raw
	return a, nil
}

func (p *nativeEdit) saveAV(a *av.AttributeView) error {
	if a == nil || !documentID.MatchString(a.ID) {
		return ErrDocument
	}
	if p.avBefore[a.ID] == nil {
		if _, e := p.readAV(a.ID); e != nil {
			return e
		}
	}
	if e := editcore.ValidateUpdatedAttributeView(a); e != nil {
		return e
	}
	if _, e := av.EncodeAttributeView(a, true); e != nil {
		return e
	}
	p.avs[a.ID], p.avDirty[a.ID] = a, true
	return p.ctx.Err()
}

func (p *nativeEdit) text(n *ast.Node) (string, string, error) { return p.avText(n, "") }
func (p *nativeEdit) avText(n *ast.Node, id string) (string, string, error) {
	return editcore.AttributeViewBlockTextWithServices(n, id, func(n *ast.Node) (string, error) {
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
			a, e := p.readAV(n.AttributeViewID)
			if e != nil {
				return "", e
			}
			if a.Name == "" {
				return "Database...", nil
			}
			return a.Name, nil
		}
		if n.Type != ast.NodeDocument && n.IsContainerBlock() {
			n = anchor.FirstLeaf(n)
		}
		text, e := docquery.BlockText(p.ctx, n, nil, false, func(c context.Context, n *ast.Node, exclude []string) (string, error) {
			return indexdb.StaticContent(c, n, exclude, false, false, false, p.content)
		})
		if e == nil && utf8.RuneCountInString(text) > 1024 {
			text = string([]rune(text)[:1024]) + "..."
		}
		return text, e
	}, func() string { return p.r.configurationBusiness.Language(p.r.configuration.Lang, 105, false, 0) })
}

func (p *nativeEdit) names(ids string) (string, error) {
	return editcore.AttributeViewNamesWithServices(ids, func(id string) (string, error) {
		a, e := p.readAV(id)
		if e != nil {
			return "", e
		}
		return a.Name, nil
	}, func() string { return p.r.configurationBusiness.Language(p.r.configuration.Lang, 105, false, 0) }, false)
}

func (p *nativeEdit) failure(op *editcore.Operation, f *editcore.Failure) error {
	if f == nil {
		return nil
	}
	return fmt.Errorf("original_operation_failed: %s %s (%d): %s", op.Action, f.ID, f.Code, f.Msg)
}
