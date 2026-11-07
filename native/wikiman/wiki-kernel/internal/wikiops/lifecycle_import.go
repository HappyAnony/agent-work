// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/internal/workspace"
)

type importDocument struct {
	name, originalID string
	raw              []byte
	tree             *parse.Tree
}

func (p *lifecycleEdit) importArchive() error {
	r := p.request
	files, e := archivepack.ReadDocumentArchive(p.ctx, r.Data)
	if e != nil {
		return errors.Join(ErrArgument, e)
	}
	l, e := p.lease()
	if e != nil {
		return e
	}
	defer l.Release()
	folder := r.BoxID
	if r.ParentDocID != "" {
		name, _, e := p.doc(r.BoxID, r.ParentDocID)
		if e != nil {
			return e
		}
		folder = strings.TrimSuffix(name, ".docj")
	}
	engine := nativeparser.NewEditor(nativeparser.EditorOptions{})
	ids, roots := map[string]string{}, map[string]string{}
	var documents []importDocument
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if !documentRelativePath(name) {
			continue
		}
		old := strings.TrimSuffix(path.Base(name), ".docj")
		raw := files[name]
		if _, _, e = (&docast.Parser{}).Parse(p.ctx, raw, old); e != nil {
			return e
		}
		input, e := docast.OriginalTreeInput(raw)
		if e != nil {
			return e
		}
		tree, _, e := dataparser.ParseJSON(input, engine.ParseOptions)
		if e != nil {
			return e
		}
		if e = archivepack.RemapDocumentIDs(tree, ids, func(id string) string {
			return id[:14] + "-" + strings.ToLower(rand.Text()[:7])
		}); e != nil {
			return errors.Join(ErrArgument, e)
		}
		roots[strings.TrimSuffix(name, ".docj")] = tree.ID
		documents = append(documents, importDocument{name: name, originalID: old, raw: raw, tree: tree})
	}
	if len(documents) == 0 || len(documents) > 1000 {
		return ErrArgument
	}
	views, avIDs, e := p.importArchiveViews(files, ids)
	if e != nil {
		return e
	}
	replacer := archivepack.IDReplacer(ids)
	avReplacer := archivepack.IDReplacer(avIDs)
	candidates := map[string][]byte{}
	external := map[string]bool{}
	trees := map[string]*parse.Tree{}
	for _, doc := range documents {
		tree := doc.tree
		target, e := archiveMountPath(strings.TrimSuffix(doc.name, ".docj"), roots)
		if e != nil {
			return e
		}
		tree.Box, tree.Path = r.BoxID, "/"+strings.TrimPrefix(path.Join(folder, target+".docj"), r.BoxID+"/")
		archivepack.RemapDocumentReferences(tree, ids, replacer)
		if e = remapArchiveDocumentAVs(tree, avIDs, avReplacer); e != nil {
			return e
		}
		name := path.Join(tree.Box, tree.Path)
		if trees[name] != nil || p.files[name].Hash != "" {
			return sourcetx.ErrConflict
		}
		trees[name] = tree
	}
	if e = p.bindArchiveViews(views, trees, external); e != nil {
		return e
	}
	riff := map[string][]byte{}
	for _, suffix := range []string{".deck", ".cards"} {
		name := "storage/riff/" + flashcard.BuiltinDeckID + suffix
		if raw, ok := files[name]; ok {
			riff[name] = raw
		}
	}
	cards := workspace.NativeFlashcardImportRequest{Files: riff, BlockIDs: ids}
	for _, doc := range documents {
		tree := doc.tree
		raw, e := docview.EncodeWriteTree(tree, engine, true)
		if e != nil {
			return e
		}
		raw, e = docast.SerializeDocumentCopy(p.ctx, doc.raw, doc.originalID, raw, tree.ID)
		if e != nil {
			return e
		}
		name := path.Join(tree.Box, tree.Path)
		candidates[name] = raw
		cards.Documents = append(cards.Documents, workspace.NativeFlashcardImportDocument{Box: tree.Box, Path: tree.Path, Data: raw})
	}
	plan, e := l.PlanNativeFlashcardImport(p.ctx, cards)
	if e != nil {
		return e
	}
	for _, t := range plan.Trees {
		name := path.Join(t.Box, t.Path)
		after, e := docview.EncodeWriteTree(t, engine, true)
		if e != nil {
			return e
		}
		candidates[name], e = docast.SerializeDocument([][]byte{candidates[name]}, after)
		if e != nil {
			return e
		}
	}
	for _, c := range plan.Changes {
		before := ""
		if c.Existed {
			before = sourcetx.Hash(c.Before)
		}
		if p.files[c.Path].Hash != before {
			return sourcetx.ErrConflict
		}
		p.write(c.Path, c.After)
	}
	for name := range external {
		before, e := p.bytes(name)
		if e != nil {
			return e
		}
		after, e := docview.EncodeWriteTree(trees[name], engine, true)
		if e != nil {
			return e
		}
		candidates[name], e = docast.SerializeDocument([][]byte{before}, after)
		if e != nil {
			return e
		}
	}
	if e = p.validateDocumentIDs(candidates, external); e != nil {
		return e
	}
	mirrors := map[string][]string{}
	for name, raw := range candidates {
		if !external[name] {
			if p.files[name].Hash != "" {
				return sourcetx.ErrConflict
			}
			refs, e := documentMirrors(raw)
			if e != nil {
				return e
			}
			for id, blocks := range refs {
				mirrors[id] = append(mirrors[id], blocks...)
			}
		}
		p.write(name, raw)
	}
	if e = p.updateMirrors(mirrors, nil); e != nil {
		return e
	}
	if e = p.importArchiveResources(files, folder, roots); e != nil {
		return e
	}
	ranks := map[string]int{}
	for _, name := range []string{".siyuan/sort.json", ".wiki/sort.json"} {
		raw, ok := files[name]
		if !ok {
			continue
		}
		var old map[string]int
		if json.Unmarshal(raw, &old) != nil || old == nil {
			return ErrArgument
		}
		for id, rank := range old {
			if newID := ids[id]; newID != "" {
				ranks[newID] = rank
			}
		}
	}
	if e = p.mergeRanks(l, r.BoxID, ranks); e != nil {
		return e
	}
	first := documents[0].tree
	p.result.DocID, p.result.Path = first.ID, path.Join(first.Box, first.Path)
	p.result.Hash = sourcetx.Hash(candidates[p.result.Path])
	return nil
}

func documentRelativePath(name string) bool {
	if !strings.HasSuffix(name, ".docj") {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, ".docj"), "/") {
		if !nativeID.MatchString(part) {
			return false
		}
	}
	return true
}

// A directory maps to an imported document at each level. A similarly named
// directory outside that hierarchy never becomes an implicit parent.
func archiveMountPath(name string, roots map[string]string) (string, error) {
	parts := strings.Split(name, "/")
	mapped := make([]string, len(parts))
	for i := range parts {
		mapped[i] = roots[strings.Join(parts[:i+1], "/")]
		if mapped[i] == "" {
			return "", ErrArgument
		}
	}
	return strings.Join(mapped, "/"), nil
}

func (p *lifecycleEdit) importArchiveResources(files map[string][]byte, folder string, roots map[string]string) error {
	staged := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if documentRelativePath(name) {
			continue
		}
		target := name
		first, _, _ := strings.Cut(name, "/")
		switch first {
		case "assets":
		case "emojis":
			target = textprefs.FilterUploadEmojiFileName(name)
		case "storage", ".siyuan", ".wiki":
			continue
		default:
			ancestor := path.Dir(name)
			for ancestor != "." && roots[ancestor] == "" {
				ancestor = path.Dir(ancestor)
			}
			if ancestor == "." {
				return ErrArgument
			}
			mapped, e := archiveMountPath(ancestor, roots)
			if e != nil {
				return e
			}
			target = path.Join(folder, mapped, strings.TrimPrefix(name, ancestor+"/"))
		}
		if !fs.ValidPath(target) || strings.HasSuffix(target, ".docj") {
			return ErrArgument
		}
		raw := files[name]
		h := sourcetx.Hash(raw)
		if other := staged[target]; other != "" {
			if other != h {
				return sourcetx.ErrConflict
			}
			continue
		}
		if current := p.files[target].Hash; current != "" {
			if current != h {
				return sourcetx.ErrConflict
			}
			continue
		}
		staged[target] = h
		p.write(target, raw)
	}
	return nil
}

func remapArchiveDocumentAVs(tree *parse.Tree, ids map[string]string, replace *strings.Replacer) error {
	var err error
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter || n.ID == "" {
			return ast.WalkContinue
		}
		for k, v := range parse.IAL2Map(n.KramdownIAL) {
			if strings.HasPrefix(k, avmodel.NodeAttrNameAvs) {
				n.RemoveIALAttr(k)
				if len(ids) > 0 {
					n.SetIALAttr(replace.Replace(k), replace.Replace(v))
				}
			}
		}
		if n.Type == ast.NodeAttributeView {
			n.AttributeViewID = ids[n.AttributeViewID]
			if n.AttributeViewID == "" {
				err = ErrArgument
				return ast.WalkStop
			}
		}
		return ast.WalkContinue
	})
	return err
}

func (p *lifecycleEdit) importArchiveViews(files map[string][]byte, blocks map[string]string) (map[string]*avmodel.AttributeView, map[string]string, error) {
	ids := map[string]string{}
	for name, raw := range files {
		if path.Dir(name) != "storage/av" || !strings.HasSuffix(name, ".json") {
			continue
		}
		old := strings.TrimSuffix(path.Base(name), ".json")
		view, e := avmodel.DecodeAttributeView(raw)
		if !nativeID.MatchString(old) || e != nil || view == nil || view.ID != old {
			return nil, nil, ErrArgument
		}
		ids[old] = ast.NewNodeID()
	}
	avReplace, blockReplace := archivepack.IDReplacer(ids), archivepack.IDReplacer(blocks)
	views := map[string]*avmodel.AttributeView{}
	for _, old := range slices.Sorted(maps.Keys(ids)) {
		raw := files["storage/av/"+old+".json"]
		raw = []byte(blockReplace.Replace(avReplace.Replace(string(raw))))
		id := ids[old]
		name := "storage/av/" + id + ".json"
		if p.files[name].Hash != "" || views[id] != nil {
			return nil, nil, sourcetx.ErrConflict
		}
		view, e := avmodel.DecodeAttributeView(raw)
		if e != nil || view.ID != id || editcore.ValidateCellSource(view) != nil || view.GetBlockKeyValues() == nil {
			return nil, nil, ErrArgument
		}
		// The legacy relation adapter can read earlier JSON that today's
		// native AV decoder rejects. Persist its compatible shape and keep
		// forward-compatible fields from the original archive.
		var current avmodel.AttributeView
		if json.Unmarshal(raw, &current) != nil {
			normalized, e := json.Marshal(view)
			if e != nil {
				return nil, nil, e
			}
			raw, e = docast.SerializeAttributeView(raw, normalized)
			if e != nil {
				return nil, nil, e
			}
		}
		views[id] = view
		p.write(name, raw)
	}
	const relations = "storage/av/relations.msgpack"
	var staged []byte
	for _, id := range slices.Sorted(maps.Keys(views)) {
		dest := ""
		for _, kv := range views[id].KeyValues {
			if kv.Key.Type == avmodel.KeyTypeRelation && kv.Key.Relation != nil {
				dest = kv.Key.Relation.AvID
			}
		}
		if dest == "" {
			continue
		}
		e := avmodel.MutateRelations(p.ctx, id, dest, true, func() ([]byte, error) {
			if staged != nil {
				return staged, nil
			}
			if p.files[relations].Hash == "" {
				return nil, fs.ErrNotExist
			}
			return p.bytes(relations)
		}, func(_, next []byte) error {
			staged = next
			return nil
		})
		if e != nil {
			return nil, nil, e
		}
	}
	if staged != nil {
		p.write(relations, staged)
	}
	return views, ids, nil
}

func (p *lifecycleEdit) bindArchiveViews(views map[string]*avmodel.AttributeView, trees map[string]*parse.Tree, external map[string]bool) error {
	members := map[string][]string{}
	for _, id := range slices.Sorted(maps.Keys(views)) {
		for _, value := range views[id].GetBlockKeyValues().Values {
			if !value.IsDetached {
				members[value.BlockID] = append(members[value.BlockID], id)
			}
		}
	}
	if len(members) == 0 {
		return nil
	}
	apply := func(tree *parse.Tree) error {
		var err error
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter {
				return ast.WalkContinue
			}
			for _, id := range members[n.ID] {
				attrs, e := editcore.CellBindingAttrs(id, n, true, func(string) (string, error) { return "", p.ctx.Err() })
				if e != nil {
					err = e
					return ast.WalkStop
				}
				n.SetIALAttr(avmodel.NodeAttrNameAvs, attrs[avmodel.NodeAttrNameAvs])
			}
			return ast.WalkContinue
		})
		return err
	}
	for _, tree := range trees {
		if e := apply(tree); e != nil {
			return e
		}
	}
	engine := nativeparser.NewEditor(nativeparser.EditorOptions{})
	for name := range p.files {
		if !activeDoc(name) {
			continue
		}
		raw, e := p.bytes(name)
		if e != nil {
			return e
		}
		spans, _, e := (&docast.Parser{}).Parse(p.ctx, raw, strings.TrimSuffix(path.Base(name), ".docj"))
		if e != nil {
			return e
		}
		if !slices.ContainsFunc(spans, func(span docast.Span) bool { return len(members[span.ID]) > 0 }) {
			continue
		}
		input, e := docast.OriginalTreeInput(raw)
		if e != nil {
			return e
		}
		tree, _, e := dataparser.ParseJSON(input, engine.ParseOptions)
		if e != nil {
			return e
		}
		if e = apply(tree); e != nil {
			return e
		}
		trees[name], external[name] = tree, true
	}
	return nil
}
