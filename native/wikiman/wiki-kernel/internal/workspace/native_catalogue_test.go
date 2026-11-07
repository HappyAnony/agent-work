//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/internal/catalogue"
)

func TestNativeCatalogueRealFilesOwnerCacheAndNoPublication(t *testing.T) {
	root := infoSource(t)
	options := lifecycleOptions(root)
	counts := map[string]*atomic.Int32{pageA: {}, pageB: {}}
	for _, page := range []string{pageA, pageB} {
		dir := filepath.Join(root, "pages", page, "data", "widgets", "owned")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(catalogue.Package{Name: "owned", Version: "1.0.0", URL: "https://github.com/owner/repo", DisplayName: &catalogue.DisplayName{Default: page}, Description: &catalogue.Description{Default: "owned"}})
		if e := os.WriteFile(filepath.Join(dir, "widget.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "README.md"), []byte(page), 0600); e != nil {
			t.Fatal(e)
		}
	}
	options.CatalogueOptions = func(store, page string) (catalogue.Options, error) {
		return catalogue.Options{Version: "3.0.0", OSS: "https://catalogue.invalid", Statistics: "https://catalogue.invalid", VersionURL: "https://catalogue.invalid/version", Now: time.Now, Fetch: func(c context.Context, u string) ([]byte, error) {
			counts[page].Add(1)
			if e := c.Err(); e != nil {
				return nil, e
			}
			switch {
			case strings.HasSuffix(u, "/version"):
				return []byte(`{"bazaar":"revision"}`), nil
			case strings.Contains(u, "/stage/"):
				raw, _ := json.Marshal(catalogue.StageIndex{Repos: []*catalogue.StageRepo{{URL: "owner/repo@commit", Package: &catalogue.StagePackage{}}}})
				return raw, nil
			case strings.HasSuffix(u, "/bazaar/index.json"):
				return []byte(`{"owner/repo":{"downloads":1}}`), nil
			default:
				raw, _ := json.Marshal(catalogue.Package{Name: "owned", Version: "2.0.0", URL: "https://github.com/owner/repo", DisplayName: &catalogue.DisplayName{Default: page}, Description: &catalogue.Description{Default: "owned"}})
				return raw, nil
			}
		}}, nil
	}
	m := open(t, root, options)
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		q := catalogue.Request{Action: "remote", Kind: "widgets"}
		got, e := l.NativeCatalogue(context.Background(), q)
		if e != nil || len(got.Items) != 1 || got.Items[0].PreferredName != page || !got.Items[0].Outdated {
			t.Fatal(got, e)
		}
		count := counts[page].Load()
		got.Items[0].Name = "mutated caller value"
		got, e = l.NativeCatalogue(context.Background(), q)
		if e != nil || got.Items[0].Name != "owned" || counts[page].Load() != count {
			t.Fatal("owner cache/value isolation", got, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("catalogue reads changed source")
	}
}

func TestNativeCatalogueDrainCancelsAndJoinsRequest(t *testing.T) {
	root := infoSource(t)
	options := lifecycleOptions(root)
	entered := make(chan struct{}, 1)
	var active atomic.Int32
	options.CatalogueOptions = func(string, string) (catalogue.Options, error) {
		return catalogue.Options{Version: "3.0.0", OSS: "https://catalogue.invalid", Statistics: "https://catalogue.invalid", VersionURL: "https://catalogue.invalid/version", Now: time.Now, Fetch: func(c context.Context, _ string) ([]byte, error) {
			active.Add(1)
			defer active.Add(-1)
			entered <- struct{}{}
			<-c.Done()
			return nil, c.Err()
		}}, nil
	}
	m := open(t, root, options)
	l := getDocLease(t, m, pageA)
	store := l.StoreID()
	done := make(chan error, 1)
	go func() {
		_, e := l.NativeCatalogue(context.Background(), catalogue.Request{Action: "remote", Kind: "widgets"})
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("catalogue did not start")
	}
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := m.DrainStore(ctx, store); e != nil {
		t.Fatal(e)
	}
	if e := <-done; !errors.Is(e, context.Canceled) || active.Load() != 0 {
		t.Fatal("request survived owner drain", e, active.Load())
	}
}
