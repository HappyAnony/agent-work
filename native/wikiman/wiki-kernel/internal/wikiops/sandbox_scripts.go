// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	nethtml "golang.org/x/net/html"
)

const SandboxScriptsPath = "/stage/protyle/js/widget/resources/"

var scriptPart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

func validSandboxScript(name string) bool {
	if len(name) > 512 || path.Clean(name) != name || path.Ext(name) != ".js" && path.Ext(name) != ".mjs" {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if !scriptPart.MatchString(part) {
			return false
		}
	}
	return true
}

type sandboxScriptGrant struct {
	expires time.Time
	files   map[string][]byte
}

type sandboxScripts struct {
	mu     sync.Mutex
	grants map[string]sandboxScriptGrant
}

// Native module loading resolves imports relative to each issued resource,
// including modules that were inline in the package's index document.
func (s *sandboxScripts) prepare(html string, files map[string][]byte) (string, string, error) {
	return s.prepareAt(html, files, ".")
}

func (s *sandboxScripts) prepareAt(html string, files map[string][]byte, dir string) (string, string, error) {
	root, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		return "", "", err
	}
	if files == nil {
		files = make(map[string][]byte)
	}
	count := 0
	err = visitSandboxScripts(root, func(node *nethtml.Node) error {
		var module, external bool
		for _, attr := range node.Attr {
			module = module || attr.Key == "type" && attr.Val == "module"
			external = external || attr.Key == "src"
		}
		if !module || external {
			return nil
		}
		if count == 32 {
			return ErrExportLimit
		}
		var name string
		for {
			name = path.Join(dir, fmt.Sprintf("__wiki_inline_%d.mjs", count))
			count++
			if _, exists := files[name]; !exists {
				break
			}
			if count == 32 {
				return ErrExportLimit
			}
		}
		if !validSandboxScript(name) {
			return ErrArgument
		}
		var code strings.Builder
		for node.FirstChild != nil {
			child := node.FirstChild
			code.WriteString(child.Data)
			node.RemoveChild(child)
		}
		files[name] = []byte(code.String())
		node.Attr = append(node.Attr, nethtml.Attribute{Key: "src", Val: name})
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if count > 0 {
		var out strings.Builder
		if err = nethtml.Render(&out, root); err != nil {
			return "", "", err
		}
		html = out.String()
	}
	base, err := s.issue(files)
	return html, base, err
}

func visitSandboxScripts(root *nethtml.Node, fn func(*nethtml.Node) error) error {
	if root.Type == nethtml.ElementNode && root.Namespace == "" && root.Data == "script" {
		return fn(root)
	}
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := visitSandboxScripts(node, fn); err != nil {
			return err
		}
	}
	return nil
}

func (s *sandboxScripts) issue(files map[string][]byte) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	var key [24]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(key[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grants == nil {
		s.grants = make(map[string]sandboxScriptGrant)
	}
	now := time.Now()
	for id, grant := range s.grants {
		if !grant.expires.After(now) {
			delete(s.grants, id)
		}
	}
	if len(s.grants) >= 24 {
		return "", ErrExportLimit
	}
	s.grants[token] = sandboxScriptGrant{expires: now.Add(2 * time.Minute), files: files}
	return SandboxScriptsPath + token + "/", nil
}

// ServeSandboxScript serves only a verified byte snapshot. The capability
// confers no editor, workspace, or source read permissions.
func ServeSandboxScript(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	token, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, SandboxScriptsPath), "/")
	if p == nil || !ok || !validSandboxScript(name) || len(token) != 48 ||
		r.Method != http.MethodGet || r.Header.Get("Origin") != "null" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	p.scripts.mu.Lock()
	grant := p.scripts.grants[token]
	data, exists := grant.files[name]
	p.scripts.mu.Unlock()
	if !exists || !grant.expires.After(time.Now()) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "null")
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(data)
}
