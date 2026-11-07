// Package cli adapts argv/stdin to the shared kernel services. It deliberately
// does not import legacy/model/util, and never starts a server or boot worker.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"wiki-kernel/internal/buildinfo"
	"wiki-kernel/internal/wikiops"
)

var commands = []string{"--help", "--version", "read", "page", "content", "query", "edit", "status", "recover"}

const help = `wiki-cli --help | --version [--json]
wiki-cli read --root PATH --store STORE_ID --page PAGE_ID --box BOX_ID --doc DOC_ID [--block BLOCK_ID] [--json]
wiki-cli page --root PATH --request - [--json]
wiki-cli content --root PATH --request - [--json | --output text|ast] [--pandoc ABSOLUTE_EXECUTABLE_OR_ZIP] [--pdf-helper ABSOLUTE_EXECUTABLE]
wiki-cli query --root PATH --request - [--json]
wiki-cli edit --root PATH --request - [--json]
wiki-cli status --root PATH --request - [--json]
wiki-cli recover --root PATH --request - [--json]
page/content/query/edit/status/recover read one strict versioned JSON object from stdin; default output is one JSON envelope.
Common request: protocolVersion:1, storeId; content/query/edit require pageId. Never guesses a current workspace.
query actions: original document/tree/block/reference/search/AV/asset/config reads; index.rebuild rebuilds only disposable indexes.
Query uses {protocolVersion:1,storeId,pageId,action,options:{...}}; same core as authenticated POST /v1/query.
Edit uses {protocolVersion:1,storeId,pageId,operationId,expectedSources,operations}; dryRun previews required source hashes. History uses history:{operationId,redo} instead of operations. Same core as authenticated POST /v1/edit.
page actions: get, list (name query/limit/cursor), apply (versioned complete records), check (default tree or scope:content), index (tree/content/all).
content actions: document.get/list/render/export/transactions/history/restore/put/create/import/rename/move/sort/delete/deleted/recover;
document.version.save/list/get/apply; block.get/create/replace/delete/move; asset.get/list/put/rename/delete; search; check.
Content mutations require operationId and expectedHash (empty=create-only); expectedTreeDigest and dryRun are optional.
Document/block reads accept format:ast|text; --output text|ast emits only that small read projection, incompatible with --json.
document.put accepts native ast and optional pageUpdate/notebookName for atomic docx + notebook creation.
document.create accepts Markdown or DOM in an existing notebook, with optional parentDocId. Original creationMode values: markdown, hpath, daily, duplicate.
document.import accepts base64 data for a .sy document archive into an existing notebook.
document.rename uses title; changing the docx entry also requires expectedPageHash. document.sort takes ordered sibling IDs and the existing sort file hash.
document.delete retains the exact subtree; set cascade:true if descendants exist. document.deleted lists tombstone hashes; document.recover takes that hash.
document.version.save preserves a source version. list/get expose versionId or legacyHistoryPath. apply uses current document CAS, versionHash for legacy history, and expectedAVHashes for changed existing AV files.
document.render returns Protyle BlockDOM; document.transactions accepts supported operations with whole-document expectedHash/CAS.
document.transactions nativeHistory:true permits reversible removal of opaque stable-ID blocks and returns an owned historyId after source and index acknowledgement.
document.history accepts historyId, optional redo, and the current expectedHash; the kernel applies exact journal images without sending document snapshots.
document.restore is a separate legacy byte import (base64 data, max 4 MiB) for an existing document with nonempty expectedHash.
document.export returns self-contained inert HTML with page assets from one read snapshot (8 MiB unique assets, 128 assets); no remote fetch/conversion daemon.
document.export format:docx uses optional --pandoc (trusted local executable/archive), fixed sandboxed arguments and isolated temp HOME; returns base64 data/mediaType/hash, max 16 MiB. No automatic downloads. Native .docj source is unchanged.
document.export format:pdf uses macOS system WebKit via the sibling wiki-pdf helper, or trusted --pdf-helper override; no browser bundle, source mutation, print dialog or network resources. Max 16 MiB PDF; missing helper affects exports only.
block create/move use parentId and optional beforeId; document move uses optional parentDocId and toBoxId within the same page.
asset.put accepts base64 data, max 8 MiB; assetName must be a safe relative name. Reads return base64 in JSON.
page apply uses updates:[{page,expectedHash,entryExpectedHash?}]; get returns hashes. Revision starts at 1, then increments by one.
For docx, page name and entry title must update in one transaction. Soft-delete/restore with deletedAt; include descendants atomically.
status request: {protocolVersion:1,storeId}. Inspects locks, recovery state, index freshness and version directly. Does not initialize a pristine store.
recover request: {protocolVersion:1,storeId,dryRun?}. dryRun:true validates pending recovery and returns ordered rollback/roll-forward target paths, hashes and modes without writes.
Run recover without dryRun to apply recovery and emit receipts. Preview does not reserve the source. Explicit recovery can open a half-published tree and refuses a held session lock.
Exit codes: 0 success, 1 output failure, 2 invalid arguments or operation failure. Machine-readable errors include type, hint and retryable.
Dry-run performs the same locked CAS/AST/graph validation, without staging/journal/source/index writes. It is not a reservation.
Operation IDs replay retained local receipts only; changed payloads conflict. A failure may carry sourceCommitted:true/indexFresh:false.
Help/version never open a source; reads may provision .local runtime locks. No server, GUI, daemon or network is started by CLI.
For an active WikiMan source, save and release its editor session before CLI writes, then resume in WikiMan.
Exact schemas/limits: kernel internal/wikiops/README.md and service request types.
`

// Run returns an exit status; only the command entry calls os.Exit.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			if jsonOutput {
				if emit(stdout, map[string]any{"ok": false, "protocolVersion": buildinfo.ProtocolVersion, "error": wikiops.ErrorInfo(wikiops.ErrArgument)}) != 0 {
					return 1
				}
				return 2
			}
			jsonOutput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) == 0 || len(filtered) == 1 && isHelp(filtered[0]) ||
		len(filtered) == 2 && slices.Contains(commands[2:], filtered[0]) && isHelp(filtered[1]) {
		if jsonOutput {
			return emit(stdout, map[string]any{"ok": true, "protocolVersion": buildinfo.ProtocolVersion, "command": "wiki-cli", "version": buildinfo.Version, "commands": commands, "exitCodes": map[string]string{"0": "success", "1": "output_failure", "2": "argument_or_operation_failure"}})
		}
		_, err := fmt.Fprint(stdout, help)
		if err != nil {
			return 1
		}
		return 0
	}
	if len(filtered) == 1 && filtered[0] == "--version" {
		if jsonOutput {
			return emit(stdout, map[string]any{"ok": true, "protocolVersion": buildinfo.ProtocolVersion, "command": "wiki-cli", "version": buildinfo.Version})
		}
		_, err := fmt.Fprintln(stdout, "wiki-cli "+buildinfo.Version)
		if err != nil {
			return 1
		}
		return 0
	}
	if len(filtered) > 0 && filtered[0] == "read" {
		return read(filtered[1:], jsonOutput, stdout, stderr)
	}
	if len(filtered) > 0 && filtered[0] == "query" {
		return query(filtered[1:], stdin, stdout)
	}
	if len(filtered) > 0 && filtered[0] == "edit" {
		return edit(filtered[1:], stdin, stdout)
	}
	if len(filtered) > 0 && filtered[0] == "page" {
		return pages(filtered[1:], stdin, stdout)
	}
	if len(filtered) > 0 && (filtered[0] == "content" || filtered[0] == "status" || filtered[0] == "recover") {
		if jsonOutput {
			for _, arg := range filtered[1:] {
				if arg == "--output" || strings.HasPrefix(arg, "--output=") {
					if emit(stdout, map[string]any{"ok": false, "protocolVersion": 1, "error": map[string]any{"type": "invalid_argument", "hint": "--json and --output are mutually exclusive", "retryable": false}}) != 0 {
						return 1
					}
					return 2
				}
			}
		}
		return operation(filtered[0], filtered[1:], stdin, stdout)
	}
	if jsonOutput {
		if emit(stdout, map[string]any{"ok": false, "protocolVersion": buildinfo.ProtocolVersion, "error": map[string]any{"type": "invalid_argument", "message": "Unknown command or arguments.", "hint": "Run wiki-cli --help.", "retryable": false}}) != 0 {
			return 1
		}
	} else {
		fmt.Fprintln(stderr, "Unknown command or arguments. Run wiki-cli --help.")
	}
	return 2
}

func isHelp(arg string) bool { return arg == "--help" || arg == "-h" }

func emit(out io.Writer, value any) int {
	if err := json.NewEncoder(out).Encode(value); err != nil {
		return 1
	}
	return 0
}
