package wikistore

import (
	"fmt"
	"path/filepath"
)

// Layout describes canonical paths without creating directories. Runtime
// initialization and lock ownership belong to StoreContext, not the scanner.
type Layout struct{ Root string }

func OpenLayout(root string) (Layout, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Layout{}, err
	}
	realRoot, err = filepath.Abs(realRoot)
	return Layout{Root: realRoot}, err
}

func (l Layout) IndexDir() string   { return filepath.Join(l.Root, ".local", "index") }
func (l Layout) JournalDir() string { return filepath.Join(l.Root, ".local", "journal") }
func (l Layout) LocksDir() string   { return filepath.Join(l.Root, ".local", "locks") }
func (l Layout) PagesDir() string   { return filepath.Join(l.Root, "pages") }

type WorkspacePaths struct {
	PageID     string
	Metadata   string
	DataDir    string
	RuntimeDir string
	ConfDir    string
	TempDir    string
}

func (l Layout) Workspace(id string) (WorkspacePaths, error) {
	if l.Root == "" || !filepath.IsAbs(l.Root) || !pageIDPattern.MatchString(id) {
		return WorkspacePaths{}, fmt.Errorf("invalid workspace identity")
	}
	runtimeDir := filepath.Join(l.Root, ".local", "workspaces", id)
	return WorkspacePaths{PageID: id, Metadata: filepath.Join(l.PagesDir(), id, "page.json"), DataDir: filepath.Join(l.PagesDir(), id, "data"), RuntimeDir: runtimeDir, ConfDir: filepath.Join(runtimeDir, "conf"), TempDir: filepath.Join(runtimeDir, "temp")}, nil
}
