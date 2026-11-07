// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// ReadProjection is a disposable owner-local index generation, not a source
// receipt. Its digest includes document, AV, attachment and parser inputs.
type ReadProjection struct {
	SourceHash string `json:"sourceHash"`
	Documents  int    `json:"documents"`
	Rebuilt    bool   `json:"rebuilt"`
}

type nativeReadSource struct {
	entries     []workspaceDirectoryEntry
	directories []workspaceDirectoryManifest
	hash        string
	files       map[string]string
}

func hashNativeReadFile(ctx context.Context, w *WorkspaceContext, e workspaceDirectoryEntry) (out string, result error) {
	f, err := openNativeFile(w, e.name)
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, f.Close())
		if result != nil {
			out = ""
		}
	}()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !sameWorkspaceDirectoryInfo(opened, e.info) {
		return "", errWorkspaceDirectorySourceChanged
	}
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, re := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			size += int64(n)
			if size > e.info.Size() {
				return "", errWorkspaceDirectorySourceChanged
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return "", re
		}
		if n == 0 {
			return "", io.ErrNoProgress
		}
	}
	after, err := f.Stat()
	if err != nil {
		return "", err
	}
	current, err := w.data.Lstat(e.name)
	if err != nil {
		return "", err
	}
	if size != e.info.Size() || !sameWorkspaceDirectoryInfo(opened, after) || !sameWorkspaceDirectoryInfo(current, e.info) {
		return "", errWorkspaceDirectorySourceChanged
	}
	return hex.EncodeToString(h.Sum(nil)), ctx.Err()
}

func (r *resources) readSourceSnapshot(ctx context.Context, w *WorkspaceContext) (out nativeReadSource, err error) {
	out.files = map[string]string{}
	h := sha256.New()
	config, err := r.configuration.Clone()
	if err != nil {
		return out, err
	}
	// These are the original parser/index inputs. Machine paths and runtime
	// counters cannot invalidate an otherwise identical projection.
	paragraphBeginningSpace := false
	if config.Export != nil {
		paragraphBeginningSpace = config.Export.ParagraphBeginningSpace
	}
	settings, e := json.Marshal([]interface{}{config.Search, config.Editor, paragraphBeginningSpace})
	if e != nil {
		return out, e
	}
	h.Write(settings)
	if w.data == nil {
		out.hash = hex.EncodeToString(h.Sum(nil))
		return out, ctx.Err()
	}
	var scan func(string) error
	scan = func(dir string) error {
		listing, e := confirmedWorkspaceDirectoryManifest(ctx, w, dir, workspaceDirectoryIO{})
		if e != nil {
			return e
		}
		out.directories = append(out.directories, listing)
		for _, entry := range listing.entries {
			if e = ctx.Err(); e != nil {
				return e
			}
			out.entries = append(out.entries, entry)
			if len(out.entries) > 100000 {
				return ErrBudget
			}
			if entry.info.Mode()&fs.ModeSymlink != 0 {
				return ErrDocument
			}
			if entry.info.IsDir() {
				if e = scan(entry.name); e != nil {
					return e
				}
				continue
			}
			if !entry.info.Mode().IsRegular() {
				return ErrDocument
			}
			hash, e := hashNativeReadFile(ctx, w, entry)
			if e != nil {
				return e
			}
			out.files[entry.name] = hash
			fmt.Fprintf(h, "%d:%s:%s;", len(entry.name), entry.name, hash)
		}
		return nil
	}
	if err = scan("."); err != nil {
		return nativeReadSource{}, err
	}
	for _, dir := range out.directories {
		current, e := workspaceDirectorySourceManifest(ctx, w, dir.name, workspaceDirectoryIO{})
		if e != nil {
			return nativeReadSource{}, e
		}
		if !sameWorkspaceDirectoryManifest(dir, current) {
			return nativeReadSource{}, errWorkspaceDirectorySourceChanged
		}
	}
	out.hash = hex.EncodeToString(h.Sum(nil))
	return out, ctx.Err()
}

// PrepareNativeReads rebuilds the complete original main/blocktree projection
// from actual source. A fresh bundle remains private until all trees, reference
// lookups and final source checks succeed. History/asset-content queues survive.
func (l *Lease) PrepareNativeReads(ctx context.Context, force bool) (out ReadProjection, err error) {
	pin, e := l.pin(true)
	if e != nil {
		return out, e
	}
	defer pin.Release()
	l = pin
	var events []IndexEvent
	err = l.WithNativeIndexes(ctx, func(original *indexdb.Bundle) (result error) {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil || r.configuration.Editor == nil {
			return indexdb.ErrProjection
		}
		before, e := r.readSourceSnapshot(ctx, w)
		if e != nil {
			return e
		}
		out.SourceHash = before.hash
		for _, entry := range before.entries {
			if isNativeReadDocument(entry.name) {
				out.Documents++
			}
		}
		if !force && r.readProjectionHash == before.hash {
			present, e := original.ReadProjectionPresent(ctx, false)
			if e != nil {
				return e
			}
			if present {
				return nil
			}
		}
		if e = l.flushNativeMainLocked(ctx, original, &events); e != nil {
			return e
		}
		candidate, e := indexdb.OpenMemory(ctx)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, candidate.Close()) }()
		ready := r.blockTreeReady
		r.native = candidate
		r.blockTreeReady = false
		defer func() { r.native = original; r.blockTreeReady = ready }()
		if w.data != nil {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		opts, e := r.configuredParserOptions(ctx, w)
		if e != nil {
			return e
		}
		ignore, e := readNativeFile(ctx, w, ".wiki/indexignore", 1<<20)
		if w.data == nil {
			ignore, e = nil, nil
		}
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		records := make([]indexdb.TreeRecords, 0, out.Documents)
		for _, entry := range before.entries {
			if !isNativeReadDocument(entry.name) {
				continue
			}
			parts := strings.Split(entry.name, "/")
			doc := strings.TrimSuffix(path.Base(entry.name), ".docj")
			tree, e := r.locatedTree(ctx, w, parts[0], doc)
			if e != nil {
				return e
			}
			projected, e := indexdb.ProjectTree(ctx, tree, nativeparser.NewConfiguredIndex(opts), indexdb.TreeServices{Content: r.nativeContent, PrepareOCR: r.nativeOCR, LocalDefinitions: true, IndexAssetPath: r.configuration.Search.IndexAssetPath, AssetHash: func(c context.Context, p, box, doc string) (string, error) {
				return originalAssetHash(c, w, p, box, doc)
			}}, r.nativeDefinition)
			if e != nil {
				return e
			}
			projected.IgnoreLines = strings.Split(strings.ReplaceAll(string(ignore), "\r\n", "\n"), "\n")
			records = append(records, projected)
		}
		if e = candidate.Transaction(ctx, indexdb.Main, func(tx *sql.Tx) error {
			for _, record := range records {
				if _, e := indexdb.WriteTree(ctx, tx, record, false, r.configuration.Search.CaseSensitive); e != nil {
					return e
				}
			}
			return ctx.Err()
		}); e != nil {
			return e
		}
		after, e := r.readSourceSnapshot(ctx, w)
		if e != nil {
			return e
		}
		if before.hash != after.hash {
			return errWorkspaceDirectorySourceChanged
		}
		if e = original.AdoptReadProjection(candidate); e != nil {
			return e
		}
		r.readProjectionHash = before.hash
		ready = true
		r.nativeBlocks.Clear()
		r.nativeRefs.Clear()
		if r.ial != nil {
			r.ial.ClearBlocks()
			r.ial.ClearDocs()
		}
		out.Rebuilt = true
		return nil
	})
	for _, event := range events {
		err = errors.Join(err, l.workspace.resources.indexEvents.Publish(event))
	}
	if err != nil {
		return ReadProjection{}, err
	}
	return out, nil
}
func isNativeReadDocument(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) > 1 && documentID.MatchString(parts[0]) && strings.HasSuffix(name, ".docj")
}

func (l *Lease) VerifyNativeReadSource(ctx context.Context, hash string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		value, e := l.workspace.resources.readSourceSnapshot(ctx, l.workspace)
		if e != nil {
			return e
		}
		if value.hash != hash {
			return errWorkspaceDirectorySourceChanged
		}
		return nil
	})
}

// ReadNativeBlockAttributes reads a detached IAL without legacy read-time repair.
func (l *Lease) ReadNativeBlockAttributes(ctx context.Context, id string) (out map[string]string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		tree, e := r.nativeTreeByBlock(ctx, w, id)
		if e != nil {
			return e
		}
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.ID == id {
				out = parse.IAL2MapUnEsc(n.KramdownIAL)
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if out == nil {
			return ErrDocument
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}

// NativeReadSourceState keeps raw/config/resource reads independent of original
// AST indexing. Unknown future nodes can still be read losslessly as bytes.
func (l *Lease) NativeReadSourceState(ctx context.Context) (out ReadProjection, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		source, e := l.workspace.resources.readSourceSnapshot(ctx, l.workspace)
		if e != nil {
			return e
		}
		out.SourceHash = source.hash
		for _, entry := range source.entries {
			if isNativeReadDocument(entry.name) {
				out.Documents++
			}
		}
		return nil
	})
	if err != nil {
		return ReadProjection{}, err
	}
	return
}
