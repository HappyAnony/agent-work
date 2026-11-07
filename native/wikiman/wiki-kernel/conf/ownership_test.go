package conf

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalConfigurationFieldsClassified(t *testing.T) {
	typ := reflect.TypeOf(AppData{})
	count := 0
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if _, err := ScopeFor(name); err != nil {
			t.Fatalf("unclassified original field %s: %v", name, err)
		}
		count++
	}
	if count != 31 {
		t.Fatalf("original contract changed: %d", count)
	}
	for name, scope := range FieldScopes() {
		if got, err := ScopeFor(name); err != nil || got != scope {
			t.Fatalf("invalid ownership override %s: %s %v", name, got, err)
		}
	}
	for name, want := range map[string]Scope{"system.dataDir": RuntimeValue, "repo.key": DevicePreference, "editor.fontSize": DevicePreference, "editor.markdown.inlineMath": WorkspacePreference} {
		if got, err := ScopeFor(name); err != nil || got != want {
			t.Fatal(name, got, err)
		}
	}
	if _, err := ScopeFor("editor.markdown.unknown"); err == nil {
		t.Fatal("unknown field silently classified")
	}
	policy := FieldScopes()
	policy["editor"] = DevicePreference
	if got, _ := ScopeFor("editor"); got != WorkspacePreference {
		t.Fatal("mutable policy leaked")
	}
}

func TestConfigurationCloneAndExplicitDefaults(t *testing.T) {
	t.Setenv("WIKI_OPENAI_API_KEY", "synthetic-not-a-secret")
	if NewAI().OpenAI.APIKey != "" {
		t.Fatal("pure default read environment")
	}
	withEnvironment := NewAIWithEnvironment("synthetic-agent", func(key string) string {
		if key == "WIKI_OPENAI_API_KEY" {
			return "explicit-fixture"
		}
		return ""
	})
	if withEnvironment.OpenAI.APIKey != "explicit-fixture" || withEnvironment.OpenAI.APIUserAgent != "synthetic-agent" {
		t.Fatal("explicit preference lost")
	}
	data := AppData{Editor: NewEditor(), Export: NewExport(), Repo: NewRepo(), LocalIPs: []string{"fixture"}, System: NewSystem()}
	data.Repo.Key = []byte("synthetic-key")
	copy, err := data.Clone()
	if err != nil {
		t.Fatal(err)
	}
	copy.Editor.Markdown.InlineMath = !data.Editor.Markdown.InlineMath
	copy.Repo.Key[0] = 'X'
	copy.LocalIPs[0] = "changed"
	if data.Repo.Key[0] == 'X' || data.LocalIPs[0] == "changed" || data.Editor.Markdown.InlineMath == copy.Editor.Markdown.InlineMath {
		t.Fatal("configuration clone shares mutable state")
	}
	if data.System.ID != "" || data.System.Name != "" || data.System.KernelVersion != "" {
		t.Fatal("configuration queried device implicitly")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if len(obj) != 31 {
		t.Fatal("serialized original shape", len(obj))
	}
	if _, err = (AppData{}).ParserOptions(nil); err == nil {
		t.Fatal("missing settings silently defaulted")
	}
}

func TestRepositoryPathRequiresExplicitOwner(t *testing.T) {
	r := NewRepo()
	for _, bad := range []string{"", string(filepath.Separator)} {
		if _, err := r.SaveDir(bad); err == nil {
			t.Fatal("unsafe repo path accepted")
		}
	}
	root := t.TempDir()
	got, err := r.SaveDir(root)
	if err != nil || got != filepath.Join(root, "repo") {
		t.Fatal(got, err)
	}
}
