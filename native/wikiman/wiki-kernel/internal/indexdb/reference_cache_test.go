package indexdb

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestOwnedReferenceCacheCopyTTLReplacementBudgetAndClose(t *testing.T) {
	c := ReferenceCache{Limit: 2}
	r := RefRecord{DefBlockID: "def", BlockID: "ref", Content: "first"}
	if e := c.Put(r); e != nil {
		t.Fatal(e)
	}
	r.Content = "outside"
	v := c.Get("def")
	v[0].Content = "outside"
	if c.Get("def")[0].Content != "first" {
		t.Fatal("aliased")
	}
	_ = c.Put(RefRecord{DefBlockID: "def", BlockID: "ref", Content: "last"})
	_ = c.Put(RefRecord{DefBlockID: "other", BlockID: "ref"})
	if e := c.Put(RefRecord{DefBlockID: "def", BlockID: "new"}); e == nil {
		t.Fatal("total budget not enforced")
	}
	c.mu.Lock()
	e := c.entries["other"]
	e.until = time.Now().Add(-time.Second)
	c.entries["other"] = e
	c.mu.Unlock()
	if len(c.Get("other")) != 0 {
		t.Fatal("expired")
	}
	if e := c.Put(RefRecord{DefBlockID: "def", BlockID: "new"}); e != nil {
		t.Fatal(e)
	}
	c.Delete("def")
	if len(c.Get("def")) != 0 {
		t.Fatal("delete")
	}
	c.Close()
	c.Close()
	if c.Put(r) == nil {
		t.Fatal("closed")
	}
}
func TestReferenceCacheConcurrentOwners(t *testing.T) {
	a, b := &ReferenceCache{}, &ReferenceCache{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = a.Put(RefRecord{DefBlockID: "same", BlockID: fmt.Sprint(i)})
			_ = a.Get("same")
			_ = b.Put(RefRecord{DefBlockID: "same", BlockID: fmt.Sprint(i), Content: "B"})
		}(i)
	}
	wg.Wait()
	a.Clear()
	if len(b.Get("same")) != 20 {
		t.Fatal("owners leaked")
	}
}
