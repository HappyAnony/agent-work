//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func TestEmojiConfigurationOwnsCatalogueParserAndRepair(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	old := util.AppearancePath
	util.AppearancePath = t.TempDir()
	defer func() { util.AppearancePath = old }()
	if err := os.MkdirAll(filepath.Join(util.AppearancePath, "emojis"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(util.AppearancePath, "emojis/conf.json"), []byte(`[{"id":"built"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(util.DataDir, "emojis")
	os.MkdirAll(filepath.Join(folder, "faces"), 0700)
	os.WriteFile(filepath.Join(folder, "faces/happy.png"), []byte("image"), 0600)
	os.WriteFile(filepath.Join(folder, "a.png"), []byte("image"), 0600)
	c.r.owner.persistenceReadOnly = true
	got, err := EmojiConfigurationContext(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatal(err, got)
	}
	if c.r.owner.customEmojis.Snapshot()["faces/happy"] != "/emojis/faces/happy.png" {
		t.Fatal("owner parser registry missing")
	}
	other := NewAppConf()
	if len(other.customEmojis.Snapshot()) != 0 {
		t.Fatal("emoji registry shared across owners")
	}
	bad := filepath.Join(folder, "bad[name].png")
	os.WriteFile(bad, []byte("bad"), 0600)
	if got, err = EmojiConfigurationContext(context.Background()); !errors.Is(err, ErrSettingReadOnly) || got != nil {
		t.Fatal(err, got)
	}
	if raw, _ := os.ReadFile(bad); !bytes.Equal(raw, []byte("bad")) {
		t.Fatal("readonly read repaired source")
	}
	c.r.owner.persistenceReadOnly = false
	if _, err = EmojiConfigurationContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("original repair missing", err)
	}
	if _, ok := c.r.owner.customEmojis.Snapshot()["badname"]; !ok {
		t.Fatal("repaired name not published")
	}
	os.Symlink(t.TempDir(), filepath.Join(folder, "link"))
	if got, err = EmojiConfigurationContext(context.Background()); err == nil || got != nil {
		t.Fatal("symlink admitted", err, got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = EmojiConfigurationContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
