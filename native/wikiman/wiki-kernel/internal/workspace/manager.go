// Package workspace owns explicit, read-only workspace lifetimes. It has no
// dependency on the legacy runtime, process-wide paths, or background workers.
package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"sync"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/appearance"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/publishsvc"

	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
)

var (
	ErrIdentity  = errors.New("invalid_workspace_identity")
	ErrAmbiguous = errors.New("ambiguous_store_root")
	ErrClosed    = errors.New("workspace_closed")
	ErrBudget    = errors.New("workspace_budget_exceeded")
	ErrStale     = errors.New("stale_workspace_lease")
)

// StoreContext pins a validated, immutable metadata snapshot to a canonical root.
// Source locks, recovery admission and refresh transactions use sourcetx.
type StoreContext struct {
	layout           wikistore.Layout
	tree             *wikistore.TreeSnapshot
	root             *os.Root
	tracker          *wikistore.Tracker
	watch            *wikistore.Watch
	watchCtx         context.Context
	draining         bool
	source           *sourcetx.Store
	mutating         bool
	requests         int
	writer           *sourcetx.Writer
	recoveryRequired bool
	writeQueue       chan struct{}
}

type WorkspaceContext struct {
	store      *StoreContext
	paths      wikistore.WorkspacePaths
	data       *os.Root
	resources  *resources
	generation uint64
	refs       int
	tasks      int
	taskOwners map[*Task]context.CancelFunc
	lastUsed   time.Time
}

type Options struct {
	// ReadDefaults explicitly opts native handlers into pure original read settings
	// and source-bound notebook/language readers; no device config or workers.
	ReadDefaults bool

	MaxWorkspaces        int
	IdleTimeout          time.Duration
	MaxTasksPerWorkspace int
	// Configuration supplies complete original settings for an explicit identity.
	// It must be bounded, side-effect free and must not reenter this manager.
	// No provider means an empty snapshot, never implicit defaults/environment IO.
	Configuration func(storeID, pageID string) (conf.AppData, error)
	// NativeContent supplies owner-specific OCR/AV readers. Callbacks are bounded,
	// run under this owner's gate/admission, and must not reenter the manager.
	// Missing dependencies fail when used; no global fallback or implicit OCR boot.
	NativeContent func(storeID, pageID string) (indexdb.ContentServices, error)
	// NativeOCR binds synchronous owner-local preparation; no source writes or unowned workers.
	NativeOCR func(storeID, pageID string) (func(context.Context, *ast.Node) error, error)
	// ConfigurationBusiness binds original notebook/language services to an
	// identity. Readers must be source-specific and must not reenter the manager.
	ConfigurationBusiness func(storeID, pageID string) (*conf.Business, error)
	// SettingServices supplies trusted bounded device-tool probes. No source JSON
	// creates these capabilities; callbacks must not reenter this manager.
	SettingServices func(storeID, pageID string) (conf.SettingServices, error)
	// PublishOptions binds a device listener/upstream and auth ports for this
	// identity. Construction starts no listener; callbacks must be bounded.
	PublishOptions    func(storeID, pageID string) (publishsvc.Options, error)
	AppearanceOptions func(storeID, pageID string) (appearance.Options, error)
	CatalogueOptions  func(storeID, pageID string) (catalogue.Options, error)
	// CloudAccountOptions supplies private device authority; never wiki source storage.
	CloudAccountOptions func(storeID, pageID string) (cloudaccount.Options, error)
	CloudRepoOptions    func(storeID, pageID string) (cloudrepo.Options, error)
	AIOptions           func(storeID, pageID string) (aiflow.Options, error)
}
type key struct{ store, page string }
type Stats struct {
	Stores, Workspaces, Leases, Tasks, Watchers int
	Draining                                    bool
}

type WorkspaceManager struct {
	mu         sync.Mutex
	stores     map[string]*StoreContext
	roots      map[string]string
	workspaces map[key]*WorkspaceContext
	options    Options
	next       uint64
	draining   bool
	changed    chan struct{}
}

func NewManager(options Options) *WorkspaceManager {
	if options.MaxWorkspaces <= 0 {
		options.MaxWorkspaces = 8
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = 5 * time.Minute
	}
	if options.MaxTasksPerWorkspace <= 0 {
		options.MaxTasksPerWorkspace = 8
	}
	return &WorkspaceManager{stores: map[string]*StoreContext{}, roots: map[string]string{}, workspaces: map[key]*WorkspaceContext{}, options: options, changed: make(chan struct{})}
}

// OpenStore is an explicit registration operation, not a request-controlled path.
// Re-registering a real-path alias is idempotent; two clones with one storeId fail.
func (m *WorkspaceManager) OpenStore(ctx context.Context, root string) (string, error) {
	if root == "" {
		return "", ErrIdentity
	}
	layout, err := wikistore.OpenLayout(root)
	if err != nil {
		return "", err
	}
	tracker := wikistore.NewTracker(layout.Root, wikistore.Scanner{})
	source, err := sourcetx.Open(layout.Root)
	if err != nil {
		return "", err
	}
	var tree *wikistore.TreeSnapshot
	err = source.Read(ctx, func() error { var err error; tree, err = tracker.Refresh(ctx); return err })
	if err != nil {
		return "", err
	}
	id := tree.Store().StoreID
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if m.draining {
		return "", ErrClosed
	}
	if old, ok := m.roots[layout.Root]; ok && old != id {
		return "", ErrAmbiguous
	}
	if old, ok := m.stores[id]; ok {
		if old.draining || old.recoveryRequired {
			return "", ErrClosed
		}
		if old.layout.Root != layout.Root {
			return "", ErrAmbiguous
		}
		if old.tree.SourceDigest() != tree.SourceDigest() || old.tree.StoreDigest() != tree.StoreDigest() {
			return "", ErrStale
		}
		return id, nil
	}
	sourceRoot, err := os.OpenRoot(layout.Root)
	if err != nil {
		return "", err
	}
	store := &StoreContext{layout: layout, tree: tree, root: sourceRoot, tracker: tracker, source: source, writeQueue: make(chan struct{}, 1)}
	store.writeQueue <- struct{}{}
	m.stores[id] = store
	m.roots[layout.Root] = id
	return id, nil
}

// Lease pins an identity and generation for the request. Retain must be called
// before handing work to an asynchronous task; each owner releases its own lease.
type Lease struct {
	manager   *WorkspaceManager
	workspace *WorkspaceContext
	released  bool
}

func (m *WorkspaceManager) Acquire(ctx context.Context, storeID, pageID string) (*Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.draining && !m.admitted(ctx, m.stores[storeID]) {
		return nil, ErrClosed
	}
	s := m.stores[storeID]
	if storeID == "" || pageID == "" || s == nil {
		return nil, ErrIdentity
	}
	if admission, ok := ctx.Value(readAdmissionKey{}).(*readAdmission); ok && admission.manager == m && !m.admitted(ctx, s) {
		return nil, ErrClosed
	}
	if (s.draining || s.mutating) && !m.admitted(ctx, s) || s.recoveryRequired {
		return nil, ErrClosed
	}
	if _, err := s.tracker.View(); err != nil {
		return nil, ErrStale
	}
	page, ok := s.tree.Get(pageID)
	if !ok || page.DeletedAt != nil {
		return nil, ErrIdentity
	}
	k := key{storeID, pageID}
	w := m.workspaces[k]
	if w == nil {
		if len(m.workspaces) >= m.options.MaxWorkspaces {
			var oldest key
			var candidate *WorkspaceContext
			for candidateKey, current := range m.workspaces {
				if current.refs == 0 && (candidate == nil || current.lastUsed.Before(candidate.lastUsed)) {
					oldest, candidate = candidateKey, current
				}
			}
			if candidate == nil {
				return nil, ErrBudget
			}
			if err := m.closeWorkspace(oldest, candidate); err != nil {
				return nil, err
			}
		}
		paths, err := s.layout.Workspace(pageID)
		if err != nil {
			return nil, err
		}
		// Open through the pinned source root. Never follow an untrusted page
		// directory to another workspace, even when it remains inside this store.
		for _, name := range []string{"pages", filepath.Join("pages", pageID)} {
			info, err := s.root.Lstat(name)
			if err != nil {
				return nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, ErrIdentity
			}
		}
		relative := filepath.Join("pages", pageID, "data")
		info, statErr := s.root.Lstat(relative)
		if statErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return nil, ErrIdentity
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		// Root confines all subsequent document reads, including symlink resolution.
		data, err := s.root.OpenRoot(relative)
		if errors.Is(err, os.ErrNotExist) {
			data, err = nil, nil
		}
		if err != nil {
			return nil, err
		}
		var configuration conf.AppData
		if m.options.ReadDefaults {
			configuration = defaultReadConfiguration()
		}
		if provider := m.options.Configuration; provider != nil {
			configuration, err = provider(storeID, pageID)
			if err == nil {
				configuration, err = configuration.Clone()
			}
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var business *conf.Business
		configuration, err = s.source.Configuration(ctx, pageID, configuration)
		if err != nil {
			if data != nil {
				_ = data.Close()
			}
			return nil, err
		}
		if provider := m.options.ConfigurationBusiness; provider != nil {
			business, err = provider(storeID, pageID)
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var settingServices conf.SettingServices
		if provider := m.options.SettingServices; provider != nil {
			settingServices, err = provider(storeID, pageID)
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var catalogueOptions catalogue.Options
		if provider := m.options.CatalogueOptions; provider != nil {
			catalogueOptions, err = provider(storeID, pageID)
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var content indexdb.ContentServices
		if settingServices.ValidateSyncLocal == nil {
			workspaceRoot := s.layout.Root
			settingServices.ValidateSyncLocal = func(ctx context.Context, endpoint string) error {
				return conf.ValidateSyncLocal(ctx, endpoint, workspaceRoot)
			}
		}
		if provider := m.options.NativeContent; provider != nil {
			content, err = provider(storeID, pageID)
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var ocr func(context.Context, *ast.Node) error
		if provider := m.options.NativeOCR; provider != nil {
			ocr, err = provider(storeID, pageID)
			if err != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, err
			}
		}
		var publish *publishsvc.Service
		if provider := m.options.PublishOptions; provider != nil {
			opts, e := provider(storeID, pageID)
			if e == nil {
				publish, e = publishsvc.New(opts)
			}
			if e != nil {
				if data != nil {
					_ = data.Close()
				}
				return nil, e
			}
		}
		var appearanceOwner *appearance.Owner
		if provider := m.options.AppearanceOptions; provider != nil {
			opts, e := provider(storeID, pageID)
			if e == nil {
				appearanceOwner, e = appearance.New(opts)
			}
			if e == nil && settingServices.NormalizeAppearance != nil {
				e = appearance.ErrDependency
			}
			if e != nil {
				if publish != nil {
					_ = publish.Close(context.Background())
				}
				if data != nil {
					_ = data.Close()
				}
				return nil, e
			}
			settingServices.NormalizeAppearance = appearanceOwner.Prepare
		}
		var cloud *cloudaccount.Service
		if provider := m.options.CloudAccountOptions; provider != nil {
			opts, e := provider(storeID, pageID)
			if e == nil {
				cloud, e = cloudaccount.New(opts)
			}
			if e != nil {
				if appearanceOwner != nil {
					_ = appearanceOwner.Close(context.Background())
				}
				if publish != nil {
					_ = publish.Close(context.Background())
				}
				if data != nil {
					_ = data.Close()
				}
				return nil, e
			}
		}
		resources, err := openResources(ctx)
		if err != nil {
			if cloud != nil {
				_ = cloud.Close(context.Background())
			}
			if appearanceOwner != nil {
				_ = appearanceOwner.Close(context.Background())
			}
			if publish != nil {
				_ = publish.Close(context.Background())
			}
			if data != nil {
				data.Close()
			}
			return nil, err
		}
		m.next++
		resources.configuration = bindConfigurationPaths(configuration, paths)
		resources.configurationBusiness = business
		resources.settingServices = settingServices
		resources.catalogueOptions = catalogueOptions
		resources.nativeContent = content
		resources.nativeOCR = ocr
		resources.publish = publish
		resources.appearance = appearanceOwner
		resources.cloudAccount = cloud
		if provider := m.options.AIOptions; provider != nil {
			opts, e := provider(storeID, pageID)
			if e == nil {
				if opts.Load == nil {
					opts.Load = func(c context.Context) (out aiflow.Settings, err error) {
						err = resources.serial(c, func() error {
							cfg := resources.configuration
							if cfg.AI == nil || cfg.AI.OpenAI == nil {
								return aiflow.ErrConfig
							}
							out.API = *cfg.AI.OpenAI
							if cfg.Editor != nil && cfg.Editor.Markdown != nil {
								md := *cfg.Editor.Markdown
								out.Markdown = &md
							}
							return nil
						})
						return
					}
				}
				resources.ai, e = aiflow.New(opts)
			}
			if e != nil {
				_ = resources.close()
				if data != nil {
					_ = data.Close()
				}
				return nil, e
			}
		}
		if provider := m.options.CloudRepoOptions; provider != nil {
			opts, e := provider(storeID, pageID)
			if e == nil {
				if opts.Load == nil {
					opts.Load = func(c context.Context) (out conf.AppData, err error) {
						err = resources.serial(c, func() error { out, err = resources.configuration.Clone(); return err })
						return
					}
				}
				resources.cloudRepo, e = cloudrepo.New(opts)
			}
			if e != nil {
				_ = resources.close()
				if data != nil {
					_ = data.Close()
				}
				return nil, e
			}
		}
		w = &WorkspaceContext{store: s, paths: paths, data: data, resources: resources, generation: m.next}
		if m.options.ReadDefaults {
			bindNativeReadServices(w)
		}
		m.workspaces[k] = w
	}
	w.refs++
	return &Lease{manager: m, workspace: w}, nil
}

func (l *Lease) Retain() (*Lease, error) {
	return l.pin(false)
}

// Existing requests may finish reading after Drain; new derived tasks may not.
func (l *Lease) pin(existingRequest bool) (*Lease, error) {
	m := l.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.released {
		return nil, ErrStale
	}
	if (m.draining || l.workspace.store.draining || l.workspace.store.mutating || l.workspace.store.recoveryRequired) && !existingRequest {
		return nil, ErrClosed
	}
	l.workspace.refs++
	return &Lease{manager: m, workspace: l.workspace}, nil
}

func (l *Lease) Generation() uint64 { return l.workspace.generation }
func (l *Lease) StoreID() string    { return l.workspace.store.tree.Store().StoreID }
func (l *Lease) PageID() string     { return l.workspace.paths.PageID }

func (l *Lease) Release() {
	m := l.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.released {
		return
	}
	l.released = true
	l.workspace.refs--
	l.workspace.lastUsed = time.Now()
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *WorkspaceManager) closeWorkspace(k key, w *WorkspaceContext) error {
	err := w.resources.close()
	if w.data != nil {
		err = errors.Join(err, w.data.Close())
	}
	delete(m.workspaces, k)
	return err
}

// EvictIdle is explicit: no ticker or unowned goroutine survives a CLI request.
func (m *WorkspaceManager) EvictIdle(now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k, w := range m.workspaces {
		if w.refs == 0 && now.Sub(w.lastUsed) >= m.options.IdleTimeout {
			if err := m.closeWorkspace(k, w); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func (m *WorkspaceManager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Stats{Stores: len(m.stores), Workspaces: len(m.workspaces), Draining: m.draining}
	for _, w := range m.workspaces {
		s.Leases += w.refs
		s.Tasks += w.tasks
	}
	for _, store := range m.stores {
		if store.watch != nil {
			select {
			case <-store.watch.Done():
			default:
				s.Watchers++
			}
		}
	}
	return s
}

// WatchStore explicitly starts one advisory metadata watcher owned by the store.
// Events invalidate admission; transactional refresh is implemented in S04.
func (m *WorkspaceManager) WatchStore(ctx context.Context, storeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.draining {
		return ErrClosed
	}
	s := m.stores[storeID]
	if s == nil {
		return ErrIdentity
	}
	if s.draining || s.mutating || s.recoveryRequired {
		return ErrClosed
	}
	if s.watch != nil {
		select {
		case <-s.watch.Done():
		default:
			return nil
		}
	}
	watch, err := s.tracker.StartWatch(ctx)
	if err != nil {
		return err
	}
	s.watch = watch
	s.watchCtx = ctx
	return nil
}

// DrainStore releases one registered source, not the server/manager or any other
// source. Keeping its gate on timeout prevents reopening it behind the owner.
func (m *WorkspaceManager) DrainStore(ctx context.Context, storeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stores[storeID]
	if s == nil {
		return ErrIdentity
	}
	s.draining = true
	for _, w := range m.workspaces {
		if w.store == s {
			w.cancelTasks()
		}
	}
	for {
		active := s.mutating || s.requests > 0
		for _, w := range m.workspaces {
			if w.store == s && w.refs > 0 {
				active = true
				break
			}
		}
		if !active {
			break
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			m.mu.Lock()
			return ctx.Err()
		case <-changed:
		}
		m.mu.Lock()
	}
	if m.stores[storeID] != s {
		return nil
	} // Another draining owner already closed this registration.
	for k, w := range m.workspaces {
		if w.store == s {
			if err := m.closeWorkspace(k, w); err != nil {
				return err
			}
		}
	}
	if s.watch != nil {
		s.watch.Close()
	}
	if s.writer != nil {
		if err := s.writer.CloseContext(ctx); err != nil {
			return err
		}
		s.writer = nil
	}
	if err := s.root.Close(); err != nil {
		return err
	}
	delete(m.stores, storeID)
	delete(m.roots, s.layout.Root)
	return nil
}

// Drain stops new requests/tasks and waits for existing owners. Timeout leaves
// the manager draining with its handles intact; a later Drain can finish safely.
func (m *WorkspaceManager) Drain(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.draining = true
	for _, w := range m.workspaces {
		w.cancelTasks()
	}
	for {
		active := false
		for _, s := range m.stores {
			if s.mutating || s.requests > 0 {
				active = true
				break
			}
		}
		for _, w := range m.workspaces {
			if w.refs > 0 {
				active = true
				break
			}
		}
		if !active {
			break
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			m.mu.Lock()
			return ctx.Err()
		case <-changed:
		}
		m.mu.Lock()
	}
	for k, w := range m.workspaces {
		if err := m.closeWorkspace(k, w); err != nil {
			return err
		}
	}
	for id, s := range m.stores {
		if s.watch != nil {
			s.watch.Close()
		}
		if s.writer != nil {
			if err := s.writer.CloseContext(ctx); err != nil {
				return err
			}
			s.writer = nil
		}
		if err := s.root.Close(); err != nil {
			return err
		}
		delete(m.stores, id)
	}
	clear(m.stores)
	clear(m.roots)
	return nil
}
