package bazaar

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/util"
)

func testPackageArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("package/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("owned package")); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackageMutationContextStagesCancellationAndFilesystemEffects(t *testing.T) {
	oldTemp := util.TempDir
	util.TempDir = t.TempDir()
	t.Cleanup(func() { util.TempDir = oldTemp })
	installPath := filepath.Join(t.TempDir(), "installed")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := installPackageContext(cancelled, testPackageArchive(t), installPath, "https://github.com/owner/repo@hash"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(installPath); !os.IsNotExist(err) {
		t.Fatal("pre-cancelled install wrote target", err)
	}
	if err := installPackageContext(context.Background(), testPackageArchive(t), installPath, "https://github.com/owner/repo@hash"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(installPath, "README.md"))
	if err != nil || string(data) != "owned package" {
		t.Fatal(string(data), err)
	}
	if err = uninstallPackageContext(cancelled, installPath); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Stat(installPath); err != nil {
		t.Fatal("pre-cancelled uninstall removed target", err)
	}
	if err = uninstallPackageContext(context.Background(), installPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(installPath); !os.IsNotExist(err) {
		t.Fatal("uninstall retained target", err)
	}
	if _, err = downloadPackageContext(context.Background(), "invalid", false, "system"); err == nil {
		t.Fatal("invalid package identity accepted")
	}
}

func TestPackageDownloadCountRequiresExplicitOwner(t *testing.T) {
	called := 0
	scheduler := BackgroundScheduler(func(fn func(context.Context)) bool {
		if fn == nil {
			t.Fatal("nil count callback")
		}
		called++
		return true
	})
	if schedulePackageDownloadCount("owner/repo@hash", "system") {
		t.Fatal("count used an implicit process owner")
	}
	if !schedulePackageDownloadCount("owner/repo@hash", "system", scheduler) || called != 1 {
		t.Fatalf("explicit owner was not used: called=%d", called)
	}
	if schedulePackageDownloadCount("owner/readme.md@hash", "system", scheduler) || schedulePackageDownloadCount("owner/repo@hash", "", scheduler) || called != 1 {
		t.Fatalf("excluded count was scheduled: called=%d", called)
	}
}
