package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/88250/lute"
	"wiki-kernel/conf"
)

func TestFullOriginalConfigurationOwnedByPage(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	input := conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), Search: conf.NewSearch(), Repo: conf.NewRepo(), System: &conf.System{DataDir: "/not-a-source-fixture", WorkspaceDir: "/not-a-source-fixture"}}
	input.Repo.Key = []byte("synthetic")
	m := open(t, root, Options{Configuration: func(store, page string) (conf.AppData, error) {
		if store != storeID {
			return conf.AppData{}, ErrIdentity
		}
		value, err := input.Clone()
		if err != nil {
			return value, err
		}
		value.Lang = page
		value.Editor.Markdown.InlineMath = page == pageA
		return value, nil
	}})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	input.Editor.FontSize = 99
	input.Repo.Key[0] = 'X'
	first, err := a.Configuration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Lang != pageA || first.Editor.FontSize == 99 || first.Repo.Key[0] == 'X' {
		t.Fatal("provider mutation leaked into acquired context")
	}
	first.Editor.FontSize = 88
	again, err := a.Configuration(ctx)
	if err != nil || again.Editor.FontSize == 88 {
		t.Fatal("returned mutable configuration", err)
	}
	for lease, want := range map[*Lease]bool{a: true, b: false} {
		if err := lease.WithConfiguredParser(ctx, nil, func(engine *lute.Lute) error {
			if engine.ParseOptions.InlineMath != want {
				t.Fatal("wrong page settings")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if digest(t, root) != before {
		t.Fatal("configuration access persisted defaults")
	}
	if _, err = a.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal("imported configuration path redirected source access", err)
	}
	if digest(t, root) != before {
		t.Fatal("configuration-driven read changed source")
	}
}

func TestConfigurationFailureDoesNotPublishContext(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	fail := true
	expected := errors.New("synthetic configuration failure")
	m := open(t, root, Options{Configuration: func(string, string) (conf.AppData, error) {
		if fail {
			return conf.AppData{}, expected
		}
		return conf.AppData{}, nil
	}})
	if _, err := m.Acquire(context.Background(), storeID, pageA); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	if m.Stats().Workspaces != 0 {
		t.Fatal("failed context published")
	}
	fail = false
	l, err := m.Acquire(context.Background(), storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if err := l.WithConfiguredParser(context.Background(), nil, func(*lute.Lute) error { return nil }); err == nil {
		t.Fatal("empty configuration silently defaulted")
	}
	if digest(t, root) != before {
		t.Fatal("configuration failure changed source")
	}
}
