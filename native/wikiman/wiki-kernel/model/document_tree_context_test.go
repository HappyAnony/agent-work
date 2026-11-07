//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestDocumentTreeActualOwnedListingAndFailure(t *testing.T) {
	c, _, _, file, before := referenceModelFixture(t)
	c.persistenceReadOnly = true
	raw, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var labels struct {
		Time map[string]string `json:"_time"`
	}
	if e = json.Unmarshal(raw, &labels); e != nil {
		t.Fatal(e)
	}
	reader := c.business.Notebooks
	c.business = conf.NewBusiness(reader, nil, nil, map[string]map[string]string{c.Lang: labels.Time})
	box := filepath.Base(filepath.Dir(file))
	ctx := context.Background()
	list, total, e := ListDocTreeContext(ctx, box, "/", util.SortModeUnassigned, false, false, 100)
	if e != nil || len(list) != 2 || total != 2 {
		t.Fatal(list, total, e)
	}
	ids, e := DocumentTreeIDsContext(ctx, box, "/")
	if e != nil || len(ids) != 2 {
		t.Fatal(ids, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("listing published source", e)
	}
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	list, total, e = ListDocTreeContext(ctx, box, "/", util.SortModeUnassigned, false, false, 100)
	if e == nil || list != nil || total != 0 {
		t.Fatal("corruption hidden", list, total, e)
	}
	if _, e = DocumentTreeIDsContext(ctx, box, "../outside"); e == nil {
		t.Fatal("source path escaped")
	}
}
