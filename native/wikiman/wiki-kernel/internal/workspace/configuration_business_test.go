package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestConfigurationBusinessBoundToLease(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	ctx := context.Background()
	m := open(t, root, Options{
		Configuration: func(_, page string) (conf.AppData, error) { return conf.AppData{Lang: page}, nil },
		ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
			return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
				return []conf.Notebook{{ID: "same-id", Name: page}}, nil
			}, map[string]map[int]string{page: {1: page}}, nil), nil
		},
	})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	for lease, page := range map[*Lease]string{a: pageA, b: pageB} {
		items, err := lease.ConfigurationNotebooks(ctx)
		if err != nil || items[0].Name != page {
			t.Fatal(items, err)
		}
		lang, err := lease.ConfigurationLanguage(ctx, 1)
		if err != nil || lang != page {
			t.Fatal(lang, err)
		}
	}
	a.Release()
	if _, err = a.ConfigurationNotebooks(ctx); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if digest(t, root) != before {
		t.Fatal("configuration business mutated source")
	}
}

func TestConfigurationBusinessProviderFailureAndMissing(t *testing.T) {
	root := fixture(t)
	ctx := context.Background()
	expected := errors.New("synthetic business failure")
	m := open(t, root, Options{ConfigurationBusiness: func(string, string) (*conf.Business, error) { return nil, expected }})
	if _, err := m.Acquire(ctx, storeID, pageA); !errors.Is(err, expected) || m.Stats().Workspaces != 0 {
		t.Fatal(err, m.Stats())
	}
	n := open(t, fixture(t), Options{})
	l, err := n.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if _, err = l.ConfigurationNotebooks(ctx); !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal(err)
	}
	if _, err = l.ConfigurationLanguage(ctx, 1); !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal(err)
	}
}
