//go:build fts5

package wikiops

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"

	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/sourcetx"
)

func TestR05ArchiveImportPublishesDocumentResourcesAndAVTogether(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		v, e := Content(ctx, m, r)
		if e != nil {
			t.Fatal(r.Action, e)
		}
		return v
	}
	seed := baseContent(id, "document.put")
	seed.NotebookName, seed.OperationID, seed.AST = "Archive target", "r05-archive-notebook", documentAST(newDoc, newBlock, "Destination", "Outside")
	call(seed)
	const parent, child = "20261019100010-doc0010", "20261019100011-doc0011"
	const block, mirror, avID = "20261019100012-par0012", "20261019100013-mir0013", "20261019100014-avi0014"
	doc := []byte(`{"ID":"` + parent + `","Type":"NodeDocument","Spec":"1","Properties":{"title":"Archive parent","custom":{"large":900719925474099312345}},"Children":[{"ID":"` + mirror + `","Type":"NodeAttributeView","AttributeViewID":"` + avID + `","AttributeViewType":"table"}]}`)
	descendant := []byte(`{"ID":"` + child + `","Type":"NodeDocument","Spec":"1","Properties":{"title":"Archive child"},"Children":[{"ID":"` + block + `","Type":"NodeParagraph","Children":[{"Type":"NodeTextMark","TextMarkType":"block-ref","TextMarkBlockRefID":"` + parent + `","TextMarkBlockRefSubtype":"d","TextMarkTextContent":"Archive parent"}]}]}`)
	view := []byte(`{"spec":4,"id":"` + avID + `","name":"Archive database","viewID":"view","keyValues":[{"key":{"id":"primary","name":"Name","type":"block"},"values":[{"id":"value","blockID":"` + newBlock + `","keyID":"primary","type":"block","block":{"id":"` + newBlock + `","content":"Outside"}}]}],"views":[{"id":"view","name":"Table","type":"table","table":{"id":"table-layout","columns":[{"id":"primary"}]}}]}`)
	archive := func(files map[string][]byte) []byte {
		t.Helper()
		var out bytes.Buffer
		z := zip.NewWriter(&out)
		for name, raw := range files {
			f, e := z.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.Write(raw); e != nil {
				t.Fatal(e)
			}
		}
		if e := z.Close(); e != nil {
			t.Fatal(e)
		}
		return out.Bytes()
	}
	r := baseContent(id, "document.import")
	r.DocID, r.ParentDocID, r.OperationID = "", newDoc, "r05-archive-import"
	files := map[string][]byte{
		"Export/" + parent + ".docj":                  doc,
		"Export/" + parent + "/" + child + ".docj":    descendant,
		"Export/" + parent + "/assets/descendant.txt": []byte("local bytes"),
		"Export/assets/shared.txt":                    []byte("attachment bytes"),
		"Export/emojis/synthetic.png":                 []byte("emoji bytes"),
		"Export/storage/av/" + avID + ".json":         view,
		"Export/.siyuan/sort.json":                    []byte(`{"` + parent + `":3,"` + child + `":1}`),
	}
	cfg := *conf.NewFlashcard()
	deck, e := flashcard.NewDeck(flashcard.BuiltinDeckID, "Archive cards", cfg, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	deck.AddCard("20261019100015-crd0015", block)
	tree := &parse.Tree{ID: child, Root: &ast.Node{ID: child, Type: ast.NodeDocument}}
	tree.Root.AppendChild(&ast.Node{ID: block, Type: ast.NodeParagraph})
	cardArchive, e := flashcard.PrepareExport(ctx, []*parse.Tree{tree}, flashcard.ArchiveServices{
		Settings: cfg, Now: time.Now,
		Cards: func(_ context.Context, ids []string) ([]riff.Card, error) { return deck.GetCardsByBlockIDs(ids), nil },
	})
	if e != nil || cardArchive.Count != 1 {
		t.Fatal(cardArchive, e)
	}
	for name, raw := range cardArchive.Files {
		files["Export/"+name] = raw
	}
	r.Data = archive(files)
	base := filepath.Join(root, "pages", emptyPage, "data")
	put := ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: emptyPage, Action: "asset.put", AssetName: "shared.txt", OperationID: "r05-archive-collision-seed", Data: []byte("conflict")}
	call(put)
	if _, e := Content(ctx, m, r); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal("resource overwrite accepted", e)
	}
	if _, e := os.Stat(filepath.Join(base, "storage", "av")); !os.IsNotExist(e) {
		t.Fatal("failed import published a partial AV")
	}
	put.OperationID, put.ExpectedHash, put.Data = "r05-archive-collision-reset", sourcetx.Hash(put.Data), []byte("attachment bytes")
	call(put)
	r.DryRun = true
	if result := call(r); result.Receipt.SourceCommitted {
		t.Fatal(result)
	}
	if _, e := os.Stat(filepath.Join(base, "emojis", "synthetic.png")); !os.IsNotExist(e) {
		t.Fatal("preview published resources")
	}
	r.DryRun = false
	result := call(r)
	if !result.OriginalIndexFresh || result.DocID == parent || !strings.HasPrefix(result.Path, box+"/"+newDoc+"/") {
		t.Fatal(result)
	}
	read := func(name string) []byte {
		t.Helper()
		raw, e := os.ReadFile(filepath.Join(base, filepath.FromSlash(name)))
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	if raw := read(result.Path); !bytes.Contains(raw, []byte("900719925474099312345")) || bytes.Contains(raw, []byte(avID)) {
		t.Fatal(string(raw))
	}
	var registry map[string][]string
	if e := msgpack.Unmarshal(read("storage/av/blocks.msgpack"), &registry); e != nil || len(registry) != 1 {
		t.Fatal(registry, e)
	}
	for newid := range registry {
		if newid == avID || !bytes.Contains(read("storage/av/"+newid+".json"), []byte("Archive database")) ||
			!bytes.Contains(read(box+"/"+newDoc+".docj"), []byte(newid)) {
			t.Fatal("imported AV and external binding diverged", newid)
		}
	}
	var ranks map[string]int
	if e := json.Unmarshal(read(box+"/.wiki/sort.json"), &ranks); e != nil || ranks[result.DocID] != 3 {
		t.Fatal(ranks, e)
	}
	for docID, rank := range ranks {
		if rank != 1 {
			continue
		}
		raw := read(path.Join(strings.TrimSuffix(result.Path, ".docj"), docID+".docj"))
		if !bytes.Contains(raw, []byte(result.DocID)) || !bytes.Contains(raw, []byte("custom-riff-decks")) {
			t.Fatal("child reference or flashcard was not remapped", string(raw))
		}
	}
	for name := range cardArchive.Files {
		if len(read(name)) == 0 {
			t.Fatal("archive flashcards not saved", name)
		}
	}
	if !bytes.Equal(read(strings.TrimSuffix(result.Path, ".docj")+"/assets/descendant.txt"), []byte("local bytes")) {
		t.Fatal("document attachment changed")
	}
	if replay := call(r); replay.DocID != result.DocID || replay.Hash != result.Hash {
		t.Fatal(replay)
	}
	r.OperationID, r.Data = "r05-archive-traversal", archive(map[string][]byte{"Export/../escape": []byte("invalid")})
	if _, e := Content(ctx, m, r); !errors.Is(e, ErrArgument) {
		t.Fatal("archive path traversal accepted", e)
	}
}
