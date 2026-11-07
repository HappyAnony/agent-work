package sourcetx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRustFs2AndGoFlockUseSameStableLockFiles(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native Unix interoperability probe")
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo unavailable; interoperability not verified")
	}
	manifest, err := filepath.Abs("../../../fixtures/source-lock-probe/Cargo.toml")
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "cargo", "build", "--offline", "--quiet", "--manifest-path", manifest, "--target-dir", target)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Rust fs2 probe build: %v %s", err, out)
	}
	root := fixture(t)
	s, w := open(t, root)
	binary := filepath.Join(target, "debug/wiki-source-lock-probe")
	probe := func(name, mode string, want int) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, filepath.Join(root, ".local/locks/"+name+".lock"), mode)
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		got := 0
		if err != nil {
			if failure, ok := err.(*exec.ExitError); ok {
				got = failure.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if got != want {
			t.Fatalf("Rust %s/%s: got %d want %d: %s", name, mode, got, want, out)
		}
	}
	probe("session", "exclusive", 23)
	if err := s.Read(ctx, func() error { probe("source", "shared", 0); probe("source", "exclusive", 23); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := w.exclusive(ctx, false, func(_ *os.Root) error { probe("source", "shared", 23); probe("source", "exclusive", 23); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	probe("session", "exclusive", 0)
}
