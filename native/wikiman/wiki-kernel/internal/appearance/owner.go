// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package appearance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"wiki-kernel/conf"
)

var ErrDependency = errors.New("appearance_resources_unbound")
var ErrClosed = errors.New("appearance_resources_closed")

type Event struct{ Theme, URL string }
type Options struct {
	Bundled, Destination, Themes, Icons string
	AllowLinkedDirectories              bool
	Change                              func(context.Context, Event) error
	Report                              func(error)
}
type Owner struct {
	options Options
	gate    chan struct{}
	closed  bool
	watch   *watch
}
type watch struct {
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	ready   atomic.Bool
}

func New(opts Options) (*Owner, error) {
	for _, p := range []string{opts.Bundled, opts.Destination, opts.Themes, opts.Icons} {
		if !filepath.IsAbs(p) {
			return nil, ErrDependency
		}
	}
	for _, pair := range [][2]string{{opts.Bundled, opts.Destination}, {opts.Destination, opts.Bundled}} {
		relative, e := filepath.Rel(pair[0], pair[1])
		if e != nil || relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, ErrDependency
		}
	}
	return &Owner{options: opts, gate: make(chan struct{}, 1)}, nil
}
func (o *Owner) enter(ctx context.Context) error {
	select {
	case o.gate <- struct{}{}:
		if o.closed {
			<-o.gate
			return ErrClosed
		}
		if e := ctx.Err(); e != nil {
			<-o.gate
			return e
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (o *Owner) leave() { <-o.gate }

func (o *Owner) directory(path string) (*os.Root, error) {
	stat, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		if !o.options.AllowLinkedDirectories {
			return nil, ErrDependency
		}
		path, e = filepath.EvalSymlinks(path)
		if e != nil {
			return nil, e
		}
	}
	if !stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
		return nil, ErrDependency
	}
	return os.OpenRoot(path)
}

type resource struct {
	root string
	name string
}

// Custom resources remain; bundled files overlay the matching installed name,
// as the original copy did. Preparing the merged view does not install files.
func (o *Owner) entries(ctx context.Context, kind, installed string) ([]resource, error) {
	return o.entriesWithIO(ctx, kind, installed, appearanceManifestIO{})
}

func (o *Owner) entriesWithIO(ctx context.Context, kind, installed string, op appearanceManifestIO) ([]resource, error) {
	found := map[string]resource{}
	for _, base := range []string{installed, filepath.Join(o.options.Bundled, kind)} {
		r, e := o.directory(base)
		if errors.Is(e, fs.ErrNotExist) {
			continue
		}
		if e != nil {
			return nil, e
		}
		entries, e := confirmedAppearanceManifest(ctx, r, appearanceManifestOptions{singleLevel: true, maxEntries: 10001}, op)
		e = errors.Join(e, op.closeDirectory(r))
		if e != nil {
			return nil, e
		}
		for _, entry := range entries[1:] {
			name := path.Base(entry.name)
			if strings.HasPrefix(name, ".") {
				continue
			}
			if entry.info.IsDir() || entry.info.Mode()&fs.ModeSymlink != 0 {
				found[name] = resource{base, name}
			}
		}
	}
	var out []resource
	for _, v := range found {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}
func readJSON(root *os.Root, name string, v any) error {
	file, e := root.Open(name)
	if e != nil {
		return e
	}
	defer file.Close()
	raw, e := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if e != nil {
		return e
	}
	if len(raw) > 2<<20 {
		return ErrDependency
	}
	return json.Unmarshal(raw, v)
}
func (o *Owner) prepare(ctx context.Context, input *conf.Appearance) (*conf.Appearance, []string, error) {
	entries, e := o.entries(ctx, "themes", o.options.Themes)
	if e != nil {
		return nil, nil, e
	}
	var themes []conf.AppearanceThemeRecord
	var folders []string
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		r, e := o.directory(filepath.Join(entry.root, entry.name))
		if e != nil {
			return nil, nil, e
		}
		var meta *struct {
			Version     string
			Modes       []string
			DisplayName *struct {
				ZhCN string `json:"zh_CN"`
			}
		}
		e = readJSON(r, "theme.json", &meta)
		if e != nil || meta == nil {
			r.Close()
			continue
		}
		_, js := r.Stat("theme.js")
		cn := ""
		if meta.DisplayName != nil {
			cn = meta.DisplayName.ZhCN
		}
		r.Close()
		// Installed custom JS can survive a bundled overlay with no theme.js file.
		if js != nil {
			if target, e := o.directory(filepath.Join(o.options.Themes, entry.name)); e == nil {
				_, js = target.Stat("theme.js")
				target.Close()
			}
		}
		themes = append(themes, conf.AppearanceThemeRecord{Name: entry.name, Version: meta.Version, ChineseName: cn, Modes: meta.Modes, JavaScript: js == nil})
		if entry.name != "midnight" && entry.name != "daylight" {
			folders = append(folders, filepath.Join(o.options.Themes, entry.name))
		}
	}
	entries, e = o.entries(ctx, "icons", o.options.Icons)
	if e != nil {
		return nil, nil, e
	}
	var icons []conf.AppearanceIconRecord
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		r, e := o.directory(filepath.Join(entry.root, entry.name))
		if e != nil {
			return nil, nil, e
		}
		var meta *struct{ Version string }
		e = readJSON(r, "icon.json", &meta)
		r.Close()
		if e == nil && meta != nil {
			icons = append(icons, conf.AppearanceIconRecord{Name: entry.name, Version: meta.Version})
		}
	}
	result, e := conf.ProjectAppearance(input, themes, icons)
	return result, folders, e
}
func (o *Owner) Prepare(ctx context.Context, input *conf.Appearance) (*conf.Appearance, error) {
	if e := o.enter(ctx); e != nil {
		return nil, e
	}
	defer o.leave()
	value, _, e := o.prepare(ctx, input)
	return value, e
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.Reader.Read(p)
}
func (o *Owner) install(ctx context.Context) error {
	return o.installWithIO(ctx, appearanceManifestIO{})
}

func (o *Owner) installWithIO(ctx context.Context, op appearanceManifestIO) (result error) {
	source, e := o.directory(o.options.Bundled)
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, op.closeDirectory(source)) }()
	manifestOptions := appearanceManifestOptions{rejectSymlinks: true, regularFilesOnly: true, maxEntries: 100000, maxBytes: 512 << 20}
	manifest, e := confirmedAppearanceManifest(ctx, source, manifestOptions, op)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(o.options.Destination, 0700); e != nil {
		return e
	}
	destination, e := o.directory(o.options.Destination)
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, op.closeDirectory(destination)) }()
	for _, entry := range manifest[1:] {
		if e = ctx.Err(); e != nil {
			return e
		}
		if entry.info.IsDir() {
			if e = destination.MkdirAll(entry.name, 0700); e != nil {
				return e
			}
			continue
		}
		input, e := openAppearanceManifestFile(source, entry, op)
		if e != nil {
			return e
		}
		// Refuse destination links before truncation; Root also refuses escapes.
		if stat, targetErr := destination.Lstat(entry.name); targetErr == nil && stat.Mode()&fs.ModeSymlink != 0 {
			return errors.Join(ErrDependency, op.closeOpenedFile(input))
		} else if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
			return errors.Join(targetErr, op.closeOpenedFile(input))
		}
		output, e := destination.OpenFile(entry.name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return errors.Join(e, op.closeOpenedFile(input))
		}
		_, copyErr := io.Copy(output, contextReader{ctx, input})
		after, statErr := input.Stat()
		if statErr == nil && !appearanceInfoEqual(after, entry.info) {
			statErr = errAppearanceSourceChanged
		}
		if e = errors.Join(copyErr, statErr, op.closeOpenedFile(input), output.Close()); e != nil {
			return e
		}
	}
	return confirmFinalAppearanceManifest(ctx, source, manifestOptions, manifest, op)
}
func closeBackend(w *fsnotify.Watcher) error {
	var result error
	for _, p := range w.WatchList() {
		result = errors.Join(result, w.Remove(p))
	}
	return errors.Join(result, w.Close())
}
func (w *watch) close(ctx context.Context) error {
	var e error
	w.once.Do(func() { w.ready.Store(false); w.cancel(); e = closeBackend(w.watcher) })
	select {
	case <-w.done:
		return e
	case <-ctx.Done():
		return errors.Join(e, ctx.Err())
	}
}
func (o *Owner) stop(ctx context.Context) error {
	if o.watch == nil {
		return nil
	}
	if e := o.watch.close(ctx); e != nil {
		return e
	}
	o.watch = nil
	return nil
}
func (o *Owner) activate(ctx context.Context, input *conf.Appearance, folders []string) error {
	if len(folders) == 0 {
		return nil
	}
	if o.options.Change == nil || o.options.Report == nil {
		return ErrDependency
	}
	snapshot, e := (conf.AppData{Appearance: input}).Clone()
	if e != nil {
		return e
	}
	backend, e := fsnotify.NewWatcher()
	if e != nil {
		return e
	}
	owner, cancel := context.WithCancel(context.Background())
	w := &watch{watcher: backend, cancel: cancel, done: make(chan struct{})}
	go consume(owner, w, snapshot.Appearance, o.options)
	failed := func(e error) error {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		return errors.Join(e, w.close(cleanup))
	}
	for _, folder := range folders {
		if e = ctx.Err(); e != nil {
			return failed(e)
		}
		if e = backend.Add(folder); e != nil {
			return failed(e)
		}
	}
	w.ready.Store(true)
	o.watch = w
	return nil
}
func consume(ctx context.Context, w *watch, a *conf.Appearance, opts Options) {
	defer close(w.done)
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if ctx.Err() != nil || !w.ready.Load() {
				continue
			} // Drain until backend Close; no late callbacks.
			if event.Op&fsnotify.Write == 0 || !strings.HasSuffix(event.Name, "theme.css") {
				continue
			}
			name := filepath.Base(filepath.Dir(event.Name))
			selected := a.ThemeLight
			if a.Mode == 1 {
				selected = a.ThemeDark
			} else if a.Mode != 0 {
				continue
			}
			if name != selected || ctx.Err() != nil {
				continue
			}
			e := opts.Change(ctx, Event{Theme: name, URL: "/appearance/themes/" + name + "/theme.css?" + fmt.Sprint(time.Now().Unix())})
			if e != nil && ctx.Err() == nil {
				opts.Report(e)
			}
		case e, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			if ctx.Err() == nil {
				opts.Report(e)
			}
		}
	}
}

// Apply installs only trusted device resources, then owns the original CSS
// listener. It never writes wiki source or preferences. Callbacks respect ctx.
func (o *Owner) Apply(ctx context.Context, input *conf.Appearance) error {
	if e := o.enter(ctx); e != nil {
		return e
	}
	defer o.leave()
	_, folders, e := o.prepare(ctx, input)
	if e != nil {
		return e
	}
	if len(folders) > 0 && (o.options.Change == nil || o.options.Report == nil) {
		return ErrDependency
	}
	if e := o.stop(ctx); e != nil {
		return e
	}
	if e := o.install(ctx); e != nil {
		return e
	}
	value, folders, e := o.prepare(ctx, input)
	if e != nil {
		return e
	}
	return o.activate(ctx, value, folders)
}
func (o *Owner) Stop(ctx context.Context) error {
	if e := o.enter(ctx); e != nil {
		return e
	}
	defer o.leave()
	return o.stop(ctx)
}
func (o *Owner) Close(ctx context.Context) error {
	select {
	case o.gate <- struct{}{}:
		defer o.leave()
	case <-ctx.Done():
		return ctx.Err()
	}
	o.closed = true
	return o.stop(ctx)
}
