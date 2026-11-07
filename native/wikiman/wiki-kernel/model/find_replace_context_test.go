//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/task"
)

func TestFindReplaceOwnedActualSourceHistorySQLAndFailures(t *testing.T) {
	commit, h, tree, file := transactionHistoryFixture(t)
	c := commit.r.owner
	c.Export = conf.NewExport()
	ctx := context.Background()
	id := tree.Root.FirstChild.ID
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = FindReplaceContext(ctx, "definition", "replacement", map[string]bool{"text": true}, []string{id}, nil, nil, nil, 0, 0, 0); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(file)
	if e != nil || bytes.Equal(before, after) || !bytes.Contains(after, []byte("replacement")) {
		t.Fatal(string(after), e)
	}
	var value string
	if e = commit.r.main.DB.QueryRow("SELECT content FROM blocks WHERE id=?", id).Scan(&value); e != nil || value != "replacement" {
		t.Fatal("replacement SQL not joined", value, e)
	}
	entries, e := os.ReadDir(h.root)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), "-replace") {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(h.root, entry.Name(), tree.Box, tree.Path))
		if e != nil || !bytes.Equal(raw, before) {
			t.Fatal("history did not retain preimage", string(raw), e)
		}
		found = true
	}
	if !found {
		t.Fatal("replacement history missing")
	}
	c.persistenceReadOnly = true
	if e = FindReplaceContext(ctx, "replacement", "no-write", map[string]bool{"text": true}, []string{id}, nil, nil, nil, 0, 0, 0); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = FindReplaceContext(canceled, "replacement", "x", nil, []string{id}, nil, nil, nil, 0, 0, 0); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = FindReplaceContext(ctx, "[", "x", map[string]bool{"text": true}, nil, nil, nil, nil, 3, 0, 0); e == nil {
		t.Fatal("regexp/search error swallowed")
	}
	if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = FindReplaceContext(ctx, "replacement", "x", map[string]bool{"text": true}, []string{id}, nil, nil, nil, 0, 0, 0); e == nil {
		t.Fatal("source failure swallowed")
	}
}

func TestSourceEventPublicationUsesOwnerLifetimeWithoutReentrantLock(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	request, cancel := context.WithCancel(context.Background())
	defer cancel()
	ui, e := captureLegacyDelayedUIOwner(request)
	if e != nil {
		t.Fatal(e)
	}
	source := &legacyAnchorContext{r: env.commitContext.r, commit: env.commitContext}
	events := sourceEventEnvironmentWithOwners(request, context.Background(), source, ui, nil)
	delivered := make(chan error, 1)
	if e = events.enqueue(task.ReloadProtyle, "real-publication", 20*time.Millisecond, func() error {
		e := events.publicationGuard(func() error { return events.pushReload("real-publication") })
		delivered <- e
		return e
	}); e != nil {
		t.Fatal(e)
	}
	cancel()
	select {
	case e := <-delivered:
		if e != nil {
			t.Fatal("accepted publication tied to completed request", e)
		}
	case <-time.After(time.Second):
		t.Fatal("publication reentered owner lock")
	}
	other := *ui
	other.owner = NewAppConf()
	bad := sourceEventEnvironmentWithOwners(context.Background(), context.Background(), source, &other, nil)
	if e = bad.validate(true); e == nil {
		t.Fatal("mixed source/UI owners accepted")
	}
}
