package sourcetx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
)

func TestClassifiedPreferencesUseRecoveryAndRejectSymlinks(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	doc := conf.PreferenceDocument{Version: 1, Scope: conf.WorkspacePreference, Values: map[string]json.RawMessage{"editor.markdown.inlineMath": json.RawMessage(`false`)}}
	request, err := WorkspacePreferencesRequest(p2, "preferences-interrupted", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Apply(ctx, request, func(point string) error {
		if point == "published:0" {
			return errors.New("synthetic interruption")
		}
		return nil
	}); err == nil {
		t.Fatal("fault did not interrupt")
	}
	base := conf.AppData{Editor: conf.NewEditor()}
	if _, err = s.Configuration(ctx, p2, base); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	device := conf.PreferenceDocument{Version: 1, Scope: conf.DevicePreference, Values: map[string]json.RawMessage{"lang": json.RawMessage(`"zh_CN"`)}}
	if _, err = w.SaveDevicePreferences(ctx, "", device); !errors.Is(err, ErrRecovery) {
		t.Fatal("local settings bypassed pending journal", err)
	}
	if _, err = w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	data, err := s.Configuration(ctx, p2, base)
	if err != nil || !data.Editor.Markdown.InlineMath {
		t.Fatal(data, err)
	}
	request.OperationID = "preferences-after-recovery"
	if receipt, err := w.Apply(ctx, request, nil); err != nil || !receipt.SourceCommitted {
		t.Fatal(receipt, err)
	}
	data, err = s.Configuration(ctx, p2, base)
	if err != nil || data.Editor.Markdown.InlineMath {
		t.Fatal(data, err)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(root, ".local", "conf")); err != nil {
		t.Fatal(err)
	}
	if _, err = w.SaveDevicePreferences(ctx, "", device); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("symlink escaped local configuration", entries, err)
	}
}
