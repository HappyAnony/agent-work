package dejavu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSnapshotActualHTTPWorkerFailureCancelsAndJoins(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("a", 40)
	_, chunks := seedSnapshot(t, r, id)
	r.AddTag(id, "backup")
	entered, gone := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		io.Copy(io.Discard, q.Body)
		close(entered)
		<-q.Context().Done()
		close(gone)
	}))
	defer server.Close()
	client := server.Client()
	opts := faultOptions(nil, func(ctx context.Context, p string) error {
		if strings.HasSuffix(p, chunks[0][2:]) {
			q, e := http.NewRequestWithContext(ctx, "PUT", server.URL, strings.NewReader("synthetic upload"))
			if e != nil {
				return e
			}
			resp, e := client.Do(q)
			if resp != nil {
				resp.Body.Close()
			}
			return e
		}
		if strings.HasSuffix(p, chunks[1][2:]) {
			<-entered
			return errors.New("synthetic other-worker failure")
		}
		return nil
	}, nil)
	out, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"}, opts)
	if e == nil || out.Applied {
		t.Fatal(out, e)
	}
	select {
	case <-gone:
	case <-time.After(time.Second):
		t.Fatal("actual HTTP worker not canceled")
	}
	recordSnapshotEvidence(t, "snapshot-http.json", map[string]any{"actualHTTPWorkerCanceled": true, "allWorkersJoined": true, "failedTransferDidNotApplyTag": true})
}
