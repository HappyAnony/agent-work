// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package util

import (
	"os"
	"strings"
)

// WIKI_SINGLE_NOTEBOOK is the env-var name that, when set to a truthy value
// ("1" / "true" / "yes", case-insensitive), enables single-notebook mode.
// CLI flag --single takes precedence over the env var (see Boot()).
const WIKI_SINGLE_NOTEBOOK = "WIKI_SINGLE_NOTEBOOK"

// Single-notebook mode runtime state.
//
// These five vars are written ONCE during boot:
//   - SingleNotebookMode / SingleNotebookName / SingleDocName: set by Boot()
//     after flag.Parse(), from CLI flags + env-var fallback for the bool.
//   - SingleNotebookBoxID / SingleNotebookRootDocID: set by
//     model.EnsureSingleNotebookAndDoc via SetSingleNotebookIDs(), once
//     box/doc election completes.
//
// After the post-boot point they are READ-ONLY for the life of the process.
// Concurrency: writes happen on the boot goroutine before the HTTP request
// goroutines start serving (see main.go SetBooted gate), so no mutex is
// required for the read path.
var (
	SingleNotebookMode      bool
	SingleNotebookName      string
	SingleDocName           string
	SingleNotebookBoxID     string
	SingleNotebookRootDocID string
)

// SetSingleNotebookIDs is the sole writer for SingleNotebookBoxID and
// SingleNotebookRootDocID. Called exactly once from
// model.EnsureSingleNotebookAndDoc after box election + root doc election
// complete successfully.
//
// No validation here — the caller is the sole legitimate source. Empty
// strings are accepted (and would indicate a bootstrap bug; M3 logs and
// aborts before reaching this path).
func SetSingleNotebookIDs(boxID, rootDocID string) {
	SingleNotebookBoxID = boxID
	SingleNotebookRootDocID = rootDocID
}

// coalesceBoolFromEnv returns true if either:
//   - the CLI flag value was true (cliValue == true), or
//   - the named env var is set to a truthy value: "1", "true", or "yes"
//     (case-insensitive, surrounding whitespace trimmed).
//
// Used by Boot() to derive util.SingleNotebookMode from --single (bool flag)
// and WIKI_SINGLE_NOTEBOOK env var. Semantics:
//   - --single (no value) or --single=true                → cliValue=true  → returns true
//   - --single=false  AND  env unset/falsy               → cliValue=false → returns false
//   - --single=false  AND  env=1/true/yes (truthy)       → cliValue=false → returns true (env wins)
//   - flag omitted    AND  env truthy                    → cliValue=false → returns true (env wins)
//   - flag omitted    AND  env unset/falsy/garbage       → cliValue=false → returns false
//
// In practice operators either pass --single or set the env var, not both
// with conflicting values; the "env wins when CLI was explicitly false"
// behavior is benign because nobody writes --single=false in real usage.
// If stricter "CLI absent vs CLI=false" distinction is ever needed, switch
// to flag.Visit() to detect explicit presence.
func coalesceBoolFromEnv(cliValue bool, envName string) bool {
	if cliValue {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envName))) {
	case "1", "true", "yes":
		return true
	}
	return false
}
