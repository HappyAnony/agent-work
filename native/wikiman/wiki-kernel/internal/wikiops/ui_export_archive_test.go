//go:build fts5

package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"wiki-kernel/internal/wikistore"
)

func TestR06SingleDocumentArchiveRejectsStaleEntry(t *testing.T) {
	_, manager, store := setupPages(t)
	entry := wikistore.EntryDocument{BoxID: box, DocID: newDoc}
	data, _ := json.Marshal(map[string]string{"id": entry.DocID})
	_, err := UI(context.Background(), manager, UIRequest{
		ProtocolVersion: 1, StoreID: store, PageID: testPage,
		Endpoint: "/api/export/exportMd", Data: data,
	}, UIView{Mode: "docxSingle", Entry: &entry})
	if !errors.Is(err, ErrStaleSession) {
		t.Fatal(err)
	}
}
