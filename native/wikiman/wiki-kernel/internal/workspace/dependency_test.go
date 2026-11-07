package workspace

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDependencyAndWorkerBoundary(t *testing.T) {
	// The extracted AV domain/render graph itself must not reintroduce the old
	// file logger. The broader pre-existing graph still includes filelock's
	// transitive logger, which this AV extraction does not claim to remove.
	avDeps := exec.Command("go", "list", "-deps", "./internal/avmodel", "./internal/avrender", "./internal/docquery", "./internal/doctree", "./internal/docview", "./internal/doclife", "./internal/flashcard", "./internal/flashcard/riff", "./internal/indexrepair")
	avDeps.Dir = "../.."
	avOut, avErr := avDeps.CombinedOutput()
	if avErr != nil {
		t.Fatalf("AV dependency audit: %v %s", avErr, avOut)
	}
	for _, line := range strings.Fields(string(avOut)) {
		if line == "github.com/siyuan-note/logging" || line == "wiki-kernel/util" || line == "wiki-kernel/sql" || line == "wiki-kernel/av" || line == "wiki-kernel/model" {
			t.Errorf("global dependency in AV domain/render graph: %s", line)
		}
	}
	cliDeps := exec.Command("go", "list", "-deps", "./cmd/wiki-cli")
	cliDeps.Dir = "../.."
	cliOut, cliErr := cliDeps.CombinedOutput()
	if cliErr != nil {
		t.Fatalf("CLI dependency audit: %v %s", cliErr, cliOut)
	}
	for _, line := range strings.Fields(string(cliOut)) {
		if strings.HasPrefix(line, "wiki-kernel/internal/handlers/server") || strings.HasPrefix(line, "wiki-kernel/internal/handlers/readhttp") || line == "wiki-kernel/cmd/wiki-kernel" {
			t.Errorf("CLI depends on server transport: %s", line)
		}
	}
	cmd := exec.Command("go", "list", "-deps", "./cmd/wiki-cli", "./cmd/wiki-kernel", "./internal/handlers/readhttp")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dependency audit: %v %s", err, out)
	}
	allowed := map[string]bool{"internal/buildinfo": true, "internal/wikistore": true, "internal/workspace": true, "internal/wikiops": true, "internal/handlers/cli": true, "internal/handlers/readhttp": true, "cmd/wiki-cli": true, "internal/docast": true, "internal/sourcetx": true}
	allowed["cmd/wiki-kernel"] = true
	allowed["internal/handlers/server"] = true
	allowed["internal/indexdb"] = true
	// Reviewed read-only repair plans: explicit DBs, no init, source writes or workers.
	allowed["internal/indexrepair"] = true
	// LocalBus is an explicitly owned synchronous bus, with no legacy ingress,
	// global registration or import-started worker.
	allowed["internal/eventscope"] = true
	allowed["internal/ialcache"] = true
	allowed["internal/nativeparser"] = true
	allowed["conf"] = true
	allowed["internal/pageprefs"] = true      // Original preferences algorithms; explicit owner backend, cancellable gate, no import IO/workers.
	allowed["internal/extensionprefs"] = true // Original snippet/plugin projections; explicit resource ports and owner gate, no import IO/workers.
	allowed["internal/configtypes"] = true
	allowed["internal/nodetype"] = true
	// Audited owned state: no init/workers. OCR commands run only on explicit
	// Configure/owned tasks with bounded CommandContext and temporary input copies.
	allowed["internal/assetcache"] = true
	allowed["internal/assetquery"] = true // Original asset search/detail projection; explicit DB/context, no IO initialization or workers.
	allowed["internal/ocr"] = true
	// Explicit one-job child entrypoint; no init, legacy import or parent global mutation.
	allowed["internal/pdfdevice"] = true
	allowed["internal/assetextract"] = true // Explicit bytes-only converter child; joined cancellation, no import IO/workers or legacy dependencies.
	allowed["internal/graphview"] = true    // Original complete graph assembly, explicit indexes/parser/settings, no workers or source writes.
	allowed["internal/catalogue"] = true    // Original catalogue projections; explicit FS/HTTP, owner cache, joined request workers, no import IO/workers.
	allowed["internal/assetflow"] = true
	allowed["internal/anchor"] = true
	allowed["internal/avquery"] = true
	allowed["internal/avmodel"] = true
	allowed["internal/avrender"] = true
	allowed["internal/markup"] = true
	allowed["internal/blockview"] = true
	allowed["internal/tagquery"] = true
	// Document navigation owns value-only projection and explicit index/card
	// callbacks; the domain graph is audited above for legacy/global imports.
	allowed["internal/docquery"] = true
	allowed["internal/doctree"] = true
	allowed["internal/docview"] = true
	allowed["internal/repoview"] = true      // Snapshot read projections; explicit repository/MIME/cache ports, no import IO/workers or legacy package dependencies.
	allowed["internal/historyindex"] = true  // Read and validate original versions below the pinned page-owned history root.
	allowed["internal/archivepack"] = true   // Original checked ZIP stream; explicit pinned source/output/guard, no import IO/workers or legacy dependencies.
	allowed["internal/workspacestat"] = true // Original statistics/rounding with explicit source and count ports, no import IO/workers or legacy dependencies.
	allowed["internal/workspacefile"] = true // Shared checked file reader with an explicit borrowed source; native admission refuses mutation actions.
	// Original lifecycle candidate preparation: explicit ports, no workers or IO.
	allowed["internal/doclife"] = true
	allowed["internal/editcore"] = true // Original edit algorithms: explicit candidate ports, no source writer or legacy runtime.
	// Explicit owner storage/clock, cancellable gate, no import-time worker or IO.
	allowed["internal/flashcard"] = true
	allowed["internal/flashcard/riff"] = true
	allowed["internal/textsort"] = true
	// Audited original keyword/filename value rules: no IO, init or workers.
	allowed["internal/textprefs"] = true
	// Explicit local listener/upstream/auth ports; New starts no worker or IO.
	allowed["internal/publishsvc"] = true
	allowed["internal/notebooks"] = true    // Explicit pinned source reads and separate repair candidates.
	allowed["internal/cloudrepo"] = true    // Explicit metadata IO and immutable owner snapshot.
	allowed["internal/aiflow"] = true       // Original AI orchestration, private history and explicit SDK/source/UI ports.
	allowed["internal/cloudaccount"] = true // Explicit device/network ports, no workers.
	allowed["internal/settingpack"] = true  // Bounded value codecs for the device account, no import-time IO or workers.
	allowed["internal/reminder"] = true     // Original value/tree preparation, explicit content/time ports; no IO or workers.
	allowed["internal/announcement"] = true // Explicit anonymous feed/device ledger, no import-time IO or workers.
	allowed["internal/appearance"] = true   // Explicit device roots and joined watcher owner.
	allowed["search"] = true
	// Explicit task-owned polling watcher; Stop/Close join all backend workers.
	allowed["internal/pollwatch"] = true
	allowed["internal/pollwatch/backend"] = true
	for _, line := range strings.Fields(string(out)) {
		if !strings.HasPrefix(line, "wiki-kernel/") {
			continue
		}
		relative := strings.TrimPrefix(line, "wiki-kernel/")
		if !allowed[relative] {
			t.Errorf("unreviewed owned dependency on read path: %s", line)
			continue
		}
		dir := filepath.Join("../..", relative)
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
				switch node := n.(type) {
				case *ast.FuncDecl:
					if node.Name.Name == "init" {
						t.Errorf("implicit owned initialization: %s", name)
					}
				case *ast.GoStmt:
					ownedPublish := false
					ownedAppearance := false
					ownedCatalogue := false
					// The catalogue's bounded per-request fan-out cancels siblings
					// on failure and joins every worker before returning. It owns
					// no import-time or persistent background worker.
					if relative == "internal/catalogue" && entry.Name() == "service.go" {
						for _, decl := range file.Decls {
							if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "remote" && fn.Body != nil && node.Pos() >= fn.Body.Pos() && node.End() <= fn.Body.End() {
								ownedCatalogue = true
							}
						}
					}
					if relative == "internal/appearance" && entry.Name() == "owner.go" {
						for _, decl := range file.Decls {
							if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "activate" && fn.Body != nil && node.Pos() >= fn.Body.Pos() && node.End() <= fn.Body.End() {
								ownedAppearance = true
							}
						}
					}
					if relative == "internal/publishsvc" && entry.Name() == "service.go" {
						for _, decl := range file.Decls {
							if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "start" && fn.Body != nil && node.Pos() >= fn.Body.Pos() && node.End() <= fn.Body.End() {
								ownedPublish = true
							}
						}
					}
					if !ownedCatalogue && !ownedAppearance && !ownedPublish && relative != "internal/pollwatch" && relative != "internal/pollwatch/backend" && !(relative == "internal/workspace" && entry.Name() == "task.go") && !(relative == "internal/wikistore" && entry.Name() == "tracker.go") && !(relative == "internal/handlers/readhttp" && entry.Name() == "events.go") && !(relative == "internal/handlers/server" && entry.Name() == "run.go") {
						t.Errorf("unowned goroutine: %s", name)
					}
				case *ast.CallExpr:
					if selector, ok := node.Fun.(*ast.SelectorExpr); ok {
						if id, ok := selector.X.(*ast.Ident); ok {
							if (id.Name == "os" && selector.Sel.Name == "Exit" && relative != "cmd/wiki-cli" && relative != "cmd/wiki-kernel") || strings.HasPrefix(selector.Sel.Name, "Fatal") || strings.HasPrefix(selector.Sel.Name, "LogFatal") {
								t.Errorf("fatal/exit in core: %s", name)
							}
						}
					}
				}
				return true
			})
		}
	}
}
