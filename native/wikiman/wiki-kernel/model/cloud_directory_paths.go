// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

// Resolve existing symlink ancestors even when a configured directory is absent.
func canonicalDirectoryPath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", cloudrepo.ErrDependency
	}
	p = filepath.Clean(p)
	var tail []string
	for {
		resolved, e := filepath.EvalSymlinks(p)
		if e == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", e
		}
		tail = append(tail, filepath.Base(p))
		p = parent
	}
}
func directoryContains(parent, child string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func validateLocalDirectoryTarget(v conf.AppData, p metadataRepoPaths) error {
	if v.Sync == nil || v.Sync.Provider != conf.ProviderLocal {
		return nil
	}
	if v.Sync.Local == nil {
		return cloudrepo.ErrDependency
	}
	endpoint, e := canonicalDirectoryPath(v.Sync.Local.Endpoint)
	if e != nil {
		return cloudrepo.ErrDependency
	}
	for _, root := range []string{p.Workspace, p.Data, p.Repo, p.History, p.Temp} {
		if root == "" {
			continue
		}
		resolved, e := canonicalDirectoryPath(root)
		if e != nil {
			return cloudrepo.ErrDependency
		}
		if directoryContains(endpoint, resolved) || directoryContains(resolved, endpoint) {
			return cloudrepo.ErrDependency
		}
	}
	return nil
}
