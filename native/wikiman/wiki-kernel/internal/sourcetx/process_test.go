package sourcetx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSourceProcessHelper(t *testing.T) {
	mode := os.Getenv("WIKI_TX_TEST_MODE")
	if mode == "" {
		return
	}
	s, err := Open(os.Getenv("WIKI_TX_TEST_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	w, err := s.Writer(ctx, false)
	if mode == "probe" {
		if errors.Is(err, ErrBusy) {
			fmt.Print("busy")
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Close()
		fmt.Print("free")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var request Request
	if name := os.Getenv("WIKI_TX_TEST_REQUEST"); name != "" {
		raw, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(raw, &request); e != nil {
			t.Fatal(e)
		}
	} else {
		request = pair(t, s.Root, "killed")
	}
	_, err = w.Apply(ctx, request, func(point string) error {
		if point == mode {
			fmt.Println("ready")
			for {
				time.Sleep(time.Hour)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessKilledDuringPublishAndCommitIsRecoverable(t *testing.T) {
	for _, point := range []string{"published:0", "committed"} {
		t.Run(point, func(t *testing.T) {
			root := fixture(t)
			s, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			before := sourceHash(t, root)
			child := exec.Command(os.Args[0], "-test.run=^TestSourceProcessHelper$")
			child.Env = append(os.Environ(), "WIKI_TX_TEST_MODE="+point, "WIKI_TX_TEST_ROOT="+root)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr strings.Builder
			child.Stderr = &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if child.ProcessState == nil {
					_ = child.Process.Kill()
					_ = child.Wait()
				}
			})
			ready := make(chan bool, 1)
			go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "ready" }()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("helper failed", stderr.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatal("helper timeout")
			}
			deadline, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			if err := s.Read(deadline, func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("reader passed active writer", err)
			}
			cancel()
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err == nil {
				t.Fatal("helper was not killed")
			}
			if err := s.Read(context.Background(), func() error { return nil }); !errors.Is(err, ErrRecovery) {
				t.Fatal(err)
			}
			w, err := s.Writer(context.Background(), false)
			if err != nil {
				t.Fatal("OS session lock did not release on SIGKILL", err)
			}
			defer w.Close()
			r, err := w.Recover(context.Background(), nil)
			if err != nil || len(r) != 1 || r[0].SourceCommitted != (point == "committed") {
				t.Fatal(r, err)
			}
			if (sourceHash(t, root) == before) == (point == "committed") {
				t.Fatal("wrong post-kill source")
			}
		})
	}
}

func TestCrossProcessSessionAndReadOnlySource(t *testing.T) {
	root := fixture(t)
	_, w := open(t, root)
	child := exec.Command(os.Args[0], "-test.run=^TestSourceProcessHelper$")
	child.Env = append(os.Environ(), "WIKI_TX_TEST_MODE=probe", "WIKI_TX_TEST_ROOT="+root)
	out, err := child.CombinedOutput()
	if err != nil || !strings.HasPrefix(string(out), "busy") {
		t.Fatal(string(out), err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX mode fault requires non-root Unix")
	}
	readOnly := fixture(t)
	if err := os.Chmod(readOnly, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(readOnly, 0700)
	if _, err := Open(readOnly); !errors.Is(err, ErrReadOnly) {
		t.Fatal("unprovisioned read-only root must fail closed", err)
	}
	if _, err := os.Stat(filepath.Join(readOnly, ".local")); !os.IsNotExist(err) {
		t.Fatal("read-only root modified", err)
	}
}

func TestReadOnlyPublishAndInterruptedRecoveryRetainGate(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX permission faults")
	}
	root := fixture(t)
	s, w := open(t, root)
	ctx := context.Background()
	before := sourceHash(t, root)
	pageDir := filepath.Join(root, "pages", p1)
	if err := os.Chmod(pageDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(pageDir, 0700)
	if _, err := w.Apply(ctx, rename(t, root, p1, "blocked", "read-only"), nil); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if sourceHash(t, root) != before {
		t.Fatal("read-only publication changed source")
	}
	if err := os.Chmod(pageDir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	_, err := w.Apply(ctx, pair(t, root, "retry-recover"), func(point string) error {
		if point == "published:1" {
			return errors.New("stop")
		}
		return nil
	})
	if err == nil {
		t.Fatal("missing injected failure")
	}
	_, err = w.Recover(ctx, func(point string) error {
		if point == "recovered:0" {
			return errors.New("interrupt recovery")
		}
		return nil
	})
	if err == nil {
		t.Fatal("recovery did not interrupt")
	}
	if err := s.Read(ctx, func() error { return nil }); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if sourceHash(t, root) != before {
		t.Fatal("second recovery failed rollback")
	}
}

func TestCreatePageThenInterruptedRollbackRemovesOnlyNewEmptyParents(t *testing.T) {
	root := fixture(t)
	_, w := open(t, root)
	ctx := context.Background()
	before := sourceHash(t, root)
	newID := "1760000000000000099"
	raw := bytesAt(t, root, "pages/"+p1+"/page.json")
	raw = []byte(strings.ReplaceAll(string(raw), p1, newID))
	request := Request{OperationID: "new-page", Changes: []Change{{Path: "pages/" + newID + "/page.json", Data: raw}, {Path: "pages/" + newID + "/data/asset.txt", Data: []byte("synthetic new asset")}}}
	if _, err := w.Apply(ctx, request, func(point string) error {
		if point == "published:0" {
			return errors.New("stop")
		}
		return nil
	}); err == nil {
		t.Fatal("did not stop")
	}
	if _, err := w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if sourceHash(t, root) != before {
		t.Fatal("rollback left new source paths")
	}
}
