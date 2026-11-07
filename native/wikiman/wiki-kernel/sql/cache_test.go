package sql

import (
	"testing"

	"wiki-kernel/search"
)

func TestLegacySQLCacheExplicitLifecycle(t *testing.T) {
	if legacyCaches.blocks != nil || legacyCaches.refs != nil {
		t.Fatal("import started caches")
	}
	if err := OpenLegacyCaches(); err != nil {
		t.Fatal(err)
	}
	defer CloseLegacyCaches()
	first := legacyCaches.blocks
	if err := OpenLegacyCaches(); err != nil || first != legacyCaches.blocks {
		t.Fatal("duplicate owner", err)
	}
	enableCache()
	block := &Block{ID: "same-id", Content: search.SearchMarkLeft + "original" + search.SearchMarkRight}
	putBlockCache(block)
	block.Content = "outside"
	got := getBlockCache("same-id")
	if got == nil || got.Content != "original" {
		t.Fatal(got)
	}
	got.Content = "mutated"
	if getBlockCache("same-id").Content != "original" {
		t.Fatal("shared result")
	}
	ref := &Ref{DefBlockID: "same-id", BlockID: "source", Content: "original"}
	putRefCache(ref)
	ref.Content = "outside"
	refs := GetRefsCacheByDefID("same-id")
	if len(refs) != 1 || refs[0].Content != "original" {
		t.Fatal(refs)
	}
	refs[0].Content = "mutated"
	if GetRefsCacheByDefID("same-id")[0].Content != "original" {
		t.Fatal("shared ref")
	}
	// TTL/expiry are tested against the same shared core with a controlled
	// expired entry; the legacy facade still owns deletion and close.
	removeRefCacheByDefID("same-id")
	if len(legacyCaches.refs.Get("same-id")) != 0 {
		t.Fatal("reference cache not deleted")
	}
	ClearBlockCache()
	if getBlockCache("same-id") != nil {
		t.Fatal("clear failed")
	}
	CloseLegacyCaches()
	CloseLegacyCaches()
	if getBlockCache("same-id") != nil {
		t.Fatal("cache usable after close")
	}
}
