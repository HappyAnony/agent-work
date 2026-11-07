// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

func (p *lifecycleEdit) create() error {
	r := p.request
	if r.ExpectedHash != "" {
		return ErrArgument
	}
	l, e := p.lease()
	if e != nil {
		return e
	}
	defer l.Release()
	trees := map[string]*parse.Tree{}
	effects := []workspace.CreationEffect{}
	var avs map[string]*avmodel.AttributeView
	if r.CreationMode == "" {
		folder := "/"
		if r.ParentDocID != "" {
			parent, _, e := p.doc(r.BoxID, r.ParentDocID)
			if e != nil {
				return e
			}
			folder = strings.TrimSuffix(strings.TrimPrefix(parent, r.BoxID), ".docj")
		}
		action := "create"
		if r.DOM != "" {
			action = "create-dom"
		}
		plan, e := l.PlanNativeDocumentLifecycle(p.ctx, doclife.Request{Action: action, Box: r.BoxID, Path: path.Join(folder, r.DocID+".docj"), Title: r.Title, Markdown: r.Markdown, DOM: r.DOM})
		if e != nil {
			return e
		}
		if e = p.checkPlanInputs(plan.SourceHashes); e != nil {
			return e
		}
		trees[plan.Tree.ID] = plan.Tree
		p.result.DocID = plan.Tree.ID
		if action == "create" {
			effects = append(effects, workspace.CreationEffect{Kind: "sort-configured", Box: r.BoxID, Path: path.Dir(plan.Tree.Path), ID: plan.Tree.ID})
		}
	} else {
		if r.CreationMode == "duplicate" {
			if _, _, e = p.doc(r.BoxID, r.DocID); e != nil {
				return e
			}
		}
		if _, e = l.PrepareNativeReads(p.ctx, false); e != nil {
			return e
		}
		plan, e := l.PlanNativeDocumentCreation(p.ctx, doclife.CreationRequest{Mode: r.CreationMode, Box: r.BoxID, ID: r.DocID, HPath: r.HPath, Markdown: r.Markdown, DOM: r.DOM, ParentID: r.ParentDocID, Tags: r.Tags, ClippingHref: r.ClippingHref, WithMath: r.WithMath})
		if e != nil {
			return e
		}
		if e = p.checkPlanInputs(plan.SourceHashes); e != nil {
			return e
		}
		trees, avs, effects = plan.Trees, plan.AttributeViews, plan.Effects
		p.result.DocID = plan.Result.ID
		if len(trees) == 0 && plan.Result.Existed {
			name, raw, e := p.doc(r.BoxID, plan.Result.ID)
			if e != nil {
				return e
			}
			p.result.Path, p.result.Hash = name, sourcetx.Hash(raw)
			return nil
		}
	}
	candidates := map[string][]byte{}
	replaced := map[string]bool{}
	mirrors := map[string][]string{}
	for _, t := range trees {
		name := path.Join(t.Box, t.Path)
		if _, e := docview.PrepareWriteTree(p.ctx, t, func() *ast.Node { return docview.NewParagraph("") }); e != nil {
			return e
		}
		raw, e := docview.EncodeWriteTree(t, nativeparser.NewEditor(nativeparser.EditorOptions{}), true)
		if e != nil {
			return e
		}
		if t.ID == p.result.DocID {
			p.result.Path, p.result.Hash = name, sourcetx.Hash(raw)
		}
		if p.files[name].Hash != "" {
			// The original path/daily creation algorithms can update an existing
			// document. Explicit create always has create-only semantics.
			if r.CreationMode == "" || r.CreationMode == "duplicate" {
				return sourcetx.ErrConflict
			}
			old, e := p.bytes(name)
			if e != nil {
				return e
			}
			raw, e = docast.SerializeDocument([][]byte{old}, raw)
			if e != nil {
				return e
			}
			replaced[name] = true
		} else if r.CreationMode == "duplicate" {
			_, original, e := p.doc(r.BoxID, r.DocID)
			if e != nil {
				return e
			}
			raw, e = docast.SerializeDocumentCopy(p.ctx, original, r.DocID, raw, t.ID)
			if e != nil {
				return e
			}
		}
		if t.ID == p.result.DocID {
			p.result.Hash = sourcetx.Hash(raw)
		}
		candidates[name] = raw
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.Type == ast.NodeAttributeView && n.AttributeViewID != "" {
				mirrors[n.AttributeViewID] = append(mirrors[n.AttributeViewID], n.ID)
			}
			return ast.WalkContinue
		})
	}
	if e = p.validateDocumentIDs(candidates, replaced); e != nil {
		return e
	}
	for n, data := range candidates {
		p.write(n, data)
	}
	for id, a := range avs {
		name := "storage/av/" + id + ".json"
		if p.files[name].Hash != "" {
			return sourcetx.ErrConflict
		}
		raw, e := avmodel.EncodeAttributeView(a, true)
		if e != nil {
			return e
		}
		p.write(name, raw)
	}
	if e = p.updateMirrors(mirrors, nil); e != nil {
		return e
	}
	for _, effect := range effects {
		switch effect.Kind {
		case "sort-after":
			if e = p.sortCreation(l, effect.Box, path.Dir(effect.Path), effect.ID, effect.Path); e != nil {
				return e
			}
		case "sort-configured":
			if e = p.sortCreation(l, effect.Box, effect.Path, effect.ID, ""); e != nil {
				return e
			}
		}
	}
	return nil
}

func (p *lifecycleEdit) sortCreation(l *workspace.Lease, box, folder, added, previous string) error {
	cfg, e := l.Configuration(p.ctx)
	if e != nil {
		return e
	}
	if cfg.FileTree == nil || cfg.FileTree.CreateDocAtTop == nil {
		return ErrArgument
	}
	top := *cfg.FileTree.CreateDocAtTop
	var docs []*doctree.File
	if _, exists := p.tx.ReadGuards.Directories[p.prefix+path.Join(box, folder)]; exists {
		max := 102400
		if previous != "" {
			max = cfg.FileTree.MaxListCount
		} else if top {
			max = 1
		}
		docs, _, e = l.NativeListDocumentTree(p.ctx, doctree.Request{Box: box, Path: folder, Sort: configtypes.SortModeUnassigned, Max: max}, nil)
		if e != nil {
			return e
		}
	}
	if previous == "" {
		return p.mergeRanks(l, box, map[string]int{added: doctree.InsertionRank(docs, top)})
	}
	existing := map[string]int{}
	metadata := box + "/.wiki/sort.json"
	if p.files[metadata].Hash != "" {
		raw, e := p.bytes(metadata)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &existing); e != nil || existing == nil {
			return ErrArgument
		}
	}
	full, _ := doctree.InsertSort(existing, docs, strings.TrimSuffix(path.Base(previous), ".docj"), added)
	return p.mergeRanks(l, box, full)
}

func (p *lifecycleEdit) updateMirrors(add map[string][]string, remove map[string]bool) error {
	if len(add)+len(remove) == 0 {
		return nil
	}
	const name = "storage/av/blocks.msgpack"
	registry := map[string][]string{}
	if p.files[name].Hash != "" {
		raw, e := p.bytes(name)
		if e != nil {
			return e
		}
		if e = msgpack.Unmarshal(raw, &registry); e != nil {
			return e
		}
		if registry == nil {
			return ErrArgument
		}
	}
	changed := false
	for id, blocks := range registry {
		keep := slices.DeleteFunc(slices.Clone(blocks), func(id string) bool { return remove[id] })
		if len(keep) != len(blocks) {
			registry[id] = keep
			changed = true
		}
	}
	for id, blocks := range add {
		if !nativeID.MatchString(id) {
			return ErrArgument
		}
		for _, block := range blocks {
			if !slices.Contains(registry[id], block) {
				registry[id] = append(registry[id], block)
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	var b bytes.Buffer
	if e := msgpack.NewEncoder(&b).SetSortMapKeys(true).Encode(registry); e != nil {
		return e
	}
	p.write(name, b.Bytes())
	return nil
}

func (p *lifecycleEdit) renameDocument(name string, raw []byte) error {
	r := p.request
	l, e := p.lease()
	if e != nil {
		return e
	}
	defer l.Release()
	plan, e := l.PlanNativeDocumentLifecycle(p.ctx, doclife.Request{Action: "rename", Box: r.BoxID, Path: strings.TrimPrefix(name, r.BoxID), Title: r.Title})
	if e != nil {
		return e
	}
	p.result.DocID, p.result.Path, p.result.Hash = r.DocID, name, sourcetx.Hash(raw)
	if e = p.checkPlanInputs(plan.SourceHashes); e != nil {
		return e
	}
	if plan.Noop {
		return p.updateEntryTitle(raw)
	}
	root := plan.Tree.Root
	next, e := rewriteRootProperties(raw, map[string]string{"title": root.IALAttr("title"), "updated": root.IALAttr("updated")}, nil)
	if e != nil {
		return e
	}
	if e = p.updateEntryTitle(next); e != nil {
		return e
	}
	if e = p.saveVersion(raw); e != nil {
		return e
	}
	p.write(name, next)
	p.result.Hash = sourcetx.Hash(next)
	return nil
}

func rewriteRootProperties(raw []byte, set map[string]string, remove []string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if e := json.Unmarshal(raw, &doc); e != nil {
		return nil, e
	}
	props := map[string]json.RawMessage{}
	if data := doc["Properties"]; len(data) > 0 {
		if e := json.Unmarshal(data, &props); e != nil {
			return nil, e
		}
		if props == nil {
			props = map[string]json.RawMessage{}
		}
	}
	changed := false
	for k, v := range set {
		next, _ := json.Marshal(v)
		if !bytes.Equal(props[k], next) {
			props[k] = next
			changed = true
		}
	}
	for _, k := range remove {
		if _, ok := props[k]; ok {
			delete(props, k)
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	doc["Properties"], _ = json.Marshal(props)
	return json.Marshal(doc)
}

func (p *lifecycleEdit) updateEntryTitle(data []byte) error {
	r := p.request
	if p.page.Type != "docx" || !p.isEntry(r.BoxID, r.DocID) {
		if r.ExpectedPageHash != "" {
			return ErrArgument
		}
		return nil
	}
	var doc struct{ Properties map[string]json.RawMessage }
	if e := json.Unmarshal(data, &doc); e != nil {
		return e
	}
	var title string
	if e := json.Unmarshal(doc.Properties["title"], &title); e != nil {
		return e
	}
	if title == p.page.Name {
		return nil
	}
	p.page.Name = title
	return p.writePage()
}

func (p *lifecycleEdit) writePage() error {
	r := p.request
	name := "pages/" + r.PageID + "/page.json"
	raw, e := p.read(name)
	if e != nil {
		return e
	}
	if r.ExpectedPageHash == "" || sourcetx.Hash(raw) != r.ExpectedPageHash {
		return sourcetx.ErrConflict
	}
	p.page.Revision++
	p.page.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	after, e := json.Marshal(p.page)
	if e != nil {
		return e
	}
	p.change(name, r.ExpectedPageHash, after, false)
	return nil
}

func (p *lifecycleEdit) moveDocument(name string, raw []byte) error {
	r := p.request
	p.result.DocID, p.result.Path, p.result.Hash = r.DocID, name, sourcetx.Hash(raw)
	toBox := r.ToBoxID
	if toBox == "" {
		toBox = r.BoxID
	}
	toPath := "/"
	if r.ParentDocID != "" {
		parent, _, err := p.doc(toBox, r.ParentDocID)
		if err != nil {
			return err
		}
		toPath = strings.TrimPrefix(parent, toBox)
	}
	return p.moveDocuments([]string{name}, toBox, toPath)
}

func (p *lifecycleEdit) moveDocuments(sources []string, toBox, toPath string) error {
	l, err := p.lease()
	if err != nil {
		return err
	}
	defer l.Release()
	paths := make([]string, 0, len(sources))
	for _, name := range sources {
		box, rel, _ := strings.Cut(name, "/")
		if toBox == box && (toPath == "/"+rel || strings.HasPrefix(toPath, "/"+strings.TrimSuffix(rel, ".docj")+"/")) {
			return ErrArgument
		}
		paths = append(paths, name)
	}
	planned, err := l.PlanNativeDocumentLifecycle(p.ctx, doclife.Request{Action: "move", Paths: paths, ToBox: toBox, ToPath: toPath})
	if err != nil {
		return err
	}
	if err = p.checkPlanInputs(planned.SourceHashes); err != nil || planned.Noop {
		return err
	}
	targets := map[string]string{}
	sourceBoxes := map[string][]string{}
	for _, change := range planned.Changes {
		targets[path.Join(change.From.Box, change.From.Path)] = path.Join(change.Tree.Box, change.Tree.Path)
		if change.From.Box != toBox {
			sourceBoxes[change.From.Box] = append(sourceBoxes[change.From.Box], change.Tree.ID)
		}
	}
	for _, name := range planned.Request.Paths {
		dest, ok := targets[name]
		if !ok || dest == name {
			continue
		}
		prefix := strings.TrimSuffix(name, ".docj") + "/"
		for _, file := range p.subtree(name) {
			target, ok := targets[file]
			if !ok {
				if strings.HasSuffix(file, ".docj") {
					return sourcetx.ErrConflict
				}
				target = strings.TrimSuffix(dest, ".docj") + "/" + strings.TrimPrefix(file, prefix)
			}
			if p.files[target].Hash != "" {
				return sourcetx.ErrConflict
			}
			data, err := p.bytes(file)
			if err != nil {
				return err
			}
			if file == name {
				data, err = rewriteRootProperties(data, nil, []string{"custom-hidden"})
				if err != nil {
					return err
				}
				if p.result.Path == name {
					p.result.Path, p.result.Hash = dest, sourcetx.Hash(data)
				}
			}
			p.change(p.prefix+file, p.files[file].Hash, nil, true)
			p.change(p.prefix+target, "", data, false)
		}
	}
	ranks := map[string]int{}
	for box, ids := range sourceBoxes {
		moved, err := p.removeRanks(l, box, ids)
		if err != nil {
			return err
		}
		for id, rank := range moved {
			ranks[id] = rank
		}
		if p.page.EntryDocument != nil && p.page.EntryDocument.BoxID == box && slices.Contains(ids, p.page.EntryDocument.DocID) {
			p.page.EntryDocument.BoxID = toBox
			if err = p.writePage(); err != nil {
				return err
			}
		}
	}
	if len(sourceBoxes) > 0 {
		return p.mergeRanks(l, toBox, ranks)
	}
	return nil
}

func (p *lifecycleEdit) reorder() error {
	r := p.request
	ordered := make([]string, 0, len(r.Order))
	seen := map[string]bool{}
	folder := ""
	for _, id := range r.Order {
		if !nativeID.MatchString(id) || seen[id] {
			return ErrArgument
		}
		seen[id] = true
		name, _, e := p.doc(r.BoxID, id)
		if e != nil {
			return e
		}
		if folder != "" && folder != path.Dir(name) {
			return ErrArgument
		}
		folder = path.Dir(name)
		ordered = append(ordered, strings.TrimPrefix(name, r.BoxID))
	}
	l, e := p.lease()
	if e != nil {
		return e
	}
	defer l.Release()
	plan, e := l.PlanNativeDocumentSort(p.ctx, doctree.SortMutation{Mode: "reorder", Box: r.BoxID, Paths: ordered})
	if e != nil {
		return e
	}
	if p.files[plan.Name].Hash != r.ExpectedHash {
		return sourcetx.ErrConflict
	}
	if e = p.checkSortPreimage(plan); e != nil {
		return e
	}
	if !bytes.Equal(plan.Before, plan.After) {
		p.write(plan.Name, plan.After)
	}
	p.result.Hash = sourcetx.Hash(plan.After)
	return nil
}

func (p *lifecycleEdit) asset() error {
	r := p.request
	name := "assets/" + r.AssetName
	raw, e := p.bytes(name)
	if e != nil {
		return e
	}
	if sourcetx.Hash(raw) != r.ExpectedHash {
		return sourcetx.ErrConflict
	}
	l, e := p.lease()
	if e != nil {
		return e
	}
	defer l.Release()
	if r.Action == "asset.delete" {
		unused, e := l.NativeAssetReferences(p.ctx, false)
		if e != nil {
			return e
		}
		if !slices.Contains(unused, name) {
			return sourcetx.ErrConflict
		}
		if e = p.checkRetainedAsset(name); e != nil {
			return e
		}
		p.change(p.prefix+name, r.ExpectedHash, nil, true)
		return nil
	}
	plan, e := l.PlanNativeAssetRename(p.ctx, name, r.NewAssetName)
	if e != nil {
		return e
	}
	if plan.CopyTo == "" {
		return nil
	}
	if plan.CopyFrom != name || p.files[plan.CopyTo].Hash != "" || sourcetx.Hash(plan.CopyData) != r.ExpectedHash {
		return sourcetx.ErrConflict
	}
	p.change(p.prefix+plan.CopyTo, "", plan.CopyData, false)
	if plan.AnnotationPresent {
		if p.files[plan.CopyTo+".sya"].Hash != "" {
			return sourcetx.ErrConflict
		}
		p.change(p.prefix+plan.CopyTo+".sya", "", plan.Annotation, false)
	}
	for _, c := range plan.Changes {
		if p.files[c.Name].Hash != sourcetx.Hash(c.Before) {
			return sourcetx.ErrConflict
		}
		p.write(c.Name, c.After)
	}
	p.result.AssetName, p.result.Hash = strings.TrimPrefix(plan.CopyTo, "assets/"), sourcetx.Hash(plan.CopyData)
	return nil
}

func (p *lifecycleEdit) checkRetainedAsset(name string) error {
	records, e := p.guardArchive("tombstones")
	if e != nil {
		return e
	}
	for _, record := range records {
		raw, e := p.read(record.Path)
		if e != nil {
			return e
		}
		if sourcetx.Hash(raw) != record.Hash {
			return sourcetx.ErrConflict
		}
		t, e := sourcetx.DecodeDocumentTombstone(raw, record.Path)
		if e != nil {
			return e
		}
		for _, f := range t.Files {
			if !activeDoc(f.Path) {
				continue
			}
			if e = p.guardRetainedAsset(t.ArchivePath(f.Path), f.Hash, name); e != nil {
				return e
			}
		}
		for _, f := range t.Effects {
			if f.Path == "storage/av/blocks.msgpack" {
				continue
			}
			if e = p.guardRetainedAsset(t.EffectPath(f.Path), f.BeforeHash, name); e != nil {
				return e
			}
		}
	}
	notebooks, e := p.guardArchive("notebooks")
	if e != nil {
		return e
	}
	for _, record := range notebooks {
		raw, e := p.read(record.Path)
		if e != nil || sourcetx.Hash(raw) != record.Hash {
			return sourcetx.ErrConflict
		}
		archive, e := sourcetx.DecodeNotebookArchive(raw, record.Path)
		if e != nil {
			return e
		}
		for _, f := range archive.Files {
			if activeDoc(archive.BoxID + "/" + f.Path) {
				if e := p.guardRetainedAsset(archive.ArchivePath(f.Path), f.Hash, name); e != nil {
					return e
				}
			}
		}
		for _, f := range archive.Effects {
			if f.Path != "storage/av/blocks.msgpack" {
				if e := p.guardRetainedAsset(archive.EffectPath(f.Path), f.BeforeHash, name); e != nil {
					return e
				}
			}
		}
	}
	return p.checkVersionAsset(name)
}

func (p *lifecycleEdit) guardRetainedAsset(file, hash, asset string) error {
	raw, e := p.read(file)
	if e != nil {
		return e
	}
	if sourcetx.Hash(raw) != hash || retainedAssetReference(raw, asset) {
		return sourcetx.ErrConflict
	}
	p.tx.ReadGuards.Files[file] = hash
	return nil
}

func (p *lifecycleEdit) deleteEffects(l *workspace.Lease, ids, removed, allRemoved []string) ([]sourcetx.Change, error) {
	// Most documents have no database effects.
	for _, file := range removed {
		if !activeDoc(file) {
			continue
		}
		raw, err := p.bytes(file)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(raw, []byte("NodeAttributeView")) && !bytes.Contains(raw, []byte("custom-avs")) {
			continue
		}
		plan, err := l.PlanNativeDocumentDeleteEffects(p.ctx, ids)
		if err != nil {
			return nil, err
		}
		return slices.DeleteFunc(plan.Changes, func(c sourcetx.Change) bool {
			return slices.Contains(allRemoved, strings.TrimPrefix(c.Path, p.prefix))
		}), nil
	}
	return nil, nil
}

func (p *lifecycleEdit) stageDeleteEffects(effects *[]sourcetx.DeletedEffect, archivePath func(string) string, changes []sourcetx.Change, seen map[string]bool) error {
	for _, c := range changes {
		rel := strings.TrimPrefix(c.Path, p.prefix)
		if seen[rel] || c.Delete || c.ExpectedHash == "" || c.ExpectedHash != p.files[rel].Hash {
			return sourcetx.ErrConflict
		}
		seen[rel] = true
		raw, err := p.bytes(rel)
		if err != nil {
			return err
		}
		effect := sourcetx.DeletedEffect{Path: rel, BeforeHash: c.ExpectedHash, AfterHash: sourcetx.Hash(c.Data)}
		*effects = append(*effects, effect)
		p.change(archivePath(rel), "", raw, false)
		p.tx.Changes = append(p.tx.Changes, c)
	}
	slices.SortFunc(*effects, func(a, b sourcetx.DeletedEffect) int { return strings.Compare(a.Path, b.Path) })
	return nil
}

func (p *lifecycleEdit) keepDocumentAssets(t sourcetx.DocumentTombstone, copied map[string]string) error {
	for _, f := range t.Files {
		_, name, ok := strings.Cut(f.Path, "/assets/")
		if !ok || !validAsset(name) {
			continue
		}
		target := "assets/" + name
		existing := p.files[target].Hash
		if existing == "" {
			existing = copied[target]
		}
		if existing != "" {
			if existing != f.Hash {
				return sourcetx.ErrConflict
			}
			continue
		}
		raw, e := p.bytes(f.Path)
		if e != nil {
			return e
		}
		copied[target] = f.Hash
		p.change(p.prefix+target, "", raw, false)
	}
	return nil
}

func (p *lifecycleEdit) readVersion() ([]byte, error) {
	r := p.request
	var raw []byte
	var e error
	if r.LegacyHistoryPath == "" {
		raw, e = p.read(p.versionPath(r.VersionID))
	} else {
		l, leaseErr := p.lease()
		if leaseErr != nil {
			return nil, leaseErr
		}
		defer l.Release()
		raw, e = l.NativeDocumentVersion(p.ctx, r.BoxID, r.DocID, r.LegacyHistoryPath)
	}
	if e == nil && r.VersionHash != "" && sourcetx.Hash(raw) != r.VersionHash {
		return nil, sourcetx.ErrConflict
	}
	return raw, e
}
func (p *lifecycleEdit) applyVersion() error {
	r := p.request
	name, raw, e := p.doc(r.BoxID, r.DocID)
	exists := e == nil
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	if exists {
		if r.ParentDocID != "" || sourcetx.Hash(raw) != r.ExpectedHash {
			return sourcetx.ErrConflict
		}
	} else {
		if r.ExpectedHash != "" {
			return sourcetx.ErrConflict
		}
		folder := r.BoxID
		if r.ParentDocID != "" {
			parent, _, e := p.doc(r.BoxID, r.ParentDocID)
			if e != nil {
				return e
			}
			folder = strings.TrimSuffix(parent, ".docj")
		}
		if _, ok := p.tx.ReadGuards.Directories[p.prefix+r.BoxID]; !ok {
			return sourcetx.ErrConflict
		}
		name = path.Join(folder, r.DocID+".docj")
		if _, e = p.read(p.tombstonePath()); e == nil {
			return sourcetx.ErrConflict
		} else if !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		p.tx.ReadGuards.Missing = append(p.tx.ReadGuards.Missing, p.tombstonePath())
	}
	data, e := p.readVersion()
	if e != nil {
		return e
	}
	if e = p.validateDocumentIDs(map[string][]byte{name: data}, map[string]bool{name: exists}); e != nil {
		return e
	}
	if e = p.updateEntryTitle(data); e != nil {
		return e
	}
	if exists {
		if e = p.saveVersion(raw); e != nil {
			return e
		}
	}
	if e = p.applyVersionAV(raw, data); e != nil {
		return e
	}
	p.write(name, data)
	p.result.DocID, p.result.Path, p.result.Hash = r.DocID, name, sourcetx.Hash(data)
	return nil
}

func documentMirrors(raw []byte) (map[string][]string, error) {
	result := map[string][]string{}
	if len(raw) == 0 {
		return result, nil
	}
	var walk func([]byte) error
	walk = func(raw []byte) error {
		var node struct {
			Type, ID, AttributeViewID string
			Children                  []json.RawMessage
		}
		if e := json.Unmarshal(raw, &node); e != nil {
			return e
		}
		if node.Type == "NodeAttributeView" {
			if !nativeID.MatchString(node.ID) || !nativeID.MatchString(node.AttributeViewID) {
				return ErrArgument
			}
			result[node.AttributeViewID] = append(result[node.AttributeViewID], node.ID)
		}
		for _, c := range node.Children {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	return result, walk(raw)
}

func (p *lifecycleEdit) versionAVSources(mirrors map[string][]string) (map[string][]byte, error) {
	r := p.request
	if r.LegacyHistoryPath != "" {
		l, e := p.lease()
		if e != nil {
			return nil, e
		}
		defer l.Release()
		ids := make([]string, 0, len(mirrors))
		for id := range mirrors {
			ids = append(ids, id)
		}
		return l.NativeDocumentVersionAVs(p.ctx, r.BoxID, r.DocID, r.LegacyHistoryPath, ids)
	}
	all, e := p.m.ArchiveFiles(p.ctx, r.StoreID, "versions", r.PageID, r.BoxID, r.DocID)
	if e != nil {
		return nil, e
	}
	prefix := path.Dir(p.versionPath(r.VersionID)) + "/av/"
	result := map[string][]byte{}
	for _, f := range all {
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		id := strings.TrimSuffix(path.Base(f.Path), ".json")
		if len(mirrors[id]) == 0 {
			return nil, ErrArgument
		}
		raw, e := p.read(f.Path)
		if e != nil {
			return nil, e
		}
		var av struct {
			ID string `json:"id"`
		}
		if sourcetx.Hash(raw) != f.Hash || json.Unmarshal(raw, &av) != nil || av.ID != id {
			return nil, sourcetx.ErrConflict
		}
		result[id] = raw
	}
	return result, nil
}

func (p *lifecycleEdit) applyVersionAV(before, after []byte) error {
	old, e := documentMirrors(before)
	if e != nil {
		return e
	}
	next, e := documentMirrors(after)
	if e != nil {
		return e
	}
	saved, e := p.versionAVSources(next)
	if e != nil {
		return e
	}
	for id := range next {
		name := "storage/av/" + id + ".json"
		h := p.files[name].Hash
		raw, ok := saved[id]
		if !ok {
			if h == "" {
				return sourcetx.ErrConflict
			}
			continue
		}
		if h == sourcetx.Hash(raw) {
			continue
		}
		if h != "" && p.request.ExpectedAVHashes[id] != h {
			return sourcetx.ErrConflict
		}
		p.write(name, raw)
	}
	remove := map[string]bool{}
	for _, blocks := range old {
		for _, id := range blocks {
			remove[id] = true
		}
	}
	return p.updateMirrors(next, remove)
}
