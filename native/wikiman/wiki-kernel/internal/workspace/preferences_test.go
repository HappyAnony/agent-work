package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
)

func TestClassifiedPreferencesPersistCASAndIsolatePages(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{Configuration: func(string, string) (conf.AppData, error) {
		return conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), Repo: conf.NewRepo(), System: &conf.System{DataDir: "trusted-runtime"}}, nil
	}})
	before := digest(t, root)
	device := conf.PreferenceDocument{Version: 1, Scope: conf.DevicePreference, Values: map[string]json.RawMessage{"editor.fontSize": json.RawMessage(`31`), "repo.key": json.RawMessage(`"c3ludGhldGlj"`)}}
	hash, err := m.SaveDevicePreferences(ctx, storeID, "", device)
	if err != nil || hash == "" {
		t.Fatal(hash, err)
	}
	if digest(t, root) != before {
		t.Fatal("device preferences entered source")
	}
	if _, err = m.SaveDevicePreferences(ctx, storeID, "", device); !errors.Is(err, sourcetx.ErrConflict) {
		t.Fatal(err)
	}
	workspace := conf.PreferenceDocument{Version: 1, Scope: conf.WorkspacePreference, Values: map[string]json.RawMessage{"editor.markdown.inlineMath": json.RawMessage(`false`)}}
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err = m.SaveWorkspacePreferences(deadline, storeID, pageA, "preferences-blocked", "", workspace); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	a.Release()
	r, err := m.SaveWorkspacePreferences(ctx, storeID, pageA, "preferences-save", "", workspace)
	if err != nil || !r.SourceCommitted {
		t.Fatal(r, err)
	}
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		data, err := l.Configuration(ctx)
		paths, pathErr := l.RuntimePaths(ctx)
		l.Release()
		if err != nil || pathErr != nil || data.Editor.FontSize != 31 || string(data.Repo.Key) != "synthetic" || data.Editor.Markdown.InlineMath != (page == pageB) || data.System.DataDir != paths.DataDir {
			t.Fatal(page, data, err)
		}
	}
	if _, err = m.SaveWorkspacePreferences(ctx, storeID, pageA, "preferences-conflict", "", workspace); !errors.Is(err, sourcetx.ErrConflict) {
		t.Fatal(err)
	}
	workspace.Values["system.dataDir"] = json.RawMessage(`"/smuggled"`)
	if _, err = m.SaveWorkspacePreferences(ctx, storeID, pageA, "preferences-invalid", "", workspace); !errors.Is(err, conf.ErrPreferences) {
		t.Fatal(err)
	}
	name, _ := sourcetx.WorkspacePreferencesPath(pageA)
	raw, err := m.SourceFile(ctx, storeID, name)
	if err != nil {
		t.Fatal(err)
	}
	// Bypassing the settings service still cannot write wrong-owner fields.
	bad, _ := json.Marshal(workspace)
	if _, err = m.Apply(ctx, storeID, sourcetx.Request{OperationID: "preferences-bypass", Changes: []sourcetx.Change{{Path: name, ExpectedHash: sourcetx.Hash(raw), Data: bad}}}); !errors.Is(err, conf.ErrPreferences) {
		t.Fatal(err)
	}
	// Missing/malformed persisted documents do not silently reset or default.
	if err = os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(`{"invalid":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Acquire(ctx, storeID, pageA); !errors.Is(err, conf.ErrPreferences) {
		t.Fatal(err)
	}
}
