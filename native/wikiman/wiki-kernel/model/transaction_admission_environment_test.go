package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/treenode"
)

func TestTransactionMissingTargetsUseCapturedReload(t *testing.T) {
	reloads := 0
	env := &transactionEnvironment{
		ctx:      context.Background(),
		legacy:   true,
		lookup:   func(string) *treenode.BlockTree { return nil },
		reloadUI: func() { reloads++ },
	}
	tx := &Transaction{environment: env}

	if err := tx.doPrependInsert(&Operation{ParentID: "missing"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.doAppendInsert(&Operation{ParentID: "missing"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.doInsert(&Operation{ParentID: "missing"}); err != nil {
		t.Fatal(err)
	}
	if reloads != 3 {
		t.Fatalf("captured reload calls = %d, want 3", reloads)
	}
}

func TestTransactionGlobalAssetsUseCapturedRoot(t *testing.T) {
	workspaceA, workspaceB := t.TempDir(), t.TempDir()
	dataA, dataB := filepath.Join(workspaceA, "data"), filepath.Join(workspaceB, "data")
	box, doc := "box", "doc.sy"
	localAssets := filepath.Join(dataA, box, "assets")
	for _, dir := range []string{filepath.Join(dataA, "assets"), filepath.Join(dataB, "assets"), localAssets} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	assetA, assetB := filepath.Join(dataA, "assets", "captured.png"), filepath.Join(dataB, "assets", "captured.png")
	if err := os.WriteFile(assetA, []byte("A"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetB, []byte("B"), 0600); err != nil {
		t.Fatal(err)
	}

	root := &ast.Node{Type: ast.NodeDocument}
	root.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/captured.png?query=1")})
	tree := &parse.Tree{Box: box, Path: doc, Root: root}
	env := &transactionEnvironment{
		ctx:           context.Background(),
		dataRoot:      dataA,
		workspaceRoot: workspaceA,
		resolveGlobalAsset: func(ctx context.Context, relative string) (string, bool, error) {
			return resolveTransactionGlobalAsset(ctx, dataA, workspaceA, relative)
		},
	}
	if err := (&Transaction{environment: env}).processGlobalAssets(tree); err != nil {
		t.Fatal(err)
	}

	moved, err := os.ReadFile(filepath.Join(localAssets, "captured.png"))
	if err != nil || string(moved) != "A" {
		t.Fatalf("captured asset not moved: %q, %v", moved, err)
	}
	if _, err = os.Stat(assetA); !os.IsNotExist(err) {
		t.Fatalf("captured global asset still exists: %v", err)
	}
	untouched, err := os.ReadFile(assetB)
	if err != nil || string(untouched) != "B" {
		t.Fatalf("other root changed: %q, %v", untouched, err)
	}
}

func TestTransactionMoveRejectsMissingClockBeforeDependencies(t *testing.T) {
	env := &transactionEnvironment{
		ctx:      context.Background(),
		parser:   func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) },
		lookup:   func(string) *treenode.BlockTree { return nil },
		load:     func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, errors.New("must not load") },
		stage:    func(*parse.Tree) error { return nil },
		cacheIAL: func(string, map[string]string) {},
		finish:   func(*Transaction) error { return nil },
	}
	err := env.validate([]*Operation{{Action: "move", ID: "source", ParentID: "target"}})
	if err == nil || err.Error() != "transaction_clock_unbound" {
		t.Fatalf("validation error = %v", err)
	}
}
