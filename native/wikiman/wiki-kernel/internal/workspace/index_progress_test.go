//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalSQLProgressUsesOwnerLanguageAndExplicitSink(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(nil, map[string]map[int]string{"en_US": {90: page + " blocks %d/%d %d %s", 93: page + " delete %d/%d %s", 234: page + " rename %d/%d %d %s", 191: page + " history %d/%d", 217: page + " asset %d/%d"}}, nil), nil
	}})
	original, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	messages := make(chan string, 16)
	sub, err := original.SubscribeOriginalSQLProgress(ctx, conf.SQLProgressPolicy{DeleteBlocks: true, InsertBlocks: true}, func(e IndexEvent, message string) {
		if e.PageID != pageA || e.StoreID != storeID {
			t.Error(e)
		}
		messages <- message
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	otherMessages := 0
	other, err := b.SubscribeOriginalSQLProgress(ctx, conf.SQLProgressPolicy{DeleteBlocks: true}, func(IndexEvent, string) { otherMessages++ })
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(ctx)
	// The callback does not capture the creating lease or consult whichever tab
	// is active. Its explicit Context remains alive until the test closes it.
	original.Release()
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if err = a.QueueNativeHistories(ctx, []indexdb.HistoryEntry{{ID: "history"}}); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeHistories(ctx); err != nil {
		t.Fatal(err)
	}
	if err = a.QueueNativeAssetContents(ctx, []indexdb.AssetContentEntry{{ID: "asset", Path: "assets/a.pdf"}}); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeAssetContents(ctx); err != nil {
		t.Fatal(err)
	}
	for _, op := range []indexdb.MainMutation{{Action: "delete_id", ID: "first"}, {Action: "rename", ID: "root", HPath: "/title"}, {Action: "delete_id", ID: "second"}} {
		if err = a.QueueNativeMainMutation(ctx, op); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	expected := []string{pageA + " history 1/1", pageA + " asset 1/1", pageA + " delete 1/2 first", pageA + " rename 1/1 1 ", pageA + " delete 2/2 second"}
	for i, prefix := range expected {
		select {
		case message := <-messages:
			if !strings.HasPrefix(message, prefix) {
				t.Fatalf("message %d %q want prefix %q", i, message, prefix)
			}
			if strings.HasPrefix(prefix, pageA+" rename") && len(strings.TrimPrefix(message, prefix)) != 7 {
				t.Fatal("original rename hash missing", message)
			}
		default:
			t.Fatal("missing real business progress", i)
		}
	}
	if otherMessages != 0 {
		t.Fatal("A progress sent to B sink", otherMessages)
	}
	if err = sub.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = a.QueueNativeHistories(ctx, []indexdb.HistoryEntry{{ID: "after-close"}}); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeHistories(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		t.Fatal("closed sink invoked", message)
	default:
	}
	noBusiness := open(t, fixture(t), Options{})
	l, err := noBusiness.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if _, err = l.SubscribeOriginalSQLProgress(ctx, conf.SQLProgressPolicy{}, func(IndexEvent, string) {}); !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal(err)
	}
}
