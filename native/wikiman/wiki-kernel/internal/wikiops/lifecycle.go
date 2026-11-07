// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

func lifecycleAction(action string) bool {
	switch action {
	case "document.create", "document.import", "document.rename", "document.move", "document.sort", "document.delete", "document.deleted", "document.recover",
		"document.version.save", "document.version.list", "document.version.get", "document.version.apply", "asset.rename", "asset.delete":
		return true
	}
	return false
}

type lifecycleEdit struct {
	ctx     context.Context
	m       *workspace.WorkspaceManager
	request ContentRequest
	page    wikistore.Page
	prefix  string
	files   map[string]sourcetx.FileInfo
	tx      sourcetx.Request
	result  ContentResult
}

func contentLifecycle(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest) (ContentResult, error) {
	return planContentLifecycle(ctx, m, r, (*lifecycleEdit).plan)
}

func planContentLifecycle(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest, plan func(*lifecycleEdit) error, clientHash ...string) (ContentResult, error) {
	out := ContentResult{StoreID: r.StoreID, PageID: r.PageID}
	canonical := r
	canonical.DryRun = false
	raw, _ := json.Marshal(canonical)
	hash := sourcetx.Hash(raw)
	if len(clientHash) > 0 {
		hash = clientHash[0]
	}
	replay := func() (bool, error) {
		if r.OperationID == "" {
			return false, nil
		}
		receipt, found, e := m.LookupReceipt(ctx, r.StoreID, r.OperationID, hash)
		if e != nil || !found {
			return found, e
		}
		out.Receipt = &receipt
		if v := receipt.Outcome; v != nil {
			out.DocID, out.Path, out.Hash, out.AssetName, out.HistoryID = v.DocID, v.Path, v.Hash, v.AssetName, v.HistoryID
		}
		if !receipt.SourceCommitted {
			return true, sourcetx.ErrConflict
		}
		if r.DryRun {
			receipt.Status = "dry_run_replay"
			return true, nil
		}
		if !receipt.IndexFresh {
			if _, e = m.Rebuild(ctx, r.StoreID, "all"); e != nil {
				return true, errors.Join(sourcetx.ErrIndex, e)
			}
		}
		e = prepareLifecycleIndex(ctx, m, r.StoreID, r.PageID)
		out.OriginalIndexFresh = e == nil
		return true, e
	}
	if found, e := replay(); found || e != nil {
		return out, e
	}
	p := lifecycleEdit{m: m, request: r, prefix: "pages/" + r.PageID + "/data/", result: out, tx: sourcetx.Request{OperationID: r.OperationID, ClientHash: hash}}
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		p.ctx = c
		tree, e := m.Tree(c, r.StoreID)
		if e != nil {
			return e
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil {
			return fs.ErrNotExist
		}
		p.page = page
		if r.ExpectedTreeDigest != "" && r.ExpectedTreeDigest != tree.SourceDigest() {
			return sourcetx.ErrConflict
		}
		p.tx.ExpectedTreeDigest = tree.SourceDigest()
		if r.OperationID == "" {
			return p.readArchive()
		}
		files, guards, e := m.PageReadGuards(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		p.files = make(map[string]sourcetx.FileInfo, len(files))
		for _, f := range files {
			p.files[strings.TrimPrefix(f.Path, p.prefix)] = f
		}
		p.tx.ReadGuards = guards
		if e = plan(&p); e != nil {
			return e
		}
		p.tx.Noop = len(p.tx.Changes) == 0
		p.tx.Outcome = &sourcetx.Outcome{DocID: p.result.DocID, Path: p.result.Path, Hash: p.result.Hash, AssetName: p.result.AssetName, HistoryID: p.result.HistoryID}
		return c.Err()
	})
	if err != nil {
		if found, e := replay(); found {
			return out, e
		}
		return out, err
	}
	out = p.result
	if r.OperationID == "" {
		return out, nil
	}
	var receipt sourcetx.Receipt
	if r.DryRun {
		receipt, err = m.Preview(ctx, r.StoreID, p.tx)
	} else {
		receipt, err = m.Apply(ctx, r.StoreID, p.tx)
	}
	if receipt.OperationID != "" {
		out.Receipt = &receipt
	}
	if err != nil {
		if !receipt.SourceCommitted {
			if found, e := replay(); found {
				return out, e
			}
		}
		return out, err
	}
	if !r.DryRun {
		err = prepareLifecycleIndex(ctx, m, r.StoreID, r.PageID)
		out.OriginalIndexFresh = err == nil
	}
	return out, err
}

func prepareLifecycleIndex(ctx context.Context, m *workspace.WorkspaceManager, store, page string) error {
	err := m.WithReadContext(ctx, store, func(c context.Context) error {
		l, e := m.Acquire(c, store, page)
		if e != nil {
			return e
		}
		defer l.Release()
		_, e = l.PrepareNativeReads(c, false)
		return e
	})
	if err != nil {
		return errors.Join(sourcetx.ErrIndex, err)
	}
	return nil
}

func (p *lifecycleEdit) lease() (*workspace.Lease, error) {
	return p.m.Acquire(p.ctx, p.request.StoreID, p.request.PageID)
}
func (p *lifecycleEdit) read(name string) ([]byte, error) {
	return p.m.SourceFile(p.ctx, p.request.StoreID, name)
}
func (p *lifecycleEdit) change(name, expected string, data []byte, remove bool) {
	p.tx.Changes = append(p.tx.Changes, sourcetx.Change{Path: name, ExpectedHash: expected, Data: data, Delete: remove})
}
func (p *lifecycleEdit) write(name string, data []byte) {
	p.change(p.prefix+name, p.files[name].Hash, data, false)
}
func (p *lifecycleEdit) doc(box, id string) (string, []byte, error) {
	name, raw, e := p.m.DocumentSource(p.ctx, p.request.StoreID, p.request.PageID, box, id)
	if e != nil {
		return "", nil, e
	}
	rel := strings.TrimPrefix(name, p.prefix)
	if p.files[rel].Hash != sourcetx.Hash(raw) {
		return "", nil, sourcetx.ErrConflict
	}
	return rel, raw, nil
}
func (p *lifecycleEdit) checkPlanInputs(hashes map[string][32]byte) error {
	for name, hash := range hashes {
		if rel, ok := strings.CutPrefix(name, "data/"); ok && p.files[rel].Hash != hex.EncodeToString(hash[:]) {
			return sourcetx.ErrConflict
		}
	}
	return nil
}

func (p *lifecycleEdit) current() (string, []byte, error) {
	name, raw, e := p.doc(p.request.BoxID, p.request.DocID)
	if e == nil && sourcetx.Hash(raw) != p.request.ExpectedHash {
		e = sourcetx.ErrConflict
	}
	return name, raw, e
}
func (p *lifecycleEdit) isEntry(box, id string) bool {
	return p.page.EntryDocument != nil && p.page.EntryDocument.BoxID == box && p.page.EntryDocument.DocID == id
}
func (p *lifecycleEdit) plan() error {
	r := p.request
	switch r.Action {
	case "document.create":
		return p.create()
	case "document.import":
		return p.importArchive()
	case "document.sort":
		return p.reorder()
	case "document.recover":
		return p.restoreDeleted()
	case "document.version.apply":
		return p.applyVersion()
	case "asset.rename", "asset.delete":
		return p.asset()
	}
	name, raw, e := p.current()
	if e != nil {
		return e
	}
	switch r.Action {
	case "document.delete":
		return p.deleteDocument(name)
	case "document.move":
		return p.moveDocument(name, raw)
	case "document.rename":
		return p.renameDocument(name, raw)
	case "document.version.save":
		return p.saveVersion(raw)

	}
	return ErrArgument
}

func (p *lifecycleEdit) versionPath(id string) string {
	r := p.request
	return path.Join("versions/documents", r.PageID, r.BoxID, r.DocID, id, r.DocID+".docj")
}
func (p *lifecycleEdit) saveVersion(raw []byte) error {
	name := p.versionPath(p.request.OperationID)
	p.change(name, "", raw, false)
	mirrors, e := documentMirrors(raw)
	if e != nil {
		return e
	}
	for id := range mirrors {
		rel := "storage/av/" + id + ".json"
		data, e := p.bytes(rel)
		if e != nil {
			return e
		}
		p.change(path.Join(path.Dir(name), "av", id+".json"), "", data, false)
	}
	p.result.HistoryID = p.request.OperationID
	return nil
}
func (p *lifecycleEdit) tombstonePath() string {
	r := p.request
	return (sourcetx.DocumentTombstone{PageID: r.PageID, BoxID: r.BoxID, DocID: r.DocID}).RecordPath()
}
func (p *lifecycleEdit) readArchive() error {
	r := p.request
	if r.Action == "document.version.get" {
		raw, e := p.readVersion()
		if e != nil {
			return e
		}
		if _, _, e = (&docast.Parser{}).Parse(p.ctx, raw, r.DocID); e != nil {
			return e
		}
		p.result.AST, p.result.Hash = raw, sourcetx.Hash(raw)
		mirrors, e := documentMirrors(raw)
		if e != nil {
			return e
		}
		avs, e := p.versionAVSources(mirrors)
		if e != nil {
			return e
		}
		if len(avs) > 0 {
			p.result.VersionAVHashes = map[string]string{}
			for id, raw := range avs {
				p.result.VersionAVHashes[id] = sourcetx.Hash(raw)
			}
		}
		return nil
	}
	namespace := "versions"
	doc := r.DocID
	if r.Action == "document.deleted" {
		namespace, doc = "tombstones", ""
	}
	files, e := p.m.ArchiveFiles(p.ctx, r.StoreID, namespace, r.PageID, r.BoxID, doc)
	if e != nil {
		return e
	}
	if namespace == "versions" {
		filtered := files[:0]
		for _, f := range files {
			if strings.HasSuffix(f.Path, ".docj") {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	legacy := map[string]workspace.LegacyDocumentVersion{}
	if namespace == "versions" {
		l, e := p.lease()
		if e != nil {
			return e
		}
		old, e := l.NativeDocumentVersions(p.ctx, r.BoxID, r.DocID)
		l.Release()
		if e != nil {
			return e
		}
		for _, v := range old {
			key := "legacy:" + v.Path
			legacy[key] = v
			files = append(files, sourcetx.FileInfo{Path: key, Hash: v.Hash, Size: v.Size})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	raw, _ := json.Marshal(files)
	digest := sourcetx.Hash(append(raw, []byte(r.StoreID+"/"+r.PageID+"/"+r.BoxID+"/"+doc+"/"+namespace)...))
	offset, e := lifecycleOffset(r.Cursor, digest, len(files))
	if e != nil {
		return e
	}
	limit := r.Limit
	if limit == 0 {
		limit = 50
	}
	end := min(offset+limit, len(files))
	p.result.Total = len(files)
	if end < len(files) {
		cursor, _ := json.Marshal(pageCursor{Digest: digest, Offset: end})
		p.result.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	for _, f := range files[offset:end] {
		if v, ok := legacy[f.Path]; ok {
			p.result.Items = append(p.result.Items, ContentItem{DocID: doc, BoxID: r.BoxID, LegacyHistoryPath: v.Path, Title: v.Title, Hash: v.Hash, Size: v.Size})
			continue
		}
		if namespace == "versions" {
			p.result.Items = append(p.result.Items, ContentItem{DocID: doc, BoxID: r.BoxID, VersionID: path.Base(path.Dir(f.Path)), Path: f.Path, Hash: f.Hash, Size: f.Size})
			continue
		}
		data, e := p.read(f.Path)
		if e != nil {
			return e
		}
		if sourcetx.Hash(data) != f.Hash {
			return sourcetx.ErrConflict
		}
		t, e := sourcetx.DecodeDocumentTombstone(data, f.Path)
		if e != nil {
			return e
		}
		p.result.Tombstones = append(p.result.Tombstones, t)
		p.result.Items = append(p.result.Items, ContentItem{DocID: t.DocID, BoxID: t.BoxID, Path: t.OriginalPath, Hash: f.Hash, Size: f.Size})
	}
	return nil
}
func lifecycleOffset(cursor, digest string, count int) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, e := base64.RawURLEncoding.DecodeString(cursor)
	var c pageCursor
	if e != nil || json.Unmarshal(raw, &c) != nil || c.Digest != digest || c.Offset < 0 || c.Offset > count {
		return 0, sourcetx.ErrCursor
	}
	return c.Offset, nil
}

func (p *lifecycleEdit) subtree(name string) []string {
	paths := []string{name}
	prefix := strings.TrimSuffix(name, ".docj") + "/"
	for f := range p.files {
		if strings.HasPrefix(f, prefix) {
			paths = append(paths, f)
		}
	}
	sort.Strings(paths)
	return paths
}
func (p *lifecycleEdit) bytes(name string) ([]byte, error) {
	raw, e := p.read(p.prefix + name)
	if e == nil && sourcetx.Hash(raw) != p.files[name].Hash {
		e = sourcetx.ErrConflict
	}
	return raw, e
}
func (p *lifecycleEdit) deleteDocument(name string) error {
	return p.deleteDocuments([]string{name}, p.request.Cascade)
}

func (p *lifecycleEdit) deleteDocuments(names []string, cascade bool) error {
	l, err := p.lease()
	if err != nil {
		return err
	}
	defer l.Release()
	type deletion struct {
		tombstone  sourcetx.DocumentTombstone
		ids, files []string
		effects    []sourcetx.Change
	}
	deletions := make([]deletion, 0, len(names))
	allFiles := []string{}
	byBox := map[string][]string{}
	when := time.Now().UTC().Format(time.RFC3339)
	for _, name := range doclife.FilterPaths(names, "", false) {
		box, rel, _ := strings.Cut(name, "/")
		id := strings.TrimSuffix(path.Base(name), ".docj")
		files := p.subtree(name)
		t := sourcetx.DocumentTombstone{Version: 1, PageID: p.request.PageID, BoxID: box, DocID: id, OriginalPath: name, DeletedAt: when}
		var ids []string
		for _, file := range files {
			if activeDoc(file) {
				doc := strings.TrimSuffix(path.Base(file), ".docj")
				if (!cascade && file != name) || p.isEntry(box, doc) {
					return ErrArgument
				}
				ids = append(ids, doc)
			}
			raw, err := p.bytes(file)
			if err != nil {
				return err
			}
			hash := p.files[file].Hash
			t.Files = append(t.Files, sourcetx.DeletedFile{Path: file, Hash: hash})
			p.change(t.ArchivePath(file), "", raw, false)
			p.change(p.prefix+file, hash, nil, true)
		}
		planned, err := l.PlanNativeDocumentLifecycle(p.ctx, doclife.Request{Action: "delete", Box: box, Path: "/" + rel})
		if err != nil {
			return err
		}
		if len(planned.IDs) != len(ids) {
			return sourcetx.ErrConflict
		}
		if err = p.checkPlanInputs(planned.SourceHashes); err != nil {
			return err
		}
		byBox[box] = append(byBox[box], ids...)
		allFiles = append(allFiles, files...)
		deletions = append(deletions, deletion{tombstone: t, ids: ids, files: files})
	}
	ranks := map[string]int{}
	for box, ids := range byBox {
		removed, err := p.removeRanks(l, box, ids)
		if err != nil {
			return err
		}
		for id, rank := range removed {
			ranks[id] = rank
		}
	}
	// Find batches that touch the same AV mirror. Each such group has a
	// single aggregate afterimage and an atomic recovery boundary.
	effectOwners := map[string]int{}
	parents := make([]int, len(deletions))
	leader := func(i int) int {
		for parents[i] != i {
			i = parents[i]
		}
		return i
	}
	for i := range deletions {
		parents[i] = i
		d := &deletions[i]
		d.effects, err = p.deleteEffects(l, d.ids, d.files, allFiles)
		if err != nil {
			return err
		}
		for _, effect := range d.effects {
			if previous, ok := effectOwners[effect.Path]; ok {
				parents[leader(i)] = leader(previous)
			} else {
				effectOwners[effect.Path] = i
			}
		}
	}
	groups := map[int][]int{}
	for i := range deletions {
		root := leader(i)
		groups[root] = append(groups[root], i)
	}
	seen := map[string]bool{}
	for _, indices := range groups {
		if len(indices) == 1 {
			d := &deletions[indices[0]]
			if err = p.stageDeleteEffects(&d.tombstone.Effects, d.tombstone.EffectPath, d.effects, seen); err != nil {
				return err
			}
			continue
		}
		key := func(i int) string {
			t := deletions[i].tombstone
			return t.BoxID + "/" + t.DocID
		}
		slices.SortFunc(indices, func(a, b int) int { return strings.Compare(key(a), key(b)) })
		var ids, files, linked []string
		for _, i := range indices {
			d := &deletions[i]
			ids, files = append(ids, d.ids...), append(files, d.files...)
			linked = append(linked, key(i))
		}
		changes, err := p.deleteEffects(l, ids, files, allFiles)
		if err != nil {
			return err
		}
		owner := &deletions[indices[0]].tombstone
		if err = p.stageDeleteEffects(&owner.Effects, owner.EffectPath, changes, seen); err != nil {
			return err
		}
		for _, i := range indices {
			deletions[i].tombstone.Linked = linked
		}
	}
	assets := map[string]string{}
	for _, d := range deletions {
		t := d.tombstone
		if err = p.keepDocumentAssets(t, assets); err != nil {
			return err
		}
		t.Ranks = map[string]int{}
		for _, id := range d.ids {
			if rank, ok := ranks[id]; ok {
				t.Ranks[id] = rank
			}
		}
		raw, err := json.Marshal(t)
		if err != nil {
			return err
		}
		p.change(t.RecordPath(), "", raw, false)
		p.result.Hash = sourcetx.Hash(raw)
	}
	return nil
}
func (p *lifecycleEdit) restoreDeleted() error {
	r := p.request
	record := p.tombstonePath()
	raw, e := p.read(record)
	if e != nil {
		return e
	}
	if sourcetx.Hash(raw) != r.ExpectedHash {
		return sourcetx.ErrConflict
	}
	t, e := sourcetx.DecodeDocumentTombstone(raw, record)
	if e != nil {
		return e
	}
	type archivedDocument struct {
		tomb sourcetx.DocumentTombstone
		hash string
	}
	archives := []archivedDocument{{t, r.ExpectedHash}}
	for _, key := range t.Linked {
		name := "tombstones/documents/" + r.PageID + "/" + key + ".json"
		if name == record {
			continue
		}
		raw, err := p.read(name)
		if err != nil {
			return err
		}
		peer, err := sourcetx.DecodeDocumentTombstone(raw, name)
		if err != nil || !slices.Equal(t.Linked, peer.Linked) {
			return sourcetx.ErrConflict
		}
		archives = append(archives, archivedDocument{peer, sourcetx.Hash(raw)})
	}
	restored := map[string][]byte{}
	replaced := map[string]bool{}
	ranks := map[string]map[string]int{}
	for _, archive := range archives {
		member := archive.tomb
		if p.files[member.BoxID+"/.wiki/conf.json"].Hash == "" && p.files[member.BoxID+"/.siyuan/conf.json"].Hash == "" {
			if _, exists := p.tx.ReadGuards.Directories[p.prefix+member.BoxID]; !exists {
				return sourcetx.ErrConflict
			}
		}
		if e := p.restoreDocumentArchive(member, restored, replaced); e != nil {
			return e
		}
		if len(member.Ranks) > 0 {
			if ranks[member.BoxID] == nil {
				ranks[member.BoxID] = map[string]int{}
			}
			for id, rank := range member.Ranks {
				ranks[member.BoxID][id] = rank
			}
		}
		p.change(member.RecordPath(), archive.hash, nil, true)
	}
	if e = p.validateDocumentIDs(restored, replaced); e != nil {
		return e
	}
	if len(ranks) > 0 {
		l, e := p.lease()
		if e != nil {
			return e
		}
		defer l.Release()
		for box, values := range ranks {
			if e = p.mergeRanks(l, box, values); e != nil {
				return e
			}
		}
	}
	p.result.DocID, p.result.Path, p.result.Hash = t.DocID, t.OriginalPath, sourcetx.Hash(restored[t.OriginalPath])
	return nil
}

func (p *lifecycleEdit) restoreDocumentArchive(t sourcetx.DocumentTombstone, restored map[string][]byte, replaced map[string]bool) error {
	for _, f := range t.Files {
		if _, exists := p.files[f.Path]; exists || restored[f.Path] != nil {
			return sourcetx.ErrConflict
		}
		archived := t.ArchivePath(f.Path)
		data, err := p.read(archived)
		if err != nil {
			return err
		}
		if sourcetx.Hash(data) != f.Hash {
			return sourcetx.ErrConflict
		}
		restored[f.Path] = data
		p.change(p.prefix+f.Path, "", data, false)
		p.change(archived, f.Hash, nil, true)
	}
	return p.restoreDeleteEffects(t.Effects, t.EffectPath, restored, replaced)
}

func (p *lifecycleEdit) restoreDeleteEffects(effects []sourcetx.DeletedEffect, archivePath func(string) string, restored map[string][]byte, replaced map[string]bool) error {
	for _, effect := range effects {
		if p.files[effect.Path].Hash != effect.AfterHash || restored[effect.Path] != nil {
			return sourcetx.ErrConflict
		}
		archived := archivePath(effect.Path)
		data, err := p.read(archived)
		if err != nil {
			return err
		}
		if sourcetx.Hash(data) != effect.BeforeHash {
			return sourcetx.ErrConflict
		}
		restored[effect.Path] = data
		replaced[effect.Path] = true
		p.change(p.prefix+effect.Path, effect.AfterHash, data, false)
		p.change(archived, effect.BeforeHash, nil, true)
	}
	return nil
}

func (p *lifecycleEdit) validateDocumentIDs(newFiles map[string][]byte, replaced map[string]bool) error {
	parser := docast.Parser{}
	ids := map[string]bool{}
	add := func(name string, raw []byte) error {
		if !activeDoc(name) {
			return nil
		}
		spans, _, e := parser.Parse(p.ctx, raw, strings.TrimSuffix(path.Base(name), ".docj"))
		if e != nil {
			return e
		}
		for _, span := range spans {
			if ids[span.ID] {
				return sourcetx.ErrConflict
			}
			ids[span.ID] = true
		}
		return nil
	}
	for name, raw := range newFiles {
		if activeDoc(name) {
			box, _, _ := strings.Cut(name, "/")
			for parent := path.Dir(name); parent != box; parent = path.Dir(parent) {
				if newFiles[parent+".docj"] == nil && p.files[parent+".docj"].Hash == "" {
					return sourcetx.ErrConflict
				}
			}
		}
		if e := add(name, raw); e != nil {
			return e
		}
	}
	for name := range p.files {
		if !activeDoc(name) || replaced[name] {
			continue
		}
		raw, e := p.bytes(name)
		if e != nil {
			return e
		}
		if e = add(name, raw); e != nil {
			return e
		}
	}
	return nil
}
func activeDoc(name string) bool {
	box, rel, ok := strings.Cut(name, "/")
	return ok && nativeID.MatchString(box) && documentRelativePath(rel)
}
func (p *lifecycleEdit) removeRanks(l *workspace.Lease, box string, ids []string) (map[string]int, error) {
	ranks := map[string]int{}
	plan, e := l.PlanNativeDocumentSort(p.ctx, doctree.SortMutation{Mode: "remove", Box: box, Remove: ids})
	if e != nil || plan.Name == "" {
		return ranks, e
	}
	if err := p.checkSortPreimage(plan); err != nil {
		return nil, err
	}
	var before map[string]int
	if e = json.Unmarshal(plan.Before, &before); e != nil {
		return nil, e
	}
	for _, id := range ids {
		if rank, ok := before[id]; ok {
			ranks[id] = rank
		}
	}
	if len(ranks) > 0 {
		p.write(plan.Name, plan.After)
	}
	return ranks, nil
}
func (p *lifecycleEdit) mergeRanks(l *workspace.Lease, box string, ranks map[string]int) error {
	if len(ranks) == 0 {
		return nil
	}
	plan, e := l.PlanNativeDocumentSort(p.ctx, doctree.SortMutation{Mode: "merge", Box: box, Values: ranks})
	if e != nil || plan.Name == "" {
		return e
	}
	if e = p.checkSortPreimage(plan); e != nil {
		return e
	}
	if !bytes.Equal(plan.Before, plan.After) {
		p.write(plan.Name, plan.After)
	}
	return nil
}

func (p *lifecycleEdit) checkSortPreimage(plan doctree.SortPlan) error {
	hash := ""
	if plan.Before != nil {
		hash = sourcetx.Hash(plan.Before)
	}
	if p.files[plan.Name].Hash != hash {
		return sourcetx.ErrConflict
	}
	return nil
}
