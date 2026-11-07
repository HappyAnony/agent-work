package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-kernel/internal/sourcetx"
)

func TestSandboxedPluginStorageBindsPagePluginAndReceipt(t *testing.T) {
	root, manager, store := setupPages(t)
	const plugin = "storagefixture"
	code := []byte("fixture plugin")
	page := filepath.Join(root, "pages", testPage, "data")
	write := func(name string, value []byte) {
		t.Helper()
		path := filepath.Join(page, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(scopedPetalsPath, []byte(`[{"name":"storagefixture","enabled":true}]`))
	write("plugins/"+plugin+"/plugin.json", []byte(`{"name":"storagefixture","backends":["all"],"frontends":["desktop"]}`))
	write("plugins/"+plugin+"/index.js", code)
	ctx, err := WithExternalPreviewPolicy(context.Background(), nil, nil, nil, nil,
		map[string]string{plugin: sourcetx.Hash(code)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(c context.Context, mode, id, op, action, file string, value *string) (any, error) {
		t.Helper()
		args := map[string]any{"plugin": plugin, "action": action, "file": file}
		if value != nil {
			args["value"] = *value
		}
		raw, _ := json.Marshal(args)
		envelope, _ := json.Marshal(UIRequest{ProtocolVersion: 1, Endpoint: uiPluginStorageEndpoint, StoreID: store, PageID: id, OperationID: op, Data: raw})
		request, err := DecodeUIRequest(strings.NewReader(string(envelope)))
		if err != nil {
			return nil, err
		}
		return UI(c, manager, request, UIView{Mode: mode})
	}
	if _, err := call(ctx, "wikiDesktop", testPage, "", "load", "settings.json", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("storage enabled without a grant", err)
	}
	ctx, err = WithScopedPluginStorage(ctx, []string{plugin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(ctx, "docxSingle", testPage, "", "load", "settings.json", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("Single obtained plugin data", err)
	}
	if _, err := call(ctx, "wikiDesktop", testPage, "", "load", "../petals.json", nil); !errors.Is(err, ErrArgument) {
		t.Fatal("accepted a path outside plugin storage", err)
	}
	value := `{"count":7}`
	for range 2 {
		if _, err := call(ctx, "wikiDesktop", testPage, "plugin-first", "save", "settings.json", &value); err != nil {
			t.Fatal("could not commit or replay plugin data", err)
		}
	}
	storagePath := filepath.Join(page, "storage/petal", plugin, "settings.json")
	if raw, err := os.ReadFile(storagePath); err != nil || string(raw) != value {
		t.Fatal("source did not preserve plugin data", err)
	}
	if reply, err := call(ctx, "wikiDesktop", testPage, "", "load", "settings.json", nil); err != nil || string(reply.(json.RawMessage)) != value {
		t.Fatal("failed to load exact plugin JSON", err)
	}
	altered := "different"
	if _, err := call(ctx, "wikiDesktop", testPage, "plugin-first", "save", "settings.json", &altered); err == nil {
		t.Fatal("an existing operation ID accepted different content")
	}
	if _, err := call(ctx, "wikiDesktop", emptyPage, "", "load", "settings.json", nil); err == nil {
		t.Fatal("another page opened the plugin's data")
	}
	for range 2 {
		if _, err := call(ctx, "wikiDesktop", testPage, "plugin-remove", "remove", "settings.json", nil); err != nil {
			t.Fatal("could not remove or replay removal", err)
		}
	}
	if _, err := os.Stat(storagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("plugin data remained after remove", err)
	}
	tooLarge := strings.Repeat("a", scopedPluginFileLimit+1)
	if _, err := call(ctx, "wikiDesktop", testPage, "plugin-oversized", "save", "settings.json", &tooLarge); !errors.Is(err, ErrArgument) {
		t.Fatal("accepted oversized plugin data", err)
	}
	write("plugins/"+plugin+"/index.js", []byte("changed code"))
	if _, err := call(ctx, "wikiDesktop", testPage, "", "load", "settings.json", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("changed code retained storage access", err)
	}
}
