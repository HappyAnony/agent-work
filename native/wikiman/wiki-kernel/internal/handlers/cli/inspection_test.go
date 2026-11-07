package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikiops"
)

func TestLocalStatusAndRecoveryPreview(t *testing.T) {
	ctx := context.Background()
	const id = "f36f1884-2299-4fde-88c1-9da8f26d2aac"
	const asset = "pages/1760000000000000002/data/assets/recovery-preview.txt"
	for _, tc := range []struct{ crash, direction string }{
		{"published:0", "roll_back"},
		{"committed", "roll_forward"},
	} {
		t.Run(tc.direction, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
				t.Fatal(err)
			}
			type envelope struct {
				OK     bool
				Result json.RawMessage
				Error  wikiops.ServiceError
			}
			run := func(command, request string, wantCode int, kind string, result any) {
				t.Helper()
				var out, stderr bytes.Buffer
				code := RunWithInput([]string{command, "--root", root, "--request=-"}, strings.NewReader(request), &out, &stderr)
				var e envelope
				if code != wantCode || stderr.Len() != 0 || json.Unmarshal(out.Bytes(), &e) != nil ||
					e.OK != (wantCode == 0) || e.Error.Type != kind || (e.Error.Retryable != (kind == "store_busy")) {
					t.Fatalf("%s returned %d: %s (%s)", command, code, out.String(), stderr.String())
				}
				if result != nil && json.Unmarshal(e.Result, result) != nil {
					t.Fatalf("invalid result: %s", e.Result)
				}
			}
			request := `{"protocolVersion":1,"storeId":"` + id + `"}`
			previewRequest := strings.TrimSuffix(request, "}") + `,"dryRun":true}`
			var status wikiops.StatusResult
			var preview wikiops.RecoverResult
			run("status", request, 0, "", &status)
			run("recover", previewRequest, 0, "", &preview)
			if status.SessionLock != "uninitialized" || status.Recovery != "clear" ||
				status.Version == "" || status.TreeIndex != "missing" || preview.Preview == nil || len(*preview.Preview) != 0 {
				t.Fatal(status, preview)
			}
			if _, err := os.Stat(filepath.Join(root, ".local")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("pristine diagnostic wrote runtime files", err)
			}
			source, err := sourcetx.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			writer, err := source.Writer(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			run("status", request, 0, "", &status)
			if status.SessionLock != "held" || status.SourceLock != "readable" {
				t.Fatal(status)
			}
			run("recover", request, 2, "store_busy", nil)
			interrupted := errors.New("interrupted")
			_, err = writer.Apply(ctx, sourcetx.Request{OperationID: "preview-witness", Changes: []sourcetx.Change{
				{Path: asset, Data: []byte("r08-preview")},
			}}, func(point string) error {
				if point == tc.crash {
					return interrupted
				}
				return nil
			})
			if !errors.Is(err, interrupted) {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			before := inspectionSnapshot(t, root)
			run("status", request, 0, "", &status)
			run("recover", previewRequest, 0, "", &preview)
			if status.Recovery != "required" || status.SessionLock != "available" ||
				preview.Preview == nil || len(*preview.Preview) != 1 || len(preview.Receipts) != 0 {
				t.Fatal(status, preview)
			}
			step := (*preview.Preview)[0]
			if step.Action != tc.direction || len(step.Files) != 1 || step.Files[0].Path != asset {
				t.Fatal(step)
			}
			if !reflect.DeepEqual(before, inspectionSnapshot(t, root)) {
				t.Fatal("inspection mutated source or runtime bytes")
			}
			target := filepath.Join(root, asset)
			if err := os.WriteFile(target, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			conflict := inspectionSnapshot(t, root)
			run("recover", previewRequest, 2, "source_conflict", nil)
			run("recover", request, 2, "source_conflict", nil)
			if !reflect.DeepEqual(conflict, inspectionSnapshot(t, root)) {
				t.Fatal("conflict changed source or runtime bytes")
			}
			if err := os.WriteFile(target, []byte("r08-preview"), 0600); err != nil {
				t.Fatal(err)
			}
			var recovered wikiops.RecoverResult
			run("recover", request, 0, "", &recovered)
			if len(recovered.Receipts) != 1 || recovered.Receipts[0].OperationID != step.OperationID ||
				recovered.Receipts[0].SourceCommitted != (step.Action == "roll_forward") || !recovered.Receipts[0].IndexFresh {
				t.Fatal(step, recovered)
			}
			if step.Files[0].TargetHash == "" {
				if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("rollback retained target", err)
				}
			} else if raw, err := os.ReadFile(target); err != nil || sourcetx.Hash(raw) != step.Files[0].TargetHash {
				t.Fatal("published hash differs from preview", err)
			}
			run("status", request, 0, "", &status)
			if status.Recovery != "clear" || status.TreeIndex != "fresh" || status.ContentIndex != "fresh" {
				t.Fatal(status)
			}
			doc := filepath.Join(root, "pages/1760000000000000002/data/20261019090000-box0001/20261019090001-doc0001.docj")
			raw, err := os.ReadFile(doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(doc, append(raw, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			run("status", request, 0, "", &status)
			if status.TreeIndex != "fresh" || status.ContentIndex != "stale" {
				t.Fatal(status)
			}
		})
	}
}

func inspectionSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(os.DirFS(root), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[name] = "dir"
			return nil
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		out[name] = sourcetx.Hash(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
