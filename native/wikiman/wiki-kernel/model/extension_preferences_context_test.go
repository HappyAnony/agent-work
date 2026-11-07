package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
)

func TestExtensionPreferencesActualResourcesAndOwner(t *testing.T) {
	c, _, p := repoMetadataFixture(t)
	c.Bazaar = conf.NewBazaar()
	c.Bazaar.Trust = true
	c.Lang = "en_US"
	base := filepath.Join(p.Data, "plugins", "demo")
	if e := os.MkdirAll(filepath.Join(base, "i18n"), 0700); e != nil {
		t.Fatal(e)
	}
	for n, raw := range map[string]string{"plugin.json": `{"name":"demo","displayName":{"en_US":"Owned"},"frontends":["desktop"],"backends":["all"]}`, "index.js": "owned JS", "i18n/en_US.json": `{"name":"owner"}`} {
		if e := os.WriteFile(filepath.Join(base, n), []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
	}
	v, e := SetPetalEnabledContext(context.Background(), "demo", true, "desktop", "", false)
	if e != nil || v.DisplayName != "Owned" || v.JS != "owned JS" {
		t.Fatal(v, e)
	}
	got, e := LoadPetalsContext(context.Background(), "desktop", false)
	if e != nil || len(got) != 1 || got[0].I18n["name"] != "owner" {
		t.Fatal(got, e)
	}
	if e = SetSnippetContext(context.Background(), []*conf.Snippet{{Name: "saved", Type: "js", Enabled: true}}); e != nil {
		t.Fatal(e)
	}
	snips, e := LoadSnippetsContext(context.Background())
	if e != nil || len(snips) != 1 || snips[0].ID == "" {
		t.Fatal(snips, e)
	}
	var raw []*conf.Snippet
	bytes, e := os.ReadFile(filepath.Join(p.Data, "snippets", "conf.json"))
	if e != nil || json.Unmarshal(bytes, &raw) != nil || raw[0].ID != snips[0].ID {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if e = SetSnippetContext(context.Background(), nil); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	if e = os.WriteFile(filepath.Join(base, "plugin.json"), []byte("{bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadPetalsContext(context.Background(), "desktop", false); e == nil {
		t.Fatal("bad package hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = LoadSnippetsContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadPetalsContext(context.Background(), "desktop", false); e == nil {
		t.Fatal("closed plugin owner admitted")
	}
}
