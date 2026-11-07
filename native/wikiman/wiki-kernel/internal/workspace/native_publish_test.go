//go:build fts5

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/publishsvc"
)

func TestNativePublishActualSourceIsolationAndEvictionClose(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	ctx := context.Background()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		page := strings.Trim(q.URL.Path, "/")
		if q.Header.Get(publishsvc.TokenHeader) != page {
			http.Error(w, "foreign identity", 403)
			return
		}
		raw, e := os.ReadFile(filepath.Join(root, "pages", page, "data", boxID, docID+".docj"))
		if e != nil {
			http.Error(w, e.Error(), 404)
			return
		}
		w.Write(raw)
	}))
	defer backend.Close()
	m := open(t, root, Options{Configuration: func(string, string) (conf.AppData, error) {
		p := conf.NewPublish()
		p.Enable = true
		p.Port = 0
		p.Auth.Enable = false
		return conf.AppData{Publish: p}, nil
	}, PublishOptions: func(_, page string) (publishsvc.Options, error) {
		target, _ := url.Parse(backend.URL + "/" + page)
		return publishsvc.Options{Host: "127.0.0.1", Target: *target, Transport: http.DefaultTransport.(*http.Transport).Clone(), Auth: publishsvc.AuthPorts{
			Configure: func(*conf.Publish) error { return nil }, Account: func(string) (publishsvc.Account, bool) { return publishsvc.Account{Token: page}, true }, Session: func(string) string { return "" }, NewSession: func() string { return "test" }, AddSession: func(string, string) {}, DeleteSession: func(string) {},
		}}, nil
	}})
	a := getDocLease(t, m, pageA)
	b := getDocLease(t, m, pageB)
	pa, e := a.StartNativePublish(ctx)
	if e != nil {
		t.Fatal(e)
	}
	pb, e := b.StartNativePublish(ctx)
	if e != nil || pa == pb {
		t.Fatal(pa, pb, e)
	}
	read := func(port uint16) string {
		t.Helper()
		client := http.Client{}
		r, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		raw, e := io.ReadAll(r.Body)
		if e != nil || r.StatusCode != 200 {
			t.Fatal(e, r.StatusCode)
		}
		return string(raw)
	}
	if !strings.Contains(read(pa), "Synthetic knowledge") || !strings.Contains(read(pb), "Workspace B knowledge") {
		t.Fatal("source selection crossed owners")
	}
	again, e := a.StartNativePublish(ctx)
	if e != nil || again != pa {
		t.Fatal(again, e)
	}
	if e := a.StopNativePublish(ctx); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(read(pb), "Workspace B knowledge") {
		t.Fatal("A stop affected B")
	}
	if _, e := a.StartNativePublish(ctx); e != nil {
		t.Fatal(e)
	}
	a.Release()
	b.Release()
	if e := m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	client := http.Client{}
	if r, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", pb)); e == nil {
		r.Body.Close()
		t.Fatal("drain left listener open")
	}
	if digest(t, root) != before {
		t.Fatal("publish runtime changed source")
	}
}

func TestNativePublishMissingBindingsAndConstructionFailure(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	l := getDocLease(t, m, pageA)
	if _, e := l.StartNativePublish(context.Background()); !errors.Is(e, publishsvc.ErrDependency) {
		t.Fatal(e)
	}
	bad := open(t, fixture(t), Options{PublishOptions: func(string, string) (publishsvc.Options, error) { return publishsvc.Options{}, nil }})
	if _, e := bad.Acquire(context.Background(), storeID, pageA); !errors.Is(e, publishsvc.ErrDependency) || bad.Stats().Workspaces != 0 {
		t.Fatal(e)
	}
}
