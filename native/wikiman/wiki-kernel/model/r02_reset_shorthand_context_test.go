package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestResetBlockAttributesContextPreservesOrderAndExactValues(t *testing.T) {
	tree := &parse.Tree{ID: "doc", Box: "box", Path: "/doc.sy", Root: &ast.Node{ID: "doc", Type: ast.NodeDocument}}
	tree.Root.SetIALAttr("custom-old", "value")
	var phases []string
	e := &blockAttributeEnvironment{
		ctx: context.Background(), gate: newAttributeMutationGate(), legacy: true,
		wait:    func() error { phases = append(phases, "wait"); return nil },
		load:    func(string) (*parse.Tree, error) { phases = append(phases, "load"); return tree, nil },
		stage:   func(*parse.Tree) error { phases = append(phases, "stage"); return nil },
		sync:    func() error { phases = append(phases, "sync"); return nil },
		evict:   func(string) error { phases = append(phases, "evict"); return nil },
		rename:  func(*parse.Tree) error { phases = append(phases, "rename"); return nil },
		invalid: func(string) error { return errors.New("invalid") }, missing: func(string) error { return errors.New("missing") },
	}
	attrs := map[string]string{"custom-key": "  exact value  ", "empty": ""}
	if err := resetBlockAttributesWithEnvironment("doc", attrs, e); err != nil {
		t.Fatal(err)
	}
	if got := tree.Root.IALAttr("custom-key"); got != "  exact value  " || tree.Root.IALAttr("custom-old") != "" || tree.Root.IALAttr("empty") != "" {
		t.Fatalf("reset values = %q attrs=%v", got, tree.Root.KramdownIAL)
	}
	if want := []string{"wait", "load", "rename", "stage", "sync", "evict"}; !reflect.DeepEqual(phases, want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
}

func TestResetBlockAttributesCancellationStopsFollowingPhases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tree := &parse.Tree{ID: "doc", Box: "box", Path: "/doc.sy", Root: &ast.Node{ID: "doc", Type: ast.NodeDocument}}
	tree.Root.SetIALAttr("old", "value")
	var synced, evicted bool
	e := &blockAttributeEnvironment{
		ctx: ctx, gate: newAttributeMutationGate(), legacy: true,
		wait: func() error { return nil }, load: func(string) (*parse.Tree, error) { return tree, nil },
		rename: func(*parse.Tree) error { return nil }, stage: func(*parse.Tree) error { cancel(); return nil },
		sync: func() error { synced = true; return nil }, evict: func(string) error { evicted = true; return nil },
		invalid: func(string) error { return errors.New("invalid") }, missing: func(string) error { return errors.New("missing") },
	}
	if err := resetBlockAttributesWithEnvironment("doc", map[string]string{"new": "value"}, e); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if synced || evicted {
		t.Fatal("cancellation allowed a later reset phase")
	}
}

func testShorthandOwner(t *testing.T) (*shorthandOwner, string, string) {
	t.Helper()
	oldConf := Conf
	oldWorkspace, oldData, oldShortcuts := util.WorkspaceDir, util.DataDir, util.ShortcutsPath
	root := t.TempDir()
	workspace, data, shortcuts := filepath.Join(root, "workspace"), filepath.Join(root, "data"), filepath.Join(root, "shortcuts")
	for _, dir := range []string{workspace, filepath.Join(data, "assets"), filepath.Join(shortcuts, "shorthands", "assets")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	owner := NewAppConfWithBusiness(conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
		return []conf.Notebook{{ID: "box"}}, nil
	}, nil, nil))
	Conf, util.WorkspaceDir, util.DataDir, util.ShortcutsPath = owner, workspace, data, shortcuts
	t.Cleanup(func() {
		_ = owner.CloseCloudRepo(context.Background())
		Conf, util.WorkspaceDir, util.DataDir, util.ShortcutsPath = oldConf, oldWorkspace, oldData, oldShortcuts
	})
	o, err := captureShorthandOwner(context.Background(), "box")
	if err != nil {
		t.Fatal(err)
	}
	o.ports.sort = func(*Box, string) error { return nil }
	return o, filepath.Join(shortcuts, "shorthands"), filepath.Join(data, "assets")
}

func TestShorthandContextMovesMultipleItemsAndCleansOnlyCompletedSources(t *testing.T) {
	o, source, assets := testShorthandOwner(t)
	for name, content := range map[string]string{"1700000000000.md": " first ", "1700000001000.md": "second", "1700000002000.md": "  "} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "assets", "image.png"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	var created []string
	o.ports.create = func(_ context.Context, _, path, content, _ string) (string, error) {
		created = append(created, path+":"+content)
		return "id-" + path, nil
	}
	ids, err := o.move(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || len(created) != 2 {
		t.Fatalf("ids=%v created=%v", ids, created)
	}
	if _, err = os.Stat(filepath.Join(assets, "image.png")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"1700000000000.md", "1700000001000.md", "1700000002000.md"} {
		if _, err = os.Stat(filepath.Join(source, name)); !os.IsNotExist(err) {
			t.Fatalf("source %s was not cleaned: %v", name, err)
		}
	}
}

func TestShorthandContextCancellationKeepsSuccessfulPrefixWithoutCleanup(t *testing.T) {
	o, source, assets := testShorthandOwner(t)
	if err := os.WriteFile(filepath.Join(source, "1700000000000.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "assets", "image.png"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	originalRename := o.ports.rename
	o.ports.rename = func(from, to string) error {
		if err := originalRename(from, to); err != nil {
			return err
		}
		cancel()
		return nil
	}
	if _, err := o.move(ctx, "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(assets, "image.png")); err != nil {
		t.Fatal("completed asset move was rolled back", err)
	}
	if _, err := os.Stat(filepath.Join(source, "1700000000000.md")); err != nil {
		t.Fatal("uncarried note was cleaned", err)
	}
}

func TestMoveLocalShorthandsRejectsPreCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MoveLocalShorthandsContext(ctx, "box", "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before dependency capture, got %v", err)
	}
}

func TestShorthandOwnerCloseCancelsAndJoinsAcceptedMove(t *testing.T) {
	o, source, _ := testShorthandOwner(t)
	if err := os.WriteFile(filepath.Join(source, "1700000000000.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	o.ports.create = func(ctx context.Context, _, _, _, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	done := make(chan error, 1)
	go func() { _, err := o.run(context.Background(), "", ""); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("move did not start")
	}
	if err := o.owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("move error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("owner close did not join move")
	}
}
