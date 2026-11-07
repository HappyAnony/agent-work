//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestFormatActualOwnerHistoryAndReadOnly(t *testing.T) {
	commit, _, tree, file := transactionHistoryFixture(t)
	c, id := commit.r.owner, tree.ID
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	if e := AutoSpaceContext(context.Background(), id); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	if e := AutoSpaceContext(context.Background(), id); e != nil {
		t.Fatal(e)
	}
	matches, e := filepath.Glob(filepath.Join(util.HistoryDir, "*-format", "*", filepath.Base(file)))
	if e != nil || len(matches) != 1 {
		t.Fatal("format history missing", matches, e)
	}
	raw, e := os.ReadFile(matches[0])
	if e != nil || string(raw) != string(before) {
		t.Fatal("format history lost preimage", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := AutoSpaceContext(ctx, id); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
