package cache

import "testing"

func TestLegacyIALHasExplicitOwner(t *testing.T) {
	if legacyIAL.cache != nil {
		t.Fatal("package import opened IAL workers")
	}
	PutDocIAL("ignored", map[string]string{"v": "not-started"})
	if GetDocIAL("ignored") != nil {
		t.Fatal("implicit cache initialization")
	}
	if err := OpenLegacyIAL(); err != nil {
		t.Fatal(err)
	}
	defer CloseLegacyIAL()
	first := legacyIAL.cache
	if err := OpenLegacyIAL(); err != nil || first != legacyIAL.cache {
		t.Fatal("duplicate cache owner", err)
	}
	PutDocIAL("same-id", map[string]string{"v": "doc"})
	PutBlockIAL("same-id", map[string]string{"v": "block"})
	first.Wait()
	if GetDocIAL("same-id")["v"] != "doc" || GetBlockIAL("same-id")["v"] != "block" {
		t.Fatal("cache adapter changed values")
	}
	CloseLegacyIAL()
	CloseLegacyIAL()
	if GetDocIAL("same-id") != nil || GetBlockIAL("same-id") != nil {
		t.Fatal("closed adapter retained values")
	}
}

func TestCapturedBlockIALPublisherStaysWithOriginalCache(t *testing.T) {
	if err := OpenLegacyIAL(); err != nil {
		t.Fatal(err)
	}
	publish := CaptureBlockIALPublisher()
	publish("captured", map[string]string{"name": "original"})
	if GetBlockIAL("captured")["name"] != "original" {
		t.Fatal("synchronous publication missing")
	}
	CloseLegacyIAL()
	if err := OpenLegacyIAL(); err != nil {
		t.Fatal(err)
	}
	defer CloseLegacyIAL()
	publish("captured", map[string]string{"name": "must not leak"})
	if GetBlockIAL("captured") != nil {
		t.Fatal("captured publisher followed replacement cache")
	}
}
