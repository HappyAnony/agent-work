package ialcache

import (
	"fmt"
	"sync"
	"testing"

	"github.com/88250/lute/editor"
)

func TestPublishedBlockDrainsOlderAdmissionsBeforeNotification(t *testing.T) {
	a, e := Open(1024)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := Open(1024)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	for i := 0; i < 64; i++ {
		id := fmt.Sprint(i)
		a.PutBlock(id, map[string]string{"name": "pending old"})
		value := map[string]string{"name": "published"}
		a.PublishBlock(id, value)
		value["name"] = "outside"
		if a.GetBlock(id)["name"] != "published" {
			t.Fatal("notification would read old/missing value", id)
		}
		if b.GetBlock(id) != nil {
			t.Fatal("publication crossed owners")
		}
	}
	a.Close()
	a.PublishBlock("late", map[string]string{"name": "late"})
	if a.GetBlock("late") != nil {
		t.Fatal("closed owner reopened")
	}
}

func TestIndependentOwnersDetachedValuesAndClose(t *testing.T) {
	for _, budget := range []int64{-1, 1 << 21} {
		if c, err := Open(budget); err == nil || c != nil {
			t.Fatal("invalid budget accepted")
		}
	}
	a, err := Open(256)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(256)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	input := map[string]string{"name": "original", "line": "a" + editor.IALValEscNewLine + "b"}
	a.PutDoc("same-id", input)
	a.PutBlock("same-id", input)
	input["name"] = "outside"
	a.Wait()
	if got := a.GetDoc("same-id"); got["name"] != "original" || got["line"] != "a\nb" {
		t.Fatal(got)
	}
	if got := a.GetBlock("same-id"); got["line"] != "a"+editor.IALValEscNewLine+"b" {
		t.Fatal(got)
	}
	a.GetBlock("same-id")["name"] = "mutated"
	if a.GetBlock("same-id")["name"] != "original" {
		t.Fatal("shared output map")
	}
	if b.GetDoc("same-id") != nil || b.GetBlock("same-id") != nil {
		t.Fatal("shared owner")
	}
	a.Close()
	a.Close()
	a.PutDoc("late", input)
	a.Wait()
	a.ClearDocs()
	a.ClearBlocks()
	if a.GetDoc("late") != nil || a.GetBlock("same-id") != nil {
		t.Fatal("closed cache usable")
	}
}

func TestConcurrentCloseAndUse(t *testing.T) {
	c, err := Open(128)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				c.PutDoc("id", map[string]string{"v": "x"})
				c.PutBlock("id", map[string]string{"v": "y"})
				c.GetDoc("id")
				c.GetBlock("id")
				c.ClearDocs()
				c.ClearBlocks()
			}
		}()
	}
	c.Close()
	wg.Wait()
}

func TestInvalidateTreeCompletesBufferedWorkAndKeepsOtherOwner(t *testing.T) {
	a, e := Open(0)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := Open(0)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	a.PutDoc("/doc", map[string]string{"value": "a"})
	a.PutBlock("same", map[string]string{"value": "a"})
	b.PutDoc("/doc", map[string]string{"value": "b"})
	b.PublishBlock("same", map[string]string{"value": "b"})
	b.docs.Wait()
	a.InvalidateTree("/doc", []string{"same"})
	if a.GetDoc("/doc") != nil || a.GetBlock("same") != nil {
		t.Fatal("invalidation returned before buffered work")
	}
	a.docs.Wait()
	a.blocks.Wait()
	if a.GetDoc("/doc") != nil || a.GetBlock("same") != nil {
		t.Fatal("old admissions resurrected invalidated cache")
	}
	if b.GetDoc("/doc")["value"] != "b" || b.GetBlock("same")["value"] != "b" {
		t.Fatal("other owner invalidated")
	}
	a.PublishBlock("same", map[string]string{"value": "fresh"})
	if a.GetBlock("same")["value"] != "fresh" {
		t.Fatal("fresh warming rejected")
	}
	a.Close()
	a.InvalidateTree("/doc", []string{"same"})
	if a.GetBlock("same") != nil {
		t.Fatal("closed cache reopened")
	}
}
