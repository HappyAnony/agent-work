// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/assetflow"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/internal/workspace"
)

type uiArchiveFormat struct{ notebook, markdown, multiple bool }

var uiArchiveExports = map[string]uiArchiveFormat{
	"/api/export/exportSY":         {},
	"/api/export/exportMd":         {markdown: true},
	"/api/export/exportMds":        {markdown: true, multiple: true},
	"/api/export/exportNotebookSY": {notebook: true},
	"/api/export/exportNotebookMd": {notebook: true, markdown: true},
}

func runUIArchiveExport(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView, format uiArchiveFormat) (any, error) {
	var input struct {
		ID, Notebook string
		IDs          []string
	}
	if Decode(bytes.NewReader(r.Data), &input) != nil {
		return nil, ErrArgument
	}
	if format.notebook && format.markdown {
		input.ID = input.Notebook
	}
	if format.multiple {
		if input.ID != "" || input.Notebook != "" || len(input.IDs) == 0 || len(input.IDs) > 1000 ||
			slices.ContainsFunc(input.IDs, func(id string) bool { return !nativeID.MatchString(id) }) {
			return nil, ErrArgument
		}
	} else if !nativeID.MatchString(input.ID) || len(input.IDs) != 0 {
		return nil, ErrArgument
	}
	if view.Mode == "docxSingle" && (format.notebook || format.multiple || input.ID != view.Entry.DocID) {
		return nil, ErrUnauthorized
	}
	var result struct {
		Name string `json:"name"`
		Data []byte `json:"data"`
	}
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		if view.Mode == "docxSingle" {
			tree, err := m.Tree(c, r.StoreID)
			if err != nil {
				return err
			}
			page, ok := tree.Get(r.PageID)
			if !ok || page.DeletedAt != nil || page.EntryDocument == nil || *page.EntryDocument != *view.Entry {
				return ErrStaleSession
			}
		}
		l, e := m.Acquire(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		defer l.Release()
		type selection struct{ box, root string }
		box, root, label := input.ID, "", input.ID
		var targets []selection
		if format.notebook {
			conf, e := l.NativeNotebookConfiguration(c, box)
			if e != nil {
				return e
			}
			label = conf.Name
			targets = append(targets, selection{box, ""})
		} else {
			if _, e = l.PrepareNativeReads(c, false); e != nil {
				return e
			}
			ids := input.IDs
			if !format.multiple {
				ids = []string{input.ID}
			} else {
				label = "Markdown"
			}
			for _, id := range ids {
				loc, e := l.NativeBlockLocation(c, id)
				if e != nil {
					return e
				}
				if loc["rootID"] != id || view.Mode == "docxSingle" && loc["box"] != view.Entry.BoxID {
					return ErrUnauthorized
				}
				targets = append(targets, selection{loc["box"], strings.TrimPrefix(loc["path"], "/")})
			}
			if !format.multiple {
				box, root = targets[0].box, targets[0].root
			}
		}
		prefix := "pages/" + r.PageID + "/data/"
		files, e := m.SourceFiles(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		index := make(map[string]string, len(files))
		var queue []uiArchiveDocument
		for _, file := range files {
			rel := strings.TrimPrefix(file.Path, prefix)
			index[rel] = file.Hash
			fileBox, local, ok := strings.Cut(rel, "/")
			if !ok || !documentRelativePath(local) {
				continue
			}
			// If two selections overlap, keep the outer selection's path.
			best := ""
			matched := false
			for _, target := range targets {
				if fileBox != target.box {
					continue
				}
				// Single-document tickets have no authority to export children.
				if target.root != "" && local != target.root && (view.Mode == "docxSingle" ||
					!strings.HasPrefix(local, strings.TrimSuffix(target.root, ".docj")+"/")) {
					continue
				}
				if !matched || len(target.root) < len(best) {
					best, matched = target.root, true
				}
			}
			if matched {
				relative := local
				if parent := path.Dir(best); parent != "." {
					relative = strings.TrimPrefix(local, parent+"/")
				}
				queue = append(queue, uiArchiveDocument{rel, relative})
			}
		}
		if len(queue) == 0 {
			return fs.ErrNotExist
		}
		b := uiArchiveBuilder{ctx: c, m: m, r: r, lease: l, prefix: prefix, format: format, index: index,
			members: map[string][]byte{}, selected: map[string]bool{}, views: map[string]*av.AttributeView{},
			markdownFolders: map[string]string{".": "."}, desktop: view.Mode == "wikiDesktop"}
		slices.SortFunc(queue, func(a, b uiArchiveDocument) int { return strings.Compare(a.target, b.target) })
		for _, doc := range queue {
			b.selected[doc.source] = true
		}
		if !format.markdown && view.Mode == "wikiDesktop" {
			if _, e = l.PrepareNativeReads(c, false); e != nil {
				return e
			}
		}
		if e = b.addDocuments(queue, box, root, &label, view.Mode == "wikiDesktop"); e != nil {
			return e
		}
		if !format.markdown {
			plan, e := l.PlanNativeFlashcardExport(c, workspace.NativeFlashcardExportRequest{Scope: "documents", DocumentIDs: b.docIDs})
			if e != nil {
				return e
			}
			for name, data := range plan.Files {
				if e := b.add(name, data); e != nil {
					return e
				}
			}
			if e := b.addRanks(box); e != nil {
				return e
			}
		}
		label = textprefs.FilterFileName(label)
		if label == "" || label == "." || label == ".." {
			label = input.ID
		}
		extension := ".docj.zip"
		if format.markdown {
			extension = ".zip"
		}
		result.Name = label + extension
		var out bytes.Buffer
		z := zip.NewWriter(&out)
		for _, name := range slices.Sorted(maps.Keys(b.members)) {
			entry, e := z.Create(path.Join(label, name))
			if e != nil {
				return e
			}
			if e := archivepack.CopyContext(c, entry, bytes.NewReader(b.members[name])); e != nil {
				return e
			}
		}
		if e := z.Close(); e != nil {
			return e
		}
		if out.Len() > 8<<20 {
			return ErrExportLimit
		}
		result.Data = out.Bytes()
		return nil
	})
	return result, err
}

type uiArchiveDocument struct{ source, target string }

type uiArchiveBuilder struct {
	ctx             context.Context
	m               *workspace.WorkspaceManager
	r               UIRequest
	lease           *workspace.Lease
	prefix          string
	format          uiArchiveFormat
	desktop         bool
	index           map[string]string
	members         map[string][]byte
	selected        map[string]bool
	views           map[string]*av.AttributeView
	markdownFolders map[string]string
	docIDs          []string
	size            int
}

func (b *uiArchiveBuilder) add(name string, data []byte) error {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		return ErrArgument
	}
	if old, ok := b.members[name]; ok {
		if !bytes.Equal(old, data) {
			return sourcetx.ErrConflict
		}
		return nil
	}
	if len(b.members) >= 5000 || len(data) > 16<<20 || b.size+len(data) > 48<<20 {
		return ErrExportLimit
	}
	b.size += len(data)
	b.members[name] = data
	return b.ctx.Err()
}

func (b *uiArchiveBuilder) read(name string) ([]byte, error) {
	hash, ok := b.index[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	data, err := b.m.SourceFile(b.ctx, b.r.StoreID, b.prefix+name)
	if err == nil && sourcetx.Hash(data) != hash {
		return nil, sourcetx.ErrConflict
	}
	return data, err
}

func (b *uiArchiveBuilder) addDocuments(queue []uiArchiveDocument, box, root string, label *string, dependencies bool) error {
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{})
	for i := 0; i < len(queue); i++ {
		if i >= 1000 {
			return ErrExportLimit
		}
		doc := queue[i]
		id := strings.TrimSuffix(path.Base(doc.source), ".docj")
		raw, err := b.read(doc.source)
		if err != nil {
			return err
		}
		_, refs, err := (&docast.Parser{}).Parse(b.ctx, raw, id)
		if err != nil {
			return err
		}
		input, err := docast.OriginalTreeInput(raw)
		if err != nil {
			return err
		}
		tree, err := dataparser.ParseJSONWithoutFix(input, parser.ParseOptions)
		if err != nil {
			return err
		}
		if root != "" && doc.source == box+"/"+root && tree.Root.IALAttr("title") != "" {
			*label = tree.Root.IALAttr("title")
		}
		target := doc.target
		if b.format.markdown {
			title := textprefs.FilterFileName(tree.Root.IALAttr("title"))
			if title == "" || title == "." || title == ".." {
				title = id
			}
			folder := path.Dir(target)
			if name, ok := b.markdownFolders[folder]; ok {
				folder = name
			}
			target = path.Join(folder, title+"-"+id+".md")
			b.markdownFolders[strings.TrimSuffix(doc.target, ".docj")] = strings.TrimSuffix(target, ".md")
			value, err := lute.ProtyleExportMdNodeSync(tree.Root, parser.ParseOptions, parser.RenderOptions)
			if err != nil {
				return err
			}
			raw = []byte(value)
		}
		if err := b.add(target, raw); err != nil {
			return err
		}
		b.docIDs = append(b.docIDs, id)
		docBox, _, _ := strings.Cut(doc.source, "/")
		if err := b.addAssets(doc.source, target, docBox, tree.Root); err != nil {
			return err
		}
		if dependencies && !b.format.markdown {
			for _, ref := range refs {
				loc, err := b.lease.NativeBlockLocation(b.ctx, ref.TargetID)
				if errors.Is(err, fs.ErrNotExist) || errors.Is(err, workspace.ErrDocument) {
					continue
				}
				if err != nil {
					return err
				}
				source := loc["box"] + loc["path"]
				if !b.selected[source] {
					b.selected[source] = true
					queue = append(queue, uiArchiveDocument{source, path.Base(source)})
				}
			}
		}
	}
	return nil
}

func (b *uiArchiveBuilder) addAssets(document, target, box string, node *ast.Node) error {
	links, err := assetflow.LinkDests(node, b.loadView)
	if err != nil {
		return err
	}
	for _, link := range links {
		uri, err := url.Parse(link)
		if err != nil || uri.Scheme != "" || uri.Host != "" {
			continue
		}
		name, ok := strings.CutPrefix(uri.Path, "assets/")
		if !ok || !validAsset(name) {
			continue
		}
		// Match the document asset lookup order and take annotations from
		// the same directory as the PDF.
		for _, dir := range []string{".", strings.TrimSuffix(document, ".docj"), path.Dir(document), box} {
			source := path.Join(dir, "assets", name)
			raw, err := b.read(source)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			dest := path.Join("assets", name)
			if b.format.markdown {
				dest = path.Join(path.Dir(target), dest)
			}
			if err = b.add(dest, raw); err != nil {
				return err
			}
			if data, err := b.read(source + ".sya"); err == nil {
				if err = b.add(dest+".sya", data); err != nil {
					return err
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			break
		}
	}
	return nil
}

func (b *uiArchiveBuilder) loadView(id string) (*av.AttributeView, error) {
	if !b.desktop {
		return nil, nil
	}
	if found, ok := b.views[id]; ok {
		return found, nil
	}
	if !nativeID.MatchString(id) {
		return nil, ErrArgument
	}
	name := "storage/av/" + id + ".json"
	raw, err := b.read(name)
	if errors.Is(err, fs.ErrNotExist) {
		b.views[id] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view, err := av.DecodeAttributeView(raw)
	if err != nil || view == nil || view.ID != id {
		return nil, ErrArgument
	}
	if !b.format.markdown {
		if err = b.add(name, raw); err != nil {
			return nil, err
		}
	}
	b.views[id] = view
	return view, nil
}

func (b *uiArchiveBuilder) addRanks(box string) error {
	if !b.desktop {
		return nil
	}
	raw, err := b.read(box + "/.wiki/sort.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var ranks map[string]int
	if err = json.Unmarshal(raw, &ranks); err != nil {
		return err
	}
	selected := make(map[string]int)
	for _, id := range b.docIDs {
		if rank, ok := ranks[id]; ok {
			selected[id] = rank
		}
	}
	if len(selected) == 0 {
		return nil
	}
	raw, err = json.Marshal(selected)
	if err != nil {
		return err
	}
	return b.add(".wiki/sort.json", raw)
}
