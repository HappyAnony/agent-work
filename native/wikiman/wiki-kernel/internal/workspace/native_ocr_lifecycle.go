package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/radovskyb/watcher"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/eventscope"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/ocr"
	"wiki-kernel/internal/pollwatch"
)

// Same original status-bar message, explicitly bound to the publishing owner.
// Notifications run outside the source/DB gate while the publisher holds a pin.
func (l *Lease) SubscribeOriginalOCRStatus(ctx context.Context, sink func(IndexEvent, string)) (*eventscope.Subscription[IndexEvent], error) {
	if sink == nil {
		return nil, ErrIdentity
	}
	return l.SubscribeIndexEvents(ctx, func(e IndexEvent) {
		if e.Kind == "ocr-status" {
			sink(e, e.Detail)
		}
	})
}

func (r *resources) queueOCRImages(ctx context.Context, w *WorkspaceContext) error {
	if r.ocr == nil {
		return nil
	}
	if !r.blockTreeReady {
		if err := r.rebuildBlockTree(ctx, w); err != nil {
			return err
		}
	}
	db, err := r.native.Database(indexdb.BlockTree)
	if err != nil {
		return err
	}
	roots, err := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
	if err != nil {
		return err
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
						r.ocr.nodes[ocrNode{Box: root.BoxID, Doc: root.RootID, ID: p.ID}] = true
					}
				}
			}
			return ast.WalkContinue
		})
		if len(r.ocr.nodes) > 10000 {
			return ErrBudget
		}
	}
	return ctx.Err()
}

// HandleNativeAssetEvent shares the original watcher dispatch policy. It handles
// the OCR branch and derived thumbnail invalidation; the original rich attachment
// parsers/sync source writes are not implicitly imported into the portable core.
func (l *Lease) HandleNativeAssetEvent(ctx context.Context, e assetflow.Event) error {
	if !validOCRAsset(e.Path) {
		return ErrDocument
	}
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	err = pin.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := pin.workspace.resources, pin.workspace
		return assetflow.Dispatch(ctx, e, assetflow.Handler{
			Refresh: func(ctx context.Context) error { return r.loadAssets(ctx, w) },
			Index: func(ctx context.Context, e assetflow.Event) error {
				// Do not reuse a legacy seed (which has no hash) after a real asset
				// write. All referring nodes are refreshed, including previously
				// processed nodes and deleted images.
				if r.ocr != nil {
					if _, ok := r.ocr.texts[e.Path]; ok {
						delete(r.ocr.texts, e.Path)
						r.ocr.dirty = true
					}
					if err := r.queueOCRImages(ctx, w); err != nil {
						return err
					}
				}
				if e.Removed {
					return r.assetQueue.DeletePath(e.Path)
				}
				return nil
			},
			InvalidateThumbnail: func(ctx context.Context, p string) error {
				if !compressibleAsset(p) {
					return nil
				}
				target := path.Join(".local/workspaces", pin.PageID(), "temp/thumbnails", p)
				// Remove only one reconstructible file. Never recursively remove
				// a caller-supplied path or follow a symbolic link.
				f, err := openRootFile(w.store.root, target)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				if err != nil {
					return err
				}
				f.Close()
				return w.store.root.Remove(target)
			},
		})
	})
	if err != nil {
		return err
	}
	action := "change"
	if e.Written {
		action = "write"
	}
	if e.Removed {
		action = "remove"
	}
	event := pin.indexEvent("asset-source-changed", action, 0, 0)
	event.Detail = e.Path
	return pin.workspace.resources.indexEvents.Publish(event)
}
func compressibleAsset(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}

// This explicit bootstrap owns configuration, watcher, actual OCR, flushes and
// cancellation as one task. There is no InitConf/go/cron global singleton here.
// The nonrecursive polling backend preserves the original macOS watcher scope.
func (l *Lease) StartNativeOCRLifecycle(ctx context.Context, options ocr.Options, interval, watchInterval time.Duration) (*Task, error) {
	if interval <= 0 || watchInterval <= 0 {
		return nil, ErrIdentity
	}
	options.Candidates = append([]string(nil), options.Candidates...)
	options.Languages = append([]string(nil), options.Languages...)
	return l.StartTask(ctx, func(ctx context.Context, owner *Lease) error {
		r := owner.workspace.resources
		if err := r.serial(ctx, func() error {
			if r.ocrLifecycle {
				return errors.New("ocr_lifecycle_active")
			}
			r.ocrLifecycle = true
			return nil
		}); err != nil {
			return err
		}
		defer func() { _ = r.serial(context.Background(), func() error { r.ocrLifecycle = false; return nil }) }()
		if err := owner.ConfigureNativeOCR(ctx, options); err != nil {
			return err
		}
		if err := owner.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return r.queueOCRImages(ctx, owner.workspace) }); err != nil {
			return err
		}
		var folder string
		if err := owner.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			w := owner.workspace
			info, err := w.data.Lstat("assets")
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return ErrDocument
			}
			folder = filepath.Join(w.paths.DataDir, "assets")
			return nil
		}); err != nil {
			return err
		}
		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		watchErrors := make(chan error, 1)
		report := func(e error) {
			select {
			case watchErrors <- e:
			default:
			}
			cancel()
		}
		watch, err := pollwatch.Open(watchCtx, folder, watchInterval, func(callbackCtx context.Context, event watcher.Event) {
			rel, e := filepath.Rel(folder, event.Path)
			if e != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return
			}
			if event.FileInfo != nil && event.FileInfo.IsDir() {
				return
			}
			e = owner.HandleNativeAssetEvent(callbackCtx, assetflow.Event{Path: "assets/" + filepath.ToSlash(rel), Removed: event.Op == watcher.Remove, Written: event.Op == watcher.Write})
			if e != nil {
				report(e)
			}
		}, report)
		if err != nil {
			return err
		}
		err = owner.workspace.resources.indexEvents.Publish(owner.indexEvent("ocr-ready", "", 0, 0))
		if err == nil {
			ticker := time.NewTicker(interval)
		loop:
			for {
				select {
				case <-watchCtx.Done():
					err = watchCtx.Err()
					break loop
				case <-ticker.C:
					if err = owner.runNativeOCRCycle(watchCtx); err != nil {
						break loop
					}
				}
			}
			ticker.Stop()
		}
		cancel()
		closeErr := watch.Close(context.Background())
		select {
		case watchErr := <-watchErrors:
			return errors.Join(watchErr, closeErr)
		default:
		}
		if ctx.Err() == nil && watchCtx.Err() != nil && err == nil {
			err = errors.New("asset_watch_failed")
		}
		return errors.Join(err, closeErr)
	})
}
func (l *Lease) runNativeOCRCycle(ctx context.Context) error {
	return ocr.Cycle(ctx, func(ctx context.Context) error {
		if err := l.RunNativeOCR(ctx); err != nil {
			return err
		}
		if err := l.FlushNativeMainQueue(ctx); err != nil {
			return err
		}
		return l.FlushNativeAssetContents(ctx)
	}, l.FlushNativeOCRCache)
}
