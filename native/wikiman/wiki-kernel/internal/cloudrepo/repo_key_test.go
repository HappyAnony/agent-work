package cloudrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/siyuan-note/encryption"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestRepoKeyOriginalDerivationAndValidationBeforePort(t *testing.T) {
	v := conf.AppData{Repo: conf.NewRepo(), Sync: conf.NewSync()}
	v.Repo.Key = bytes.Repeat([]byte{1}, 32)
	calls := 0
	var received []byte
	s, e := New(Options{Load: func(context.Context) (conf.AppData, error) { return v, nil }, Execute: func(context.Context, conf.AppData, Request) (Result, error) { return Result{}, nil }, RepoKey: func(_ context.Context, snapshot conf.AppData, _ RepoKeyRequest, key []byte) (RepoKeyResult, error) {
		calls++
		received = append([]byte{}, key...)
		snapshot.Repo.Key[0] = 9
		return RepoKeyResult{Applied: true}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	for _, kind := range []string{"import", "passphrase"} {
		out, e := s.RepoKey(context.Background(), RepoKeyRequest{Kind: kind, Input: " \u200b" + encoded + " "})
		if e != nil || !out.Applied || !bytes.Equal(received, bytes.Repeat([]byte{7}, 32)) || !bytes.Equal(out.Key, received) {
			t.Fatal("base64 original rule failed")
		}
		if kind == "import" && out.Normalized != encoded {
			t.Fatal("normalization")
		}
	}
	input := "synthetic test passphrase"
	salt := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))[:16]
	expected, e := encryption.KDF(input, salt)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.RepoKey(context.Background(), RepoKeyRequest{Kind: "passphrase", Input: input})
	if e != nil || !bytes.Equal(expected, out.Key) {
		t.Fatal("original KDF mismatch")
	}
	for _, r := range []RepoKeyRequest{{Kind: "import", Input: "invalid"}, {Kind: "import", Input: base64.StdEncoding.EncodeToString([]byte("short"))}, {Kind: "passphrase", Input: " \u200b "}} {
		before := calls
		if _, e = s.RepoKey(context.Background(), r); e == nil || calls != before {
			t.Fatal("invalid key reached mutation")
		}
	}
	a, e := s.RepoKey(context.Background(), RepoKeyRequest{Kind: "random"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.RepoKey(context.Background(), RepoKeyRequest{Kind: "random"})
	if e != nil || len(a.Key) != 32 || len(b.Key) != 32 || bytes.Equal(a.Key, b.Key) {
		t.Fatal("random key")
	}
	s.repoAfterSync = true
	s.lastRepoPurge = time.Now()
	out, e = s.RepoKey(context.Background(), RepoKeyRequest{Kind: "reset"})
	if e != nil || len(out.Key) != 0 || len(received) != 0 || s.repoAfterSync || !s.lastRepoPurge.IsZero() {
		t.Fatal("reset state")
	}
	if v.Repo.Key[0] != 1 {
		t.Fatal("configuration aliased")
	}
}

func TestRepoKeySeparateCapabilityCloseAndPartialOutcome(t *testing.T) {
	options := Options{Load: func(context.Context) (conf.AppData, error) {
		return conf.AppData{Repo: conf.NewRepo(), Sync: conf.NewSync()}, nil
	}, Execute: func(context.Context, conf.AppData, Request) (Result, error) {
		t.Fatal("metadata grants key changes")
		return Result{}, nil
	}}
	no, _ := New(options)
	defer no.Close(context.Background())
	if _, e := no.RepoKey(context.Background(), RepoKeyRequest{Kind: "reset"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	options.RepoKey = func(ctx context.Context, _ conf.AppData, _ RepoKeyRequest, _ []byte) (RepoKeyResult, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return RepoKeyResult{Applied: true, CleanupPending: true}, ctx.Err()
	}
	s, _ := New(options)
	done := make(chan error, 1)
	go func() {
		out, e := s.RepoKey(context.Background(), RepoKeyRequest{Kind: "reset"})
		if !out.Applied || !out.CleanupPending {
			t.Error("partial result discarded")
		}
		done <- e
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	<-canceled
	select {
	case <-done:
		t.Fatal("early completion")
	default:
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
