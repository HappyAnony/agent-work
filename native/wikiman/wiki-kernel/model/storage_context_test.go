package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/pageprefs"
)

func TestStorageContextActualFilesFailureReadonlyAndOwner(t *testing.T) {
	c, _, p := repoMetadataFixture(t)
	for _, q := range []pageprefs.Request{{Action: "local.set", Value: []byte(`{"a":1}`)}, {Action: "criteria.set", Value: []byte(`{"name":"saved","k":"needle"}`)}, {Action: "outline.set", Key: "doc", Value: []byte(`{"folded":true}`)}} {
		if _, e := StorageContext(context.Background(), q); e != nil {
			t.Fatal(e)
		}
	}
	v, e := StorageContext(context.Background(), pageprefs.Request{Action: "local.get"})
	if e != nil || v.(map[string]interface{})["a"] != float64(1) {
		t.Fatal(v, e)
	}
	local := filepath.Join(p.Data, "storage", "local.json")
	before, e := os.ReadFile(local)
	if e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e = StorageContext(context.Background(), pageprefs.Request{Action: "local.set", Value: []byte(`{}`)}); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	after, _ := os.ReadFile(local)
	if string(before) != string(after) {
		t.Fatal("readonly wrote local state")
	}
	if e = os.WriteFile(local, []byte("{bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = StorageContext(context.Background(), pageprefs.Request{Action: "local.get"}); e == nil {
		t.Fatal("read corruption hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = StorageContext(ctx, pageprefs.Request{Action: "criteria.get"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = StorageContext(context.Background(), pageprefs.Request{Action: "criteria.get"}); e == nil {
		t.Fatal("closed owner accepted storage")
	}
}
