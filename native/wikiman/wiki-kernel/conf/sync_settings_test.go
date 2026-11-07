package conf

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncSettingsNormalizeWithoutMutatingInput(t *testing.T) {
	d := AppData{Sync: NewSync()}
	for _, tt := range []struct{ kind, raw string }{
		{"syncEnabled", "true"}, {"syncConflict", "true"}, {"syncMode", "2"}, {"syncProvider", "4"}, {"syncInterval", "90000"}, {"syncPerception", "true"},
		{"syncS3", `{"endpoint":" example.test ","accessKey":" a ","secretKey":" b ","bucket":" bucket ","timeout":1,"concurrentReqs":99}`},
		{"syncWebDAV", `{"endpoint":" example.test ","username":" u ","password":" p ","timeout":0,"concurrentReqs":0}`},
	} {
		p, err := d.PrepareSetting(tt.kind, []byte(tt.raw))
		if err != nil {
			t.Fatal(tt.kind, err)
		}
		if d.Sync.Enabled || d.Sync.GenerateConflictDoc || d.Sync.S3 != nil || d.Sync.Interval != 30 {
			t.Fatal("input mutated")
		}
		switch tt.kind {
		case "syncInterval":
			if p.Data.Sync.Interval != 43200 || p.Effects[0].Int != 43200 {
				t.Fatal(p)
			}
		case "syncS3":
			v := p.Data.Sync.S3
			if v.Endpoint != "http://example.test/" || v.AccessKey != "a" || v.SecretKey != "b" || v.Timeout != 7 || v.ConcurrentReqs != 16 {
				t.Fatal(v)
			}
		case "syncWebDAV":
			v := p.Data.Sync.WebDAV
			if v.Timeout != 60 || v.ConcurrentReqs != 1 || v.Username != "u" {
				t.Fatal(v)
			}
		}
	}
	for _, tt := range []struct{ kind, raw string }{{"syncEnabled", "1"}, {"syncInterval", "1.5"}, {"syncS3", `{"bucket":"bad/name"}`}, {"syncWebDAV", `{"endpoint":"dav.jianguoyun.com"}`}, {"syncLocal", `{"endpoint":"/tmp"}`}} {
		if _, err := d.PrepareSetting(tt.kind, []byte(tt.raw)); err == nil {
			t.Fatal(tt)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.PrepareSettingWithServices(ctx, "syncEnabled", json.RawMessage("true"), SettingServices{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	p, err := d.PrepareSettingWithServices(context.Background(), "syncPerception", []byte("true"), SettingServices{DisableSyncPerception: true})
	if err != nil || p.Data.Sync.Perception {
		t.Fatal(p, err)
	}
}

func TestSyncLocalCanonicalOwnerBoundary(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	for _, p := range []string{workspace, outside, filepath.Join(workspace, "nested")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{workspace, root, filepath.Join(workspace, "nested"), alias} {
		if err := ValidateSyncLocal(context.Background(), p, workspace); err == nil {
			t.Fatal(p)
		}
	}
	if err := ValidateSyncLocal(context.Background(), outside, workspace); err != nil {
		t.Fatal(err)
	}
}
