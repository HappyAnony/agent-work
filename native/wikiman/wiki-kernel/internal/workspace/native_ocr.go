package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/ocr"
)

type ocrEntry struct {
	Text   string `json:"text"`
	Hash   string `json:"hash"`
	Engine string `json:"engine"`
}
type ocrDocument struct {
	Version int                 `json:"version"`
	Entries map[string]ocrEntry `json:"entries"`
}
type ocrNode struct{ Box, Doc, ID string }
type nativeOCRState struct {
	engine *ocr.Engine
	texts  map[string]ocrEntry
	nodes  map[ocrNode]bool
	dirty  bool
	busy   bool
}

func readBounded(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, ErrBudget
	}
	return raw, ctx.Err()
}
func ocrCachePath(page string) string { return ".local/workspaces/" + page + "/temp/ocr-texts.json" }

// Configuration is explicit, including disabled mode. It probes only the supplied
// executable candidates; no WIKI_* policy, chdir or global Tesseract state is
// consulted. A bare executable name still uses the device's normal PATH lookup.
func (l *Lease) ConfigureNativeOCR(ctx context.Context, options ocr.Options) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		engine, err := ocr.Open(ctx, options)
		if err != nil {
			return err
		}
		state := &nativeOCRState{engine: engine, texts: map[string]ocrEntry{}, nodes: map[ocrNode]bool{}}
		raw, err := readNativeFile(ctx, w, "assets/ocr-texts.json", 16<<20)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(raw) > 0 {
			var legacy map[string]string
			if err = json.Unmarshal(raw, &legacy); err != nil {
				return err
			}
			if len(legacy) > 100000 {
				return ErrBudget
			}
			for k, v := range legacy {
				if !validOCRAsset(k) {
					return ErrDocument
				}
				state.texts[k] = ocrEntry{Text: v}
			}
		}
		f, err := openRootFile(w.store.root, ocrCachePath(l.PageID()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if f != nil {
			raw, err = readBounded(ctx, f, 16<<20)
			f.Close()
			if err != nil {
				return err
			}
			var doc ocrDocument
			if err = json.Unmarshal(raw, &doc); err != nil {
				return err
			}
			if doc.Version != 1 || len(doc.Entries) > 100000 {
				return ErrDocument
			}
			for k, v := range doc.Entries {
				if !validOCRAsset(k) {
					return ErrDocument
				}
				state.texts[k] = v
			}
		}
		if err = r.loadAssets(ctx, w); err != nil {
			return err
		}
		// Enabling/reconfiguring OCR after documents were already indexed must
		// refresh those existing image-bearing blocks, not only future admissions.
		if engine.Enabled && r.blockTreeReady {
			db, e := r.native.Database(indexdb.BlockTree)
			if e != nil {
				return e
			}
			roots, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
			if e != nil {
				return e
			}
			for _, root := range roots {
				tree, e := r.originalTree(ctx, w, root.BoxID, root.RootID)
				if e != nil {
					return e
				}
				ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
					if enter && n.Type == ast.NodeImage {
						for p := n.Parent; p != nil && p.Type != ast.NodeDocument; p = p.Parent {
							if p.IsBlock() && p.ID != "" {
								state.nodes[ocrNode{Box: root.BoxID, Doc: root.RootID, ID: p.ID}] = true
							}
						}
					}
					return ast.WalkContinue
				})
				if len(state.nodes) > 10000 {
					return ErrBudget
				}
			}
		}
		r.ocr = state
		r.nativeContent.AssetText = func(ctx context.Context, p string) (string, error) { return r.ocrText(ctx, w, p) }
		r.nativeOCR = func(ctx context.Context, n *ast.Node) error {
			if !state.engine.Enabled {
				return nil
			}
			if len(state.nodes) >= 10000 {
				return ErrBudget
			}
			if n != nil && n.ID != "" {
				state.nodes[ocrNode{Box: n.Box, Doc: docFromNativePath(n.Path), ID: n.ID}] = true
			}
			return ctx.Err()
		}
		return nil
	})
}
func validOCRAsset(p string) bool {
	return strings.HasPrefix(p, "assets/") && fs.ValidPath(p) && !strings.Contains(p, "\\")
}
func (r *resources) ocrText(ctx context.Context, w *WorkspaceContext, p string) (string, error) {
	if !validOCRAsset(p) {
		return "", nil
	}
	entry, ok := r.ocr.texts[p]
	if !ok {
		return "", nil
	}
	if entry.Hash != "" {
		hash, err := originalAssetHash(ctx, w, p, "", "")
		if err != nil {
			return "", err
		}
		if hash != entry.Hash {
			return "", nil
		}
	}
	return entry.Text, ctx.Err()
}

// RunNativeOCR mirrors autoOCRAssets: at most eight images (the original i>=7
// loop), cleanup missing text, then reindex queued nodes. Failed nodes stay queued.
func (l *Lease) RunNativeOCR(ctx context.Context) error {
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	r, w := pin.workspace.resources, pin.workspace
	var state *nativeOCRState
	var names []string
	err = pin.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		state = r.ocr
		if state == nil {
			return ocr.ErrInvalid
		}
		if !state.engine.Enabled {
			return nil
		}
		if state.busy {
			return errors.New("ocr_job_active")
		}
		if e := r.loadAssets(ctx, w); e != nil {
			return e
		}
		for p := range r.nativeAssets.Snapshot() {
			if ocr.Extractable(p) {
				names = append(names, p)
			}
		}
		sort.Strings(names)
		state.busy = true
		return nil
	})
	if err != nil {
		return err
	}
	if !state.engine.Enabled {
		return nil
	}
	defer func() { _ = r.serial(context.Background(), func() error { state.busy = false; return nil }) }()
	processed := 0
	for _, p := range names {
		var data []byte
		var hash string
		eligible := false
		err = pin.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			if r.ocr != state {
				return ErrStale
			}
			current, e := originalAssetHash(ctx, w, p, "", "")
			if e != nil {
				return e
			}
			if old, ok := state.texts[p]; ok && old.Hash == current && old.Engine == state.engine.Signature {
				return nil
			}
			f, e := openNativeFile(w, p)
			if errors.Is(e, fs.ErrNotExist) {
				return nil
			}
			if e != nil {
				return e
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				return e
			}
			if info.Size() > state.engine.MaxSize() {
				return nil
			}
			data, e = state.engine.ReadImage(f)
			if e != nil {
				return e
			}
			hash, e = indexdb.AssetEtag(ctx, bytes.NewReader(data), int64(len(data)))
			eligible = e == nil
			return e
		})
		if err != nil {
			return err
		}
		if !eligible {
			continue
		}
		// Only a private byte snapshot and immutable engine cross this boundary.
		// Source locks and the Context gate are not held while the child is running.
		rows, e := state.engine.Recognize(ctx, p, data)
		if e != nil {
			return e
		}
		processed++
		accepted := false
		err = pin.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			if r.ocr != state {
				return ErrStale
			}
			current, e := originalAssetHash(ctx, w, p, "", "")
			if e != nil {
				return e
			}
			if current != hash {
				return nil
			}
			state.texts[p] = ocrEntry{Text: ocr.Text(rows), Hash: hash, Engine: state.engine.Signature}
			state.dirty = true
			accepted = true
			return nil
		})
		if err != nil {
			return err
		}
		if accepted {
			event := pin.indexEvent("ocr-status", p, processed, len(names))
			event.Detail = ocr.StatusMessage(p, ocr.Text(rows))
			if e := r.indexEvents.Publish(event); e != nil {
				return e
			}
		}
		if processed >= 8 {
			break
		}
	}
	return pin.enqueueNativeMain(ctx, func(r *resources) error {
		if r.ocr != state {
			return ErrStale
		}
		if err := r.loadAssets(ctx, w); err != nil {
			return err
		}
		assets := r.nativeAssets.Snapshot()
		for p := range state.texts {
			if _, ok := assets[p]; !ok {
				delete(state.texts, p)
				state.dirty = true
			}
		}
		for node := range state.nodes {
			if !documentID.MatchString(node.Box) || !documentID.MatchString(node.Doc) || !documentID.MatchString(node.ID) {
				return ErrDocument
			}
			ready, e := r.ocrNodeReady(ctx, w, node)
			if e != nil {
				return e
			}
			if !ready {
				continue
			}
			if e = r.queueNativeIndexNode(ctx, w, node.Box, node.Doc, node.ID); e != nil {
				return e
			}
			delete(state.nodes, node)
		}
		return nil
	})
}

// Keep refresh requests across the original eight-image batch limit. A queued
// node is acknowledged only when its eligible catalog images have current text;
// otherwise a later OCR batch would never refresh that node's FTS content.
func (r *resources) ocrNodeReady(ctx context.Context, w *WorkspaceContext, key ocrNode) (bool, error) {
	tree, err := r.originalTree(ctx, w, key.Box, key.Doc)
	if err != nil {
		return false, err
	}
	var node *ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, e bool) ast.WalkStatus {
		if e && n.ID == key.ID {
			node = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if node == nil {
		delete(r.ocr.nodes, key)
		return false, nil
	} // A removed block needs no further refresh.
	ready := true
	ast.Walk(node, func(n *ast.Node, e bool) ast.WalkStatus {
		if !e || n.Type != ast.NodeImage {
			return ast.WalkContinue
		}
		dest := n.ChildByType(ast.NodeLinkDest)
		if dest == nil {
			return ast.WalkContinue
		}
		p := string(dest.Tokens)
		if !r.nativeAssets.Exists(p) || !ocr.Extractable(p) {
			return ast.WalkContinue
		}
		f, openErr := openNativeFile(w, p)
		if openErr != nil {
			err = openErr
			return ast.WalkStop
		}
		info, statErr := f.Stat()
		f.Close()
		if statErr != nil {
			err = statErr
			return ast.WalkStop
		}
		if info.Size() > r.ocr.engine.MaxSize() {
			return ast.WalkContinue
		}
		entry, ok := r.ocr.texts[p]
		if !ok || entry.Engine != r.ocr.engine.Signature {
			ready = false
			return ast.WalkStop
		}
		hash, hashErr := originalAssetHash(ctx, w, p, "", "")
		if hashErr != nil {
			err = hashErr
			return ast.WalkStop
		}
		if entry.Hash != hash {
			ready = false
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return ready, err
}

// The cache is reconstructible local runtime state, not assets source, source
// journal or a save receipt. Legacy source OCR text is read-only seed material.
func (l *Lease) FlushNativeOCRCache(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s := l.workspace.resources.ocr
		if s == nil || !s.dirty {
			return nil
		}
		raw, err := json.Marshal(ocrDocument{Version: 1, Entries: s.texts})
		if err != nil {
			return err
		}
		if len(raw) > 16<<20 {
			return ErrBudget
		}
		if err = writeLocalOCR(ctx, l.workspace.store.root, ocrCachePath(l.PageID()), raw); err != nil {
			return err
		}
		s.dirty = false
		return nil
	})
}
func (l *Lease) StartNativeOCRBackground(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, lease *Lease) error { return lease.runNativeOCRCycle(ctx) })
}
func openRootFile(root *os.Root, name string) (*os.File, error) {
	if root == nil || !fs.ValidPath(name) {
		return nil, ErrDocument
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, ErrDocument
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, ErrDocument
		}
	}
	return root.Open(name)
}
func writeLocalOCR(ctx context.Context, root *os.Root, name string, raw []byte) error {
	return writeLocalState(ctx, root, name, raw)
}

// Shared atomic writer for reconstructible per-Workspace local state.
func writeLocalState(ctx context.Context, root *os.Root, name string, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parts := strings.Split(path.Dir(name), "/")
	for i := range parts {
		dir := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			if err = root.Mkdir(dir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			info, err = root.Lstat(dir)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrDocument
		}
	}
	if info, err := root.Lstat(name); err == nil && (!info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0) {
		return ErrDocument
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp := path.Join(path.Dir(name), ".ocr-"+rand.Text()+".tmp")
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = root.Rename(tmp, name); err != nil {
		return err
	}
	dir, err := root.Open(path.Dir(name))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
