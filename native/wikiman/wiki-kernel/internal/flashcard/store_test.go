package flashcard

import (
	"bytes"
	"context"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

const cardID = "20261027000000-card001"
const blockID = "20261027000000-block01"
const otherDeck = "20261027000000-deck001"

func source(m memory) Source {
	return Source{List: func(context.Context) ([]string, error) {
		out := []string{}
		for p := range m {
			if strings.HasSuffix(p, ".deck") {
				out = append(out, p)
			}
		}
		return out, nil
	}, Read: func(_ context.Context, p string) ([]byte, error) { return m.Read(p) }}
}
func seeded(t *testing.T, now time.Time) memory {
	t.Helper()
	m := memory{}
	c := conf.NewFlashcard()
	d, e := riff.LoadDeck(m, BuiltinDeckID, c.RequestRetention, c.MaximumInterval, c.Weights, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	d.AddCard(cardID, blockID)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	return m
}
func TestOriginalStorageCandidatesAndRoundIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Add(-time.Hour)
	clock := func() time.Time { return now }
	files := seeded(t, now)
	settings := *conf.NewFlashcard()
	a, b := Open(source(files), settings, clock), Open(source(files), settings, clock)
	defer a.Close()
	defer b.Close()
	before := clone(files)
	for _, s := range []*Store{a, b} {
		has, e := s.HasBuiltin(ctx)
		if e != nil || !has {
			t.Fatal(has, e)
		}
		n, e := s.Count(ctx, []string{blockID, blockID, "missing"})
		if e != nil || n.Total != 1 || n.New != 1 || n.Due != 1 {
			t.Fatal(n, e)
		}
		cards, e := s.Cards(ctx, BuiltinDeckID, nil)
		if e != nil || len(cards) != 1 {
			t.Fatal(cards, e)
		}
		cards[0].SetDue(now.Add(time.Hour))
		n, e = s.Count(ctx, []string{blockID})
		if e != nil || n.Due != 1 {
			t.Fatal("borrowed card", n, e)
		}
	}
	got, e := a.Due(ctx, BuiltinDeckID, []string{blockID}, nil)
	if e != nil || len(got.Cards) != 1 {
		t.Fatal(got, e)
	}
	if e = a.Skip(ctx, BuiltinDeckID, cardID); e != nil {
		t.Fatal(e)
	}
	got, e = a.Due(ctx, BuiltinDeckID, []string{blockID}, []string{"round-in-progress"})
	if e != nil || len(got.Cards) != 0 {
		t.Fatal("skip lost", got, e)
	}
	got, e = b.Due(ctx, BuiltinDeckID, []string{blockID}, nil)
	if e != nil || len(got.Cards) != 1 {
		t.Fatal("foreign skip", got, e)
	}
	got, e = a.Due(ctx, BuiltinDeckID, []string{}, nil)
	if e != nil || len(got.Cards) != 0 {
		t.Fatal("empty set became all", got, e)
	}
	oldLog := []*riff.Log{{ID: "old", CardID: cardID, Reviewed: now.Add(-time.Hour).Unix()}}
	logPath := "logs/" + now.Format("200601") + ".msgpack"
	files[logPath], _ = msgpack.Marshal(oldLog)
	before = clone(files)
	for _, edit := range []Edit{{Action: "rename", DeckID: BuiltinDeckID, Name: "renamed"}, {Action: "due", DeckID: BuiltinDeckID, CardID: cardID, Due: now.Add(time.Hour)}, {Action: "review", DeckID: BuiltinDeckID, CardID: cardID, Rating: riff.Good}, {Action: "remove-card", DeckID: BuiltinDeckID, CardID: cardID}, {Action: "remove", DeckID: BuiltinDeckID}, {Action: "create", DeckID: otherDeck, Name: "new"}, {Action: "add", DeckID: BuiltinDeckID, CardID: "20261027000001-card002", BlockID: "20261027000001-block02"}} {
		changes, e := a.Plan(ctx, edit)
		if e != nil || len(changes) == 0 {
			t.Fatal(edit, changes, e)
		}
		after := clone(files)
		for _, c := range changes {
			old, exists := files[c.Path]
			if exists != c.Existed || !bytes.Equal(old, c.Before) {
				t.Fatal("wrong preimage", c)
			}
			if c.Remove {
				delete(after, c.Path)
			} else {
				after[c.Path] = c.After
			}
		}
		if !reflect.DeepEqual(files, before) {
			t.Fatal("plan wrote source")
		}
		if edit.Action == "review" {
			var logs []*riff.Log
			if e = msgpack.Unmarshal(after[logPath], &logs); e != nil || len(logs) != 2 || logs[0].ID != "old" {
				t.Fatal(logs, e)
			}
			d, e := a.decode(after, BuiltinDeckID)
			if e != nil || d.GetCard(cardID).GetReps() != 1 {
				t.Fatal(d, e)
			}
		}
	}
	if _, e = a.Plan(ctx, Edit{Action: "rename", DeckID: "../../escape"}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = a.Plan(ctx, Edit{Action: "review", DeckID: BuiltinDeckID, CardID: cardID, Rating: 10}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestTaskCancellationCloseDrainsAndRejectsLateWork(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := Open(Source{List: func(ctx context.Context) ([]string, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return nil, nil
		}
	}, Read: func(context.Context, string) ([]byte, error) { return nil, fs.ErrNotExist }}, *conf.NewFlashcard(), nil)
	done := make(chan error, 1)
	go func() { _, e := s.Decks(context.Background()); done <- e }()
	<-entered
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Decks(cancelled); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("close abandoned accepted task")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	<-closed
	if _, e := s.Decks(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if len(s.decks) != 0 || len(s.files) != 0 || s.source.Read != nil {
		t.Fatal("retained resources")
	}
	s.Close()
}
func TestCorruptAndAbsentStorageDoesNotSynthesizeDeck(t *testing.T) {
	ctx := context.Background()
	m := memory{}
	s := Open(source(m), *conf.NewFlashcard(), nil)
	defer s.Close()
	if has, e := s.HasBuiltin(ctx); e != nil || has || len(m) != 0 {
		t.Fatal(has, e)
	}
	m[BuiltinDeckID+".deck"] = []byte("broken")
	if _, e := s.Decks(ctx); e == nil {
		t.Fatal("corruption ignored")
	}
	now := time.Now()
	valid := seeded(t, now)
	for p, b := range valid {
		m[p] = b
	}
	m[BuiltinDeckID+".cards"], _ = msgpack.Marshal(map[string]interface{}{cardID: nil})
	if _, e := s.Cards(ctx, BuiltinDeckID, nil); e == nil {
		t.Fatal("nil card accepted")
	}
	s.source = Source{List: func(context.Context) ([]string, error) { return nil, fs.ErrPermission }, Read: source(m).Read}
	if _, e := s.Decks(ctx); !errors.Is(e, fs.ErrPermission) {
		t.Fatal(e)
	}
}
