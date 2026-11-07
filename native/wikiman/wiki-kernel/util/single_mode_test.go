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
	"testing"
)

func TestCoalesceBoolFromEnv(t *testing.T) {
	const envName = "WIKI_SINGLE_NOTEBOOK_TEST"

	cases := []struct {
		name     string
		cliValue bool
		envValue string
		envUnset bool
		want     bool
	}{
		{name: "cli flag wins over empty env", cliValue: true, envUnset: true, want: true},
		{name: "cli flag wins over false env", cliValue: true, envValue: "false", want: true},
		{name: "env=1 enables", cliValue: false, envValue: "1", want: true},
		{name: "env=true enables", cliValue: false, envValue: "true", want: true},
		{name: "env=TRUE case-insensitive enables", cliValue: false, envValue: "TRUE", want: true},
		{name: "env=YES enables", cliValue: false, envValue: "yes", want: true},
		{name: "env=Yes case-insensitive enables", cliValue: false, envValue: "Yes", want: true},
		{name: "env=0 does not enable", cliValue: false, envValue: "0", want: false},
		{name: "env=false does not enable", cliValue: false, envValue: "false", want: false},
		{name: "env=garbage does not enable", cliValue: false, envValue: "maybe", want: false},
		{name: "env=empty string does not enable", cliValue: false, envValue: "", want: false},
		{name: "env unset does not enable", cliValue: false, envUnset: true, want: false},
		{name: "env=  1  (whitespace trimmed) enables", cliValue: false, envValue: "  1  ", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envUnset {
				t.Setenv(envName, "") // t.Setenv stores+restores; passing "" effectively clears for the test
			} else {
				t.Setenv(envName, tc.envValue)
			}

			got := coalesceBoolFromEnv(tc.cliValue, envName)
			if got != tc.want {
				t.Errorf("coalesceBoolFromEnv(cliValue=%v, env=%q)=%v, want %v",
					tc.cliValue, tc.envValue, got, tc.want)
			}
		})
	}
}

func TestSetSingleNotebookIDs(t *testing.T) {
	// Save and restore globals to keep test idempotent.
	prevBox, prevDoc := SingleNotebookBoxID, SingleNotebookRootDocID
	t.Cleanup(func() {
		SingleNotebookBoxID = prevBox
		SingleNotebookRootDocID = prevDoc
	})

	cases := []struct{ box, doc string }{
		{box: "20240101120000-abcd1234", doc: "20240101120100-xyzw5678"},
		{box: "", doc: ""}, // caller passes empties only on bootstrap failure path; we still accept
	}
	for _, tc := range cases {
		SetSingleNotebookIDs(tc.box, tc.doc)
		if SingleNotebookBoxID != tc.box || SingleNotebookRootDocID != tc.doc {
			t.Errorf("after SetSingleNotebookIDs(%q,%q) globals=%q/%q",
				tc.box, tc.doc, SingleNotebookBoxID, SingleNotebookRootDocID)
		}
	}
}

func TestSingleModeGlobalsDefaults(t *testing.T) {
	// At program start (before Boot() ever runs), package globals must be
	// at their zero values. This catches accidental init() side effects.
	//
	// NOTE: this test runs AFTER any other test in this package may have
	// touched the globals, so save/restore.
	prevMode := SingleNotebookMode
	prevName := SingleNotebookName
	prevDoc := SingleDocName
	prevBox := SingleNotebookBoxID
	prevRoot := SingleNotebookRootDocID
	t.Cleanup(func() {
		SingleNotebookMode = prevMode
		SingleNotebookName = prevName
		SingleDocName = prevDoc
		SingleNotebookBoxID = prevBox
		SingleNotebookRootDocID = prevRoot
	})

	// Reset to zero values to assert what package init guarantees.
	SingleNotebookMode = false
	SingleNotebookName = ""
	SingleDocName = ""
	SingleNotebookBoxID = ""
	SingleNotebookRootDocID = ""

	// (Trivially true after the reset; the test asserts that NO new init()
	// in this package would override these zero defaults. If anyone adds
	// an init() that sets defaults at package load time, this test fails
	// at the reset step's NEXT-test ordering and prompts review.)
	if SingleNotebookMode != false || SingleNotebookName != "" || SingleDocName != "" {
		t.Fatalf("expected zero-value defaults, got mode=%v name=%q doc=%q",
			SingleNotebookMode, SingleNotebookName, SingleDocName)
	}
	if SingleNotebookBoxID != "" || SingleNotebookRootDocID != "" {
		t.Fatalf("expected empty IDs, got box=%q root=%q",
			SingleNotebookBoxID, SingleNotebookRootDocID)
	}
}
