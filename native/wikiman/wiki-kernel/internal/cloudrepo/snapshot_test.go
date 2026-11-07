package cloudrepo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/conf"
)

func TestSnapshotValidationCapabilityMembershipAndPartialOutcome(t *testing.T) {
	cfg := conf.AppData{Sync: conf.NewSync(), Repo: conf.NewRepo()}
	cfg.Repo.Key = make([]byte, 32)
	cfg.User = &conf.User{UserId: "synthetic", UserSiYuanProExpireTime: -1, UserSiYuanSubscriptionStatus: 0}
	opts := Options{Load: func(context.Context) (conf.AppData, error) { return cfg, nil }, Execute: func(context.Context, conf.AppData, Request) (Result, error) {
		return Result{Data: json.RawMessage(`{}`)}, nil
	}}
	read, _ := New(opts)
	defer read.Close(context.Background())
	if _, e := read.Snapshot(context.Background(), SnapshotRequest{Kind: "purge-local"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	called := 0
	opts.Snapshot = func(ctx context.Context, v conf.AppData, r SnapshotRequest) (SnapshotResult, error) {
		called++
		v.Repo.Key[0] = 9
		return SnapshotResult{Applied: true, Uncertain: true}, context.Canceled
	}
	s, _ := New(opts)
	defer s.Close(context.Background())
	id := strings.Repeat("a", 40)
	out, e := s.Snapshot(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"})
	if !out.Applied || !out.Uncertain || !errors.Is(e, context.Canceled) || cfg.Repo.Key[0] != 0 {
		t.Fatal(out, e)
	}
	for _, r := range []SnapshotRequest{{Kind: "checkout"}, {Kind: "tag", ID: "short", Tag: "a"}, {Kind: "untag", Tag: "../a"}, {Kind: "upload", ID: id, Tag: ".."}} {
		if _, e = s.Snapshot(context.Background(), r); e == nil {
			t.Fatal(r)
		}
	}
	cfg.User = nil
	out, e = s.Snapshot(context.Background(), SnapshotRequest{Kind: "download", ID: id})
	if e != nil || out.Notice != 29 || called != 1 {
		t.Fatal(out, e, called)
	}
	cfg.Sync.Provider = conf.ProviderS3
	out, e = s.Snapshot(context.Background(), SnapshotRequest{Kind: "cloud-untag", Tag: "tag"})
	if e != nil || out.Notice != 214 {
		t.Fatal(out, e)
	}
	cfg.Repo.Key = nil
	if _, e = s.Snapshot(context.Background(), SnapshotRequest{Kind: "tag", ID: id, Tag: "a"}); !errors.Is(e, ErrKey) {
		t.Fatal(e)
	}
}
