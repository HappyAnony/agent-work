package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"wiki-kernel/conf"
	"wiki-kernel/internal/wikiops"
)

// Assets are trusted, read-only application resources, never a source root or
// its parent. Do not use FileServer: no directory indexes, hidden files, source
// downloads or symlink traversal are part of the portable editor contract.
type editorAssets struct{ root *os.Root }

func openEditorAssets(appDir, sourceDir string) (*editorAssets, error) {
	app, err := filepath.EvalSymlinks(appDir)
	if err != nil {
		return nil, err
	}
	source, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return nil, err
	}
	app, _ = filepath.Abs(app)
	source, _ = filepath.Abs(source)
	within := func(a, b string) bool {
		rel, e := filepath.Rel(a, b)
		return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if within(app, source) || within(source, app) {
		return nil, fs.ErrPermission
	}
	root, err := os.OpenRoot(app)
	if err != nil {
		return nil, err
	}
	a := &editorAssets{root: root}
	for _, name := range []string{"stage/build/portable/index.html", "stage/build/portable/boot.js", "stage/protyle/js/lute/lute.min.js", "appearance/langs/en_US.json"} {
		f, e := a.open(name)
		if e != nil {
			root.Close()
			return nil, e
		}
		f.Close()
	}
	return a, nil
}
func (a *editorAssets) open(name string) (*os.File, error) {
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if part == "" || part == "." || strings.HasPrefix(part, ".") || strings.Contains(part, "\\") {
			return nil, fs.ErrPermission
		}
		info, err := a.root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fs.ErrPermission
		}
	}
	f, err := a.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fs.ErrPermission
	}
	return f, nil
}

var themeDirectory = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,95}$`)

func (a *editorAssets) themes() []conf.AppearanceThemeRecord {
	if a == nil {
		return nil
	}
	const base = "appearance/themes"
	entries, err := fs.ReadDir(a.root.FS(), base)
	if err != nil || len(entries) > 128 {
		return nil
	}
	themes := []conf.AppearanceThemeRecord{}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !themeDirectory.MatchString(name) {
			continue
		}
		css, err := a.open(path.Join(base, name, "theme.css"))
		if err != nil {
			continue
		}
		_ = css.Close()
		meta, err := a.open(path.Join(base, name, "theme.json"))
		if err != nil {
			continue
		}
		var info struct {
			Version     string
			Modes       []string
			DisplayName struct {
				ZhCN string `json:"zh_CN"`
			}
		}
		err = json.NewDecoder(io.LimitReader(meta, 64<<10)).Decode(&info)
		_ = meta.Close()
		if err == nil && len(info.Version) <= 128 && len(info.DisplayName.ZhCN) <= 256 && !strings.ContainsAny(info.DisplayName.ZhCN, "<>&\"'") {
			themes = append(themes, conf.AppearanceThemeRecord{Name: name, Version: info.Version,
				Modes: info.Modes, ChineseName: info.DisplayName.ZhCN})
		}
	}
	return themes
}

func (a *editorAssets) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	const plantUML = "stage/protyle/js/plantuml/"
	const widgetFrame = "stage/protyle/js/widget/"
	const ancestors = "frame-ancestors http://127.0.0.1:* http://localhost:* tauri://localhost;"
	frameURL := "http://" + r.Host + "/" + plantUML + "scoped.html http://" + r.Host + "/" + widgetFrame + "scoped.html"
	policy := "default-src 'none'; script-src 'self' 'unsafe-eval'; worker-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; media-src blob:; font-src 'self'; connect-src 'self' blob: ws://" + r.Host + "; " + ancestors + " frame-src " + frameURL + "; base-uri 'none'; form-action 'none'; object-src 'none'"
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == plantUML+"scoped.html" {
		policy = "default-src 'none'; sandbox allow-scripts; script-src 'wasm-unsafe-eval'"
		for _, script := range []string{"scoped.js", "plantuml.js", "viz-global.js", "emoji.js", "openiconic.js", "themes.js"} {
			policy += " http://" + r.Host + "/" + plantUML + script
		}
		policy += "; style-src 'unsafe-inline'; connect-src 'none'; base-uri 'none'; " + ancestors
	}
	if name == widgetFrame+"scoped.html" {
		policy = "default-src 'none'; sandbox allow-scripts; script-src 'unsafe-eval' http://" + r.Host + "/" + widgetFrame +
			"scoped.js http://" + r.Host + wikiops.SandboxScriptsPath +
			"; style-src 'unsafe-inline'; img-src data:; media-src data:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'; " + ancestors
	}
	w.Header().Set("Content-Security-Policy", policy)
	if name == plantUML+"plantuml.js" {
		w.Header().Set("Access-Control-Allow-Origin", "null")
	}
	if name == "stage/build/portable/" {
		name += "index.html"
	}
	// The native editor uses this bundled loading icon during normal editing.
	// Permit the exact trusted file, not arbitrary /stage content or source SVG.
	allowed := name == "stage/loading-pure.svg"
	for _, prefix := range []string{"stage/build/portable/", "stage/protyle/", "appearance/", "emojis/"} {
		allowed = allowed || strings.HasPrefix(name, prefix)
	}
	if !allowed || path.Clean(name) != name || strings.HasSuffix(name, ".map") {
		http.NotFound(w, r)
		return
	}
	f, err := a.open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, path.Base(name), info.ModTime(), f)
}

func portableHandler(api http.Handler, a *editorAssets, address string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject DNS rebinding for static UI too, not only authenticated APIs.
		if r.Host != address {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if a != nil && !strings.HasPrefix(r.URL.Path, "/v1/") {
			if strings.HasPrefix(r.URL.Path, wikiops.SandboxScriptsPath) {
				wikiops.ServeSandboxScript(r.Context(), w, r)
				return
			}
			a.serve(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
