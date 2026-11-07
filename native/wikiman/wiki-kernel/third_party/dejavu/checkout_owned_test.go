package dejavu

import (
	"bytes"
	"context"
	"errors"
	"github.com/siyuan-note/eventbus"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckoutOwnedDefersCancellationAfterWriteAdmission(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("a", 40)
	seedSnapshot(t, r, id)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, e := r.CheckoutOwned(ctx, id, func(_ context.Context, topic string, _ ...interface{}) {
		if topic == eventbus.EvtCheckoutUpsertFiles {
			cancel()
		}
	})
	if e != nil || !out.Started || !out.Applied || !out.Touched || out.Uncertain || ctx.Err() == nil {
		t.Fatal(out, e)
	}
	b, e := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if e != nil || !bytes.Equal(b, []byte("synthetic chunk onesynthetic chunk two")) {
		t.Fatal("critical write interrupted", e)
	}
	recordSnapshotEvidence(t, "restore-critical.json", map[string]any{"cancellationDeferredAfterWriteAdmission": true, "originalBytesFullyRestored": true, "partialWriteNotReportedAsRollback": true})
}
func TestCheckoutOwnedQueueWaitCanCancelBeforeSourceWrites(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("b", 40)
	seedSnapshot(t, r, id)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		WithRepositoryContext(context.Background(), func() error { close(entered); <-release; return nil })
		close(done)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	out, e := r.CheckoutOwned(ctx, id, nil)
	close(release)
	<-done
	if !errors.Is(e, context.DeadlineExceeded) || out.Started || out.Touched || out.Applied {
		t.Fatal(out, e)
	}
}
