package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestVersionAndHonestHelp(t *testing.T) {
	for _, args := range [][]string{{"--version", "--json"}, {"--help", "--json"}, {"--json"}, {"status", "--help", "--json"}, {"recover", "-h", "--json"}} {
		var out, errOut bytes.Buffer
		if status := Run(args, &out, &errOut); status != 0 {
			t.Fatalf("%v: status %d", args, status)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true || result["command"] != "wiki-cli" || errOut.Len() != 0 {
			t.Fatalf("bad envelope: %s %s", &out, &errOut)
		}
	}
}

func TestUnknownCommandsNeverClaimStoreSuccess(t *testing.T) {
	for _, args := range [][]string{{"page", "list", "--json"}, {"--root", "--help", "--json"}, {"--version", "extra"}, {"--help", "--json", "--json"}, {"recover", "--root", "--help", "--json"}} {
		var out, errOut bytes.Buffer
		if status := Run(args, &out, &errOut); status != 2 {
			t.Fatalf("%v: status %d", args, status)
		}
		if strings.Contains(out.String(), `"ok":true`) {
			t.Fatalf("unexpected success: %s", &out)
		}
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestOutputFailureHasDedicatedExitStatus(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version", "--json"}, {"unknown", "--json"}} {
		if code := Run(args, failedOutput{}, io.Discard); code != 1 {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
}
