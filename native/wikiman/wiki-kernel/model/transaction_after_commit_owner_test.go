//go:build fts5

package model

import (
	"context"
	"errors"
	"sync"
	"testing"
	"wiki-kernel/cache"
	"wiki-kernel/internal/flashcard"
)

func TestTransactionAfterCommitOwnerRejectsLateEffects(t *testing.T) {
	c, env, _, _, _ := commitFixture(t)
	effects := []flashcard.AttributeEffect{{ID: "20261104090000-card001", New: map[string]string{"custom-riff-decks": "deck"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := env.afterCommit(ctx, effects, nil); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel lost: %v", e)
	}
	old := Conf
	Conf = &AppConf{}
	e := env.afterCommit(context.Background(), effects, nil)
	Conf = old
	if e == nil {
		t.Fatal("replacement owner accepted effects")
	}
	if got := cache.GetBlockIAL(effects[0].ID); got["custom-riff-decks"] != "" {
		t.Fatal("rejected effect changed cache")
	}
	if e = env.afterCommit(context.Background(), effects, nil); e != nil {
		t.Fatal(e)
	}
	if got := cache.GetBlockIAL(effects[0].ID); got["custom-riff-decks"] != "deck" {
		t.Fatal("captured cache not published")
	}
	if e = c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = env.afterCommit(context.Background(), effects, nil); e == nil {
		t.Fatal("closed owner accepted effects")
	}
}

func TestTransactionAfterCommitFailurePreservesCommittedState(t *testing.T) {
	want := errors.New("publication failed")
	called := 0
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background(), finish: func(*Transaction) error { return nil }, afterCommit: func(context.Context, []flashcard.AttributeEffect, []string) error { called++; return want }}, m: &sync.Mutex{}, flashcardAfterCommit: []flashcard.AttributeEffect{{ID: "id"}}, reloadAfterCommit: []string{"root"}}
	tx.m.Lock()
	tx.state.Store(1)
	if e := tx.commit(); !errors.Is(e, want) || !errors.Is(e, errTransactionAfterCommit) || tx.state.Load() != 2 || called != 1 {
		t.Fatalf("committed outcome lost: %v %d", e, tx.state.Load())
	}
	if len(tx.flashcardAfterCommit) != 0 || len(tx.reloadAfterCommit) != 0 {
		t.Fatal("pending effects retained")
	}
	if !tx.m.TryLock() {
		t.Fatal("commit mutex retained")
	}
	tx.m.Unlock()
}

func TestTransactionAfterCommitReloadReturnsBoundQueryFailure(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := env.afterCommit(ctx, nil, []string{"root"}); !errors.Is(e, context.Canceled) {
		t.Fatalf("reload cancellation lost: %v", e)
	}
	if _, e := env.commitContext.r.main.DB.Exec("DROP TABLE refs"); e != nil {
		t.Fatal(e)
	}
	if e := env.afterCommit(context.Background(), nil, []string{"root", "root"}); e == nil {
		t.Fatal("reload query failure hidden")
	}
}
