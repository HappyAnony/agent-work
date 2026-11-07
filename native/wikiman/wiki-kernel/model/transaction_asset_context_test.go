package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"testing"
)

func transactionAssetTree(box, name string) *parse.Tree {
	root := &ast.Node{Type: ast.NodeDocument}
	root.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/" + name)})
	return &parse.Tree{Box: box, Path: "doc.sy", Root: root}
}

func TestTransactionAssetsSelectDestinationPerTree(t *testing.T) {
	data := t.TempDir()
	for _, dir := range []string{"assets", "one/assets", "two/assets"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	env := &transactionEnvironment{ctx: context.Background(), dataRoot: data, workspaceRoot: data}
	env.resolveGlobalAsset = func(ctx context.Context, relative string) (string, bool, error) {
		return resolveTransactionGlobalAsset(ctx, data, data, relative)
	}
	tx := &Transaction{environment: env}
	for _, box := range []string{"one", "two"} {
		name := box + ".png"
		if err := os.WriteFile(filepath.Join(data, "assets", name), []byte(box), 0600); err != nil {
			t.Fatal(err)
		}
		if err := tx.processGlobalAssets(transactionAssetTree(box, name)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(data, box, "assets", name))
		if err != nil || string(got) != box {
			t.Fatalf("destination %s: %q %v", box, got, err)
		}
	}
}

func TestTransactionAssetsCancelAndFailureStopBeforeMove(t *testing.T) {
	for _, scenario := range []string{"pre-cancel", "resolver-error", "late-cancel", "rename-error", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			data := t.TempDir()
			if err := os.MkdirAll(filepath.Join(data, "box", "assets"), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := os.WriteFile(filepath.Join(data, "source"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			want := errors.New("resolver failed")
			calls := 0
			env := &transactionEnvironment{ctx: ctx, dataRoot: data}
			env.resolveGlobalAsset = func(context.Context, string) (string, bool, error) {
				calls++
				switch scenario {
				case "resolver-error":
					return "", false, want
				case "late-cancel":
					cancel()
					return filepath.Join(data, "source"), true, nil
				case "missing":
					return "", false, nil
				}
				return filepath.Join(data, "nonexistent"), true, nil
			}
			if scenario == "pre-cancel" {
				cancel()
			}
			tree := transactionAssetTree("box", "a.png")
			tree.Root.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/b.png")})
			err := (&Transaction{environment: env}).processGlobalAssets(tree)
			switch scenario {
			case "pre-cancel", "late-cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel lost: %v", err)
				}
			case "resolver-error":
				if !errors.Is(err, want) {
					t.Fatalf("resolver failure lost: %v", err)
				}
			case "rename-error":
				if err == nil {
					t.Fatal("rename failure lost")
				}
			case "missing":
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "pre-cancel" && calls != 0 {
				t.Fatal("resolver entered after cancellation")
			}
			if scenario != "pre-cancel" && scenario != "missing" && calls != 1 {
				t.Fatalf("continued after failure: %d", calls)
			}
			if got, e := os.ReadFile(filepath.Join(data, "source")); e != nil || string(got) != "keep" {
				t.Fatalf("source changed after rejection: %q %v", got, e)
			}
		})
	}
}

func TestTransactionAssetsFailureRetainsSuccessfulPrefix(t *testing.T) {
	data := t.TempDir()
	for _, dir := range []string{"assets", "box/assets"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(data, "assets", "first.png")
	if err := os.WriteFile(source, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	want := errors.New("second failed")
	calls := 0
	env := &transactionEnvironment{ctx: context.Background(), dataRoot: data}
	env.resolveGlobalAsset = func(context.Context, string) (string, bool, error) {
		calls++
		if calls == 1 {
			return source, true, nil
		}
		return "", false, want
	}
	tree := transactionAssetTree("box", "first.png")
	tree.Root.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/second.png")})
	if err := (&Transaction{environment: env}).processGlobalAssets(tree); !errors.Is(err, want) {
		t.Fatalf("failure lost: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(data, "box", "assets", "first.png"))
	if err != nil || string(got) != "first" {
		t.Fatalf("successful prefix lost: %q %v", got, err)
	}
	if _, err = os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("first rename not complete: %v", err)
	}
}
