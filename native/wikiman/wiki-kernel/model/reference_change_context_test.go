//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func referenceMutationPresentation(t *testing.T, c *AppConf) {
	labels := map[string]interface{}{}
	for _, key := range []string{"now", "1s", "xs", "1m", "xm", "1h", "xh", "1d", "xd", "1w", "xw", "1M", "xM", "1y", "2y", "xy", "max", "albl", "blbl"} {
		labels[key] = key
	}
	old := util.TimeLangs
	util.TimeLangs = map[string]map[string]interface{}{c.Lang: labels}
	t.Cleanup(func() { util.TimeLangs = old })
}

func TestReferenceTransferActualSourceAndIndexJoin(t *testing.T) {
	c, r, def, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	referenceMutationPresentation(t, c)
	ctx := context.Background()
	to := "20261028140001-refdoc1"
	if e := TransferBlockRefContext(ctx, def, to, nil); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || bytes.Equal(before, raw) || !strings.Contains(string(raw), to) {
		t.Fatal("source transfer missing", string(raw), e)
	}
	refs, e := (indexdb.References{DB: r.main.DB}).RefIDs(ctx, to, false)
	if e != nil || len(refs) != 1 || refs[0] != "20261028140004-refpar1" {
		t.Fatal("derived commit not joined", refs, e)
	}
	before = raw
	c.persistenceReadOnly = true
	if e = TransferBlockRefContext(ctx, to, def, nil); e == nil {
		t.Fatal("readonly published")
	}
	raw, _ = os.ReadFile(file)
	if !bytes.Equal(before, raw) {
		t.Fatal("readonly changed source")
	}
}

func TestReferenceSwapActualCrossDocumentAndRejection(t *testing.T) {
	c, r, def, file, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	referenceMutationPresentation(t, c)
	ctx := context.Background()
	using := "20261028140004-refpar1"
	if e := SwapBlockRefContext(ctx, using, def, false); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || !strings.Contains(string(raw), `"Data":"definition"`) {
		t.Fatal("swap missing", string(raw), e)
	}
	row, e := r.referenceLookup(ctx, def)
	if e != nil || row == nil || row.RootID != "20261028140003-refdoc2" {
		t.Fatal("index not joined", row, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = SwapBlockRefContext(canceled, using, def, false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = SwapBlockRefContext(ctx, "missing", def, false); e == nil {
		t.Fatal("missing source succeeded")
	}
}
