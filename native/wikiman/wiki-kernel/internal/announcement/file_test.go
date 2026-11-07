package announcement

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnnouncementFileLedgerRejectsLinksAndCancelsLockWait(t *testing.T) {
	for _, at := range []string{".config", ledgerDir, ledgerPath, ledgerDir + "/.announcement.lock"} {
		t.Run(at, func(t *testing.T) {
			home := t.TempDir()
			outside := t.TempDir()
			os.MkdirAll(filepath.Dir(filepath.Join(home, at)), 0700)
			target := outside
			if at == ledgerPath || at == ledgerDir+"/.announcement.lock" {
				target = filepath.Join(outside, "file")
				os.WriteFile(target, []byte(`[]`), 0600)
			}
			if e := os.Symlink(target, filepath.Join(home, at)); e != nil {
				t.Fatal(e)
			}
			o, _, _ := options(t, home)
			s := open(t, o)
			if e := s.Refresh(context.Background()); !errors.Is(e, ErrLedger) {
				t.Fatal("link accepted", e)
			}
		})
	}
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ledgerDir), 0700)
	l := flock.New(filepath.Join(home, ledgerDir, ".announcement.lock"))
	if e := l.Lock(); e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	o, _, _ := options(t, home)
	s := open(t, o)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.Refresh(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("lock wait not cancellable", e)
	}
}
