package txqueue

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This is a source boundary check, not a substitute for compiling/running the
// legacy runtime. It deliberately does not import model to test package boot.
func TestOriginalModelHasNoImplicitRuntimeStart(t *testing.T) {
	entries, err := os.ReadDir("../../model")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		name := filepath.Join("../../model", entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "init" {
				t.Errorf("implicit runtime initialization in %s", name)
			}
		}
	}
	for _, name := range []string{"internal/legacy/server.go", "mobile/kernel.go", "harmony/kernel.go"} {
		data, err := os.ReadFile(filepath.Join("../..", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "model.StartLegacyRuntime()") {
			t.Errorf("missing explicit runtime startup: %s", name)
		}
	}
}

func TestOriginalCallersDoNotIgnoreAdmissionErrors(t *testing.T) {
	for _, dir := range []string{"../../model", "../../api"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			name := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				stmt, ok := n.(*ast.ExprStmt)
				if !ok {
					return true
				}
				call, ok := stmt.X.(*ast.CallExpr)
				if !ok {
					return true
				}
				var method string
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					method = fn.Name
				case *ast.SelectorExpr:
					// Only the original model API has this admission contract;
					// unrelated receivers may expose methods with the same names.
					if owner, ok := fn.X.(*ast.Ident); ok && owner.Name == "model" {
						method = fn.Sel.Name
					}
				}
				if method == "PerformTransactions" || method == "createTreeTx" || method == "DuplicateDoc" || method == "resetFlashcards" || method == "ResetFlashcards" || method == "processSyncMergeResult" {
					t.Errorf("ignored admission error in %s", name)
				}
				return true
			})
		}
	}
}
