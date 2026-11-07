package extensionprefs

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"wiki-kernel/conf"
)

func TestExtensionsOriginalRulesAndFailure(t *testing.T) {
	fake := fstest.MapFS{"plugins/demo/plugin.json": &fstest.MapFile{Data: []byte(`{"name":"demo","displayName":{"default":"Default","en_US":"English"},"backends":["all"],"frontends":["desktop"],"disabledInPublish":true}`)}, "plugins/demo/index.js": &fstest.MapFile{Data: []byte("synthetic JS")}, "plugins/demo/index.css": &fstest.MapFile{Data: []byte("synthetic CSS")}, "plugins/demo/i18n/en_US.json": &fstest.MapFile{Data: []byte(`{"hello":"world"}`)}}
	failure := false
	writes := 0
	p := Ports{Language: "missing", Backend: "darwin", Trusted: true, RequireTrust: true, Repair: true, NewID: func() string { return "new-id" }, Read: func(_ context.Context, n string) ([]byte, error) { return fs.ReadFile(fake, n) }, ReadDir: func(_ context.Context, n string) ([]fs.DirEntry, error) { return fs.ReadDir(fake, n) }, Write: func(_ context.Context, n string, b []byte) error {
		if failure {
			return errors.New("failed write")
		}
		fake[n] = &fstest.MapFile{Data: append([]byte(nil), b...)}
		writes++
		return nil
	}}
	s := &Service{}
	out, e := s.Do(context.Background(), Request{Action: "petal.set", Name: "demo", Frontend: "desktop", Enabled: true}, p)
	if e != nil {
		t.Fatal(e)
	}
	v := out.(*Petal)
	if v.DisplayName != "English" || v.JS != "synthetic JS" || v.I18n["hello"] != "world" {
		t.Fatal(v)
	}
	out, e = s.Do(context.Background(), Request{Action: "petal.list", Frontend: "desktop", Publish: true}, p)
	if e != nil || len(out.([]*Petal)) != 0 {
		t.Fatal(out, e)
	}
	if _, e = s.Do(context.Background(), Request{Action: "petal.set", Name: "demo", Frontend: "mobile"}, p); !errors.Is(e, ErrIncompatible) {
		t.Fatal(e)
	}
	if _, e = s.Do(context.Background(), Request{Action: "petal.set", Name: "../demo"}, p); e == nil {
		t.Fatal("path traversal")
	}
	failure = true
	before := writes
	if _, e = s.Do(context.Background(), Request{Action: "petal.set", Name: "demo", Frontend: "desktop"}, p); e == nil || before != writes {
		t.Fatal("write failure hidden")
	}
	failure = false
	raw, _ := json.Marshal([]*conf.Snippet{{Name: "visible", Type: "js", Enabled: true}, {ID: "hidden", Name: "hidden", Type: "css", Enabled: true, DisabledInPublish: true}})
	if _, e = s.Do(context.Background(), Request{Action: "snippet.set", Value: raw}, p); e != nil {
		t.Fatal(e)
	}
	out, e = s.Do(context.Background(), Request{Action: "snippet.list", FilterSnippets: true, SnippetType: "all", SnippetEnabled: 2, Publish: true}, p)
	if e != nil || len(out.([]*conf.Snippet)) != 1 || out.([]*conf.Snippet)[0].ID != "new-id" {
		t.Fatal(out, e)
	}
	out, e = s.Do(context.Background(), Request{Action: "snippet.remove", Name: "new-id"}, p)
	if e != nil || out.(*conf.Snippet).Name != "visible" {
		t.Fatal(out, e)
	}
}
