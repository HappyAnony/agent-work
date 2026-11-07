//go:build fts5

package sourcetx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestR05DocumentArchiveHardKillRollsBackOrCompletesWholeManifest(t *testing.T) {
	for _, point := range []string{"prepared", "published:0", "published:2", "committed"} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			const box = "20261019090000-box0001"
			const doc = "20261105090100-doc0001"
			const child = "20261105090101-doc0002"
			prefix := "pages/" + p2 + "/data/"
			a, b := box+"/"+doc+".docj", box+"/"+doc+"/"+child+".docj"
			rawA := []byte(`{"Type":"NodeDocument","ID":"` + doc + `","Opaque":900719925474099312345,"Children":[]}`)
			rawB := []byte(`{"Type":"NodeDocument","ID":"` + child + `","Children":[]}`)
			if e := os.MkdirAll(filepath.Join(root, prefix, box, doc), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(root, prefix+a), rawA, 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(root, prefix+b), rawB, 0600); e != nil {
				t.Fatal(e)
			}
			s, e := Open(root)
			if e != nil {
				t.Fatal(e)
			}
			_, guard, e := s.PageReadGuards(ctx, p2)
			if e != nil {
				t.Fatal(e)
			}
			tomb := DocumentTombstone{Version: 1, PageID: p2, BoxID: box, DocID: doc, OriginalPath: a, DeletedAt: "2026-11-05T01:02:03Z", Files: []DeletedFile{{a, Hash(rawA)}, {b, Hash(rawB)}}}
			record, _ := json.Marshal(tomb)
			q := Request{OperationID: "r05-kill-delete", ReadGuards: guard, Outcome: &Outcome{DocID: doc, Hash: Hash(record)}, Changes: []Change{
				{Path: tomb.RecordPath(), Data: record}, {Path: prefix + a, ExpectedHash: Hash(rawA), Delete: true}, {Path: prefix + b, ExpectedHash: Hash(rawB), Delete: true},
				{Path: tomb.ArchivePath(a), Data: rawA}, {Path: tomb.ArchivePath(b), Data: rawB},
			}}
			request, _ := json.Marshal(q)
			file := filepath.Join(t.TempDir(), "request.json")
			if e = os.WriteFile(file, request, 0600); e != nil {
				t.Fatal(e)
			}
			process := exec.Command(os.Args[0], "-test.run=^TestSourceProcessHelper$")
			process.Env = append(os.Environ(), "WIKI_TX_TEST_MODE="+point, "WIKI_TX_TEST_ROOT="+root, "WIKI_TX_TEST_REQUEST="+file)
			stdout, e := process.StdoutPipe()
			if e != nil {
				t.Fatal(e)
			}
			if e = process.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if process.ProcessState == nil {
					_ = process.Process.Kill()
					_ = process.Wait()
				}
			}()
			ready := make(chan bool, 1)
			go func() { sc := bufio.NewScanner(stdout); ready <- sc.Scan() && sc.Text() == "ready" }()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("process did not reach kill point")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("kill point timeout")
			}
			if e = process.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e = process.Wait(); e == nil {
				t.Fatal("worker survived")
			}
			if e = s.Read(ctx, func() error { return nil }); !errors.Is(e, ErrRecovery) {
				t.Fatal(e)
			}
			w, e := s.Writer(ctx, false)
			if e != nil {
				t.Fatal(e)
			}
			defer w.Close()
			result, e := w.Recover(ctx, nil)
			committed := point == "committed"
			if e != nil || len(result) != 1 || result[0].SourceCommitted != committed || result[0].Outcome == nil || result[0].Outcome.Hash != Hash(record) {
				t.Fatal(result, e)
			}
			read := func(name string, want []byte) {
				t.Helper()
				raw, e := os.ReadFile(filepath.Join(root, name))
				if want == nil {
					if !errors.Is(e, fs.ErrNotExist) {
						t.Fatal(name, e)
					}
					return
				}
				if e != nil || !bytes.Equal(raw, want) {
					t.Fatal(name, e)
				}
			}
			if committed {
				read(prefix+a, nil)
				read(prefix+b, nil)
				read(tomb.ArchivePath(a), rawA)
				read(tomb.ArchivePath(b), rawB)
				read(tomb.RecordPath(), record)
			} else {
				read(prefix+a, rawA)
				read(prefix+b, rawB)
				read(tomb.ArchivePath(a), nil)
				read(tomb.ArchivePath(b), nil)
				read(tomb.RecordPath(), nil)
			}
		})
	}
}
