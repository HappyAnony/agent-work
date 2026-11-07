package catalogue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func catalogueFixture(t *testing.T) (Options, *Files) {
	t.Helper()
	base := t.TempDir()
	roots := map[string]string{}
	for _, kind := range []string{"plugins", "widgets", "icons", "themes", "templates"} {
		roots[kind] = filepath.Join(base, kind)
		dir := filepath.Join(roots[kind], kind+"-one")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		item := Item{Package: &Package{Name: kind + "-one", Version: "1.0.0", URL: "https://github.com/owner/" + kind, DisplayName: &DisplayName{Default: "Default", ZhCN: "中文"}, Description: &Description{Default: "needle"}, Readme: &Readme{Default: "README.md"}}, Modes: []string{"light", "dark"}}
		raw, _ := json.Marshal(item)
		if e := os.WriteFile(filepath.Join(dir, filename(kind)), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "README.md"), []byte("[Local](other.md)"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version":
			fmt.Fprint(w, `{"bazaar":"revision"}`)
		case r.URL.Path == "/bazaar/index.json":
			fmt.Fprint(w, `{"owner/plugins":{"downloads":7},"owner/widgets":{"downloads":8},"owner/themes":{"downloads":9}}`)
		case strings.HasPrefix(r.URL.Path, "/bazaar@revision/stage/"):
			kind := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/bazaar@revision/stage/"), ".json")
			json.NewEncoder(w).Encode(StageIndex{Repos: []*StageRepo{{URL: "owner/" + kind + "@commit", Updated: "2026-11-01T12:00:00Z", Size: 100, InstallSize: 200, Package: &StagePackage{Readme: &Readme{Default: "README.md", ZhCN: "missing.md"}, Funding: &Funding{GitHub: "owner"}}}}})
		case strings.HasSuffix(r.URL.Path, "/README.md"):
			fmt.Fprint(w, "[Remote](relative.md)")
		case strings.HasSuffix(r.URL.Path, "/missing.md"):
			http.NotFound(w, r)
		default:
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/package/owner/"), "@")
			if len(parts) != 2 {
				http.NotFound(w, r)
				return
			}
			kind := parts[0]
			json.NewEncoder(w).Encode(Item{Package: &Package{Name: kind + "-one", Version: "2.0.0", URL: "https://github.com/owner/" + kind, DisplayName: &DisplayName{Default: "Default", ZhCN: "中文"}, Description: &Description{Default: "needle"}, Backends: []string{"linux"}, Frontends: []string{"desktop"}}, Modes: []string{"light", "dark"}})
		}
	}))
	t.Cleanup(server.Close)
	files, e := OpenFiles(roots, false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := files.Close(); e != nil {
			t.Error(e)
		}
	})
	return Options{Language: "zh_CN", Version: "3.0.0", Backend: "linux", OSS: server.URL, Statistics: server.URL, VersionURL: server.URL + "/version", Fetch: HTTPFetch(server.Client()), Read: files.Read, List: files.List, Stat: files.Stat, Now: time.Now, ThemeNames: []string{"themes-one"}, PluginEnabled: map[string]bool{"plugins-one": true}}, files
}

func TestCatalogueActualRemoteLocalProjectionAndREADME(t *testing.T) {
	p, files := catalogueFixture(t)
	s := &Service{}
	ctx := context.Background()
	for _, kind := range []string{"plugins", "widgets", "icons", "themes", "templates"} {
		got, e := s.Do(ctx, Request{Action: "remote", Kind: kind, Frontend: "desktop", Keyword: "needle"}, p)
		if e != nil || len(got.Items) != 1 {
			t.Fatal(kind, got, e)
		}
		item := got.Items[0]
		if item.PreferredName != "中文" || !item.Installed || !item.Outdated || item.HUpdated != "2026-11-01" || item.PreferredFunding != "https://github.com/sponsors/owner" || item.Incompatible {
			t.Fatal(item)
		}
		got, e = s.Do(ctx, Request{Action: "installed", Kind: kind, Frontend: "desktop", CheckUpdate: true}, p)
		if e != nil || len(got.Items) != 1 || !got.Items[0].Outdated || got.Items[0].InstallSize < 1 || !strings.Contains(got.Items[0].PreferredReadme, publicBase(kind, kind+"-one")+"other.md") {
			t.Fatal(kind, got, e)
		}
		if kind == "plugins" && !got.Items[0].Enabled {
			t.Fatal("plugin enable metadata omitted")
		}
	}
	got, e := s.Do(ctx, Request{Action: "readme", Kind: "plugins", RepoURL: "https://github.com/owner/plugins", RepoHash: "commit"}, p)
	if e != nil || !strings.Contains(got.README, `target="_blank"`) || !strings.Contains(got.README, "cdn.jsdelivr.net/gh/owner/plugins") {
		t.Fatal(got, e)
	}
	if e = files.Check(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestCatalogueCancellationJoinsEveryActualRequestWorker(t *testing.T) {
	p, _ := catalogueFixture(t)
	original := p.Fetch
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var active atomic.Int32
	p.Fetch = func(c context.Context, u string) ([]byte, error) {
		if strings.HasSuffix(u, "plugin.json") {
			active.Add(1)
			defer active.Add(-1)
			entered <- struct{}{}
			<-release
		}
		return original(c, u)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := (&Service{}).Do(ctx, Request{Action: "remote", Kind: "plugins"}, p); done <- e }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not enter")
	}
	cancel()
	select {
	case e := <-done:
		t.Fatal("reported completion while actual request still owned", e)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) || active.Load() != 0 {
			t.Fatal(e, active.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request workers did not join")
	}
}

func TestCatalogueFilesRejectReplacementAndPreserveExplicitLinkedRoots(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprint(linked), func(t *testing.T) {
			base, foreign := t.TempDir(), t.TempDir()
			if e := os.WriteFile(filepath.Join(foreign, "widget.json"), []byte(`{"name":"linked"}`), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(foreign, filepath.Join(base, "linked")); e != nil {
				t.Fatal(e)
			}
			files, e := OpenFiles(map[string]string{"widgets": base}, linked)
			if e != nil {
				t.Fatal(e)
			}
			defer files.Close()
			raw, e := files.Read(context.Background(), "widgets", "linked", "widget.json")
			if !linked {
				if e == nil {
					t.Fatal("undeclared link accepted")
				}
				return
			}
			if e != nil || !strings.Contains(string(raw), "linked") {
				t.Fatal(string(raw), e)
			}
			if e = os.WriteFile(filepath.Join(foreign, "widget.json"), []byte(`{"name":"changed"}`), 0600); e != nil {
				t.Fatal(e)
			}
			if e = files.Check(context.Background()); e == nil {
				t.Fatal("read metadata replacement accepted")
			}
		})
	}
}
