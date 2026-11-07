package cloudrepo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestDirectoryCapabilityValidationAndPreservedPartialResult(t *testing.T) {
	cfg := conf.AppData{Sync: conf.NewSync()}
	calls := 0
	opts := Options{Load: func(context.Context) (conf.AppData, error) { return cfg, nil }, Execute: func(context.Context, conf.AppData, Request) (Result, error) {
		return Result{Data: json.RawMessage(`{}`)}, nil
	}}
	read, _ := New(opts)
	defer read.Close(context.Background())
	if _, e := read.Directory(context.Background(), DirectoryRequest{Kind: "set", Name: "next"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	opts.Directory = func(ctx context.Context, v conf.AppData, r DirectoryRequest) (DirectoryResult, error) {
		calls++
		if r.Kind == "create" && r.Name != "Name" {
			t.Error(r.Name)
		}
		v.Sync.CloudName = "detached"
		return DirectoryResult{Applied: true, Active: "main"}, context.Canceled
	}
	s, _ := New(opts)
	defer s.Close(context.Background())
	out, e := s.Directory(context.Background(), DirectoryRequest{Kind: "create", Name: "  Na\u200bme  "})
	if !out.Applied || !errors.Is(e, context.Canceled) || cfg.Sync.CloudName != "main" {
		t.Fatal(out, e)
	}
	for _, r := range []DirectoryRequest{{"purge", "main"}, {"remove", "../x"}, {"remove", "a/b"}, {"set", "bad name"}, {"create", "\uE000"}} {
		if _, e = s.Directory(context.Background(), r); e == nil {
			t.Fatal(r)
		}
	}
	cfg.Sync.Provider = conf.ProviderS3
	if _, e = s.Directory(context.Background(), DirectoryRequest{"remove", "main"}); !errors.Is(e, ErrProviderType) {
		t.Fatal(e)
	}
	cfg.Sync.Provider = conf.ProviderLocal
	if out, e = s.Directory(context.Background(), DirectoryRequest{"remove", ""}); e != nil || out.Active != "main" || calls != 1 {
		t.Fatal(out, e, calls)
	}
}
