//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"

	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/sourcetx"
)

func TestR04OriginalEditCandidatesShareStateCommitAtomicallyAndPreserveExtensions(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var source map[string]json.RawMessage
	if e = json.Unmarshal(raw, &source); e != nil {
		t.Fatal(e)
	}
	source["R04Future"] = json.RawMessage(`{"huge":900719925474099312345}`)
	raw, _ = json.Marshal(source)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	op := []*editcore.Operation{{Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"candidate"}`}, {Action: "update", ID: blockID, Data: `<div data-type="NodeParagraph" data-node-id="` + blockID + `"><div contenteditable="true">R04 original edited content</div></div>`}, {Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"final"}`}}
	plan, e := l.PlanNativeEdits(ctx, op)
	l.Release()
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Changes) != 1 || plan.ReadGuards == nil || !strings.Contains(string(plan.Changes[0].Data), "R04 original edited content") || !strings.Contains(string(plan.Changes[0].Data), `"custom-r04":"final"`) || !strings.Contains(string(plan.Changes[0].Data), "900719925474099312345") {
		t.Fatal("bad original plan", len(plan.Changes), string(plan.Changes[0].Data))
	}
	if digest(t, root) != before {
		t.Fatal("candidate preparation wrote source")
	}
	request := sourcetx.Request{OperationID: "r04-source-edit", Changes: plan.Changes, ReadGuards: plan.ReadGuards}
	preview, e := m.Preview(ctx, storeID, request)
	if e != nil || preview.SourceCommitted || digest(t, root) != before {
		t.Fatal("preview changed source", preview, e)
	}
	receipt, e := m.Apply(ctx, storeID, request)
	if e != nil || !receipt.SourceCommitted {
		t.Fatal(receipt, e)
	}
	again, e := m.Apply(ctx, storeID, request)
	if e != nil || !reflect.DeepEqual(receipt, again) {
		t.Fatal("retry changed receipt", again, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || string(after) != string(plan.Changes[0].Data) {
		t.Fatal("actual source mismatch", e)
	}
	b, e := os.ReadFile(filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj"))
	if e != nil || strings.Contains(string(b), "R04 original edited content") {
		t.Fatal("same-ID page leaked", e)
	}
	newLease, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer newLease.Release()
	if _, e = newLease.PrepareNativeReads(ctx, false); e != nil {
		t.Fatal("committed source cannot reindex", e)
	}
}

func TestR04AVConsecutiveCandidatesPreserveUnknownFieldsAndNeverPublishEarly(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261105121000-av00001"
	a := layoutFixture(id, pageA, av.CoverFromNone)
	writeLayoutFixture(t, root, pageA, a)
	file := filepath.Join(root, "pages", pageA, "data", "storage", "av", id+".json")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	fields["r04Future"] = json.RawMessage(`900719925474099312345`)
	raw, _ = json.Marshal(fields)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	plan, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: "setAttrViewName", ID: id, Data: "first"}, {Action: "setAttrViewName", ID: id, Data: "second"}, {Action: "setAttrViewPageSize", AvID: id, Data: float64(27)}})
	l.Release()
	if e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before {
		t.Fatal("AV candidates wrote source")
	}
	if len(plan.Changes) != 1 {
		t.Fatal("wrong AV changes", len(plan.Changes))
	}
	var output map[string]json.RawMessage
	if e = json.Unmarshal(plan.Changes[0].Data, &output); e != nil {
		t.Fatal(e)
	}
	if string(output["r04Future"]) != "900719925474099312345" || string(output["name"]) != `"second"` || !strings.Contains(string(plan.Changes[0].Data), `"pageSize":27`) {
		t.Fatal("lost unknown/consecutive AV changes", string(plan.Changes[0].Data))
	}
	if r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: "r04-av-edit", Changes: plan.Changes, ReadGuards: plan.ReadGuards}); e != nil || !r.SourceCommitted {
		t.Fatal(r, e)
	}
}

func TestR04EditReadDependenciesAndDirectoryPhantomsConflictBeforePublication(t *testing.T) {
	for _, variant := range []string{"file", "phantom"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			m := open(t, root, Options{ReadDefaults: true})
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			plan, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"must-not-publish"}`}})
			l.Release()
			if e != nil {
				t.Fatal(e)
			}
			name := filepath.Join(root, "pages", pageA, "data", "dependency.txt")
			if variant == "file" {
				name = filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
			}
			if e = os.WriteFile(name, []byte("external revision"), 0600); e != nil {
				t.Fatal(e)
			}
			before := digest(t, root)
			r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: "r04-conflict-" + variant, Changes: plan.Changes, ReadGuards: plan.ReadGuards})
			if !errors.Is(e, sourcetx.ErrConflict) || r.SourceCommitted || digest(t, root) != before {
				t.Fatal("dependency conflict published source", r, e)
			}
		})
	}
}

func TestR04EditInvalidAndCanceledRequestsHaveNoCandidates(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	before := digest(t, root)
	for _, ops := range [][]*editcore.Operation{nil, {nil}, {{Action: "update", ID: blockID, Data: map[string]string{"bad": "type"}}}, {{Action: "unknown"}}, {{Action: "create"}}} {
		out, e := l.PlanNativeEdits(ctx, ops)
		if e == nil || !reflect.DeepEqual(out, NativeEditPlan{}) {
			t.Fatal("invalid edit returned candidates", out, e)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	out, e := l.PlanNativeEdits(canceled, []*editcore.Operation{{Action: "delete", ID: blockID}})
	if !errors.Is(e, context.Canceled) || !reflect.DeepEqual(out, NativeEditPlan{}) || digest(t, root) != before {
		t.Fatal(out, e)
	}
}

func TestR04NativeEditCrashWorker(t *testing.T) {
	point := os.Getenv("WIKI_R04_CRASH_POINT")
	if point == "" {
		return
	}
	root := os.Getenv("WIKI_R04_CRASH_ROOT")
	raw, e := os.ReadFile(os.Getenv("WIKI_R04_CRASH_REQUEST"))
	if e != nil {
		t.Fatal(e)
	}
	var request sourcetx.Request
	if e = json.Unmarshal(raw, &request); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{ReadDefaults: true})
	e = m.mutate(context.Background(), storeID, false, func(w *sourcetx.Writer) error {
		_, e := w.Apply(context.Background(), request, func(at string) error {
			if at != point {
				return nil
			}
			_, _ = os.Stdout.WriteString("kill-point:" + at + "\n")
			proc, e := os.FindProcess(os.Getpid())
			if e != nil {
				return e
			}
			if e = proc.Kill(); e != nil {
				return e
			}
			select {}
		})
		return e
	})
	t.Fatalf("did not terminate at %s: %v", point, e)
}

func TestR04MultiSourceOriginalEditSurvivesHardKill(t *testing.T) {
	for _, point := range []string{"prepared", "published:0", "committed"} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			const id = "20261105121000-av00001"
			writeLayoutFixture(t, root, pageA, layoutFixture(id, pageA, av.CoverFromNone))
			m := open(t, root, Options{ReadDefaults: true})
			before := digest(t, root)
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			plan, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"hard kill"}`}, {Action: "setAttrViewName", ID: id, Data: "R04 recovered AV"}})
			l.Release()
			if e != nil || len(plan.Changes) != 2 {
				t.Fatal(plan, e)
			}
			request := sourcetx.Request{OperationID: "r04-hard-kill", Changes: plan.Changes, ReadGuards: plan.ReadGuards}
			raw, e := json.Marshal(request)
			if e != nil {
				t.Fatal(e)
			}
			input := filepath.Join(t.TempDir(), "request.json")
			if e = os.WriteFile(input, raw, 0600); e != nil {
				t.Fatal(e)
			}
			child := exec.Command(os.Args[0], "-test.run=^TestR04NativeEditCrashWorker$")
			child.Env = append(os.Environ(), "WIKI_R04_CRASH_POINT="+point, "WIKI_R04_CRASH_ROOT="+root, "WIKI_R04_CRASH_REQUEST="+input)
			output, e := child.CombinedOutput()
			if child.ProcessState == nil || child.ProcessState.ExitCode() != -1 || !strings.Contains(string(output), "kill-point:"+point) {
				t.Fatalf("worker did not reach kill point: %s %v", output, e)
			}
			if _, e = m.Tree(ctx, storeID); !errors.Is(e, sourcetx.ErrRecovery) {
				t.Fatal("interrupted view was admitted", e)
			}
			results, e := m.Recover(ctx, storeID)
			committed := point == "committed"
			if e != nil || len(results) != 1 || results[0].SourceCommitted != committed {
				t.Fatal(results, e)
			}
			if committed {
				for _, change := range plan.Changes {
					got, e := os.ReadFile(filepath.Join(root, change.Path))
					if e != nil || string(got) != string(change.Data) {
						t.Fatal(change.Path, e)
					}
				}
				r, e := m.Apply(ctx, storeID, request)
				if e != nil || r.RequestHash != results[0].RequestHash || !r.SourceCommitted {
					t.Fatal(r, e)
				}
			} else if digest(t, root) != before {
				t.Fatal("rollback retained source changes")
			}
			other, e := os.ReadFile(filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj"))
			if e != nil || !strings.Contains(string(other), "Workspace B knowledge") || strings.Contains(string(other), "hard kill") {
				t.Fatal("unrelated page was modified", e)
			}
		})
	}
}

func TestR04OriginalDocumentStructureCrossDocumentAndLargeInsert(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	dir := filepath.Join(root, "pages", pageA, "data", boxID)
	const target = "20261105141300-doc0002"
	const targetP = "20261105141300-par0002"
	const h1 = "20261105141301-hed0001"
	const h2 = "20261105141301-hed0002"
	const insert = "20261105141302-par0001"
	const prepend = "20261105141303-par0001"
	const appendID = "20261105141304-par0001"
	write := func(doc, raw string) {
		if e := os.WriteFile(filepath.Join(dir, doc+".docj"), []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(target, `{"ID":"`+target+`","Type":"NodeDocument","Properties":{"title":"Destination"},"Children":[{"ID":"`+targetP+`","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"target"}]}]}`)
	write(docID, `{"ID":"`+docID+`","Type":"NodeDocument","Properties":{"title":"Origin"},"Children":[{"ID":"`+h1+`","Type":"NodeHeading","HeadingLevel":1,"Children":[{"Type":"NodeText","Data":"first heading"}]},{"ID":"`+blockID+`","Type":"NodeParagraph","FutureBlock":{"large":900719925474099312345},"Children":[{"Type":"NodeText","Data":"cross document"}]},{"ID":"`+h2+`","Type":"NodeHeading","HeadingLevel":1,"Children":[{"Type":"NodeText","Data":"second heading"}]}]}`)
	m := open(t, root, Options{ReadDefaults: true})
	run := func(id string, ops []*editcore.Operation) NativeEditPlan {
		t.Helper()
		l, e := m.Acquire(ctx, storeID, pageA)
		if e != nil {
			t.Fatal(e)
		}
		p, e := l.PlanNativeEdits(ctx, ops)
		l.Release()
		if e != nil {
			t.Fatal(id, e)
		}
		r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: id, Changes: p.Changes, ReadGuards: p.ReadGuards})
		if e != nil || !r.SourceCommitted {
			t.Fatal(r, e)
		}
		return p
	}
	dom := func(id, html string) string {
		return `<div data-node-id="` + id + `" data-type="NodeParagraph" class="p"><div contenteditable="true">` + html + `</div></div>`
	}
	out := run("r04-doc-matrix", []*editcore.Operation{
		{Action: "foldHeading", ID: h1}, {Action: "unfoldHeading", ID: h1},
		{Action: "moveOutlineHeading", ID: h1, PreviousID: h2},
		{Action: "move", ID: blockID, ParentID: target},
		{Action: "insert", ID: insert, PreviousID: blockID, Data: dom(insert, "middle")},
		{Action: "prependInsert", ID: prepend, ParentID: target, Data: dom(prepend, "first")},
		{Action: "appendInsert", ID: appendID, ParentID: target, Data: dom(appendID, "last")},
		{Action: "append", ID: insert, ParentID: docID},
		{Action: "update", ID: blockID, Data: dom(blockID, "<strong>cross document updated</strong>")},
		{Action: "setAttrs", ID: blockID, Data: `{"custom-proof":"r04"}`},
		{Action: "doUpdateUpdated", ID: blockID, Data: "20261105142000"},
		{Action: "delete", ID: prepend},
	})
	if len(out.Changes) != 2 || out.Operations[0].RetData == nil || out.Operations[1].RetData == nil {
		t.Fatal("missing documents or heading results", len(out.Changes))
	}
	actual := func(doc string) string {
		b, e := os.ReadFile(filepath.Join(dir, doc+".docj"))
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	a, b := actual(docID), actual(target)
	if strings.Index(a, h2) > strings.Index(a, h1) || !strings.Contains(a, insert) || strings.Contains(a, blockID) ||
		!strings.Contains(b, "900719925474099312345") || !strings.Contains(b, "NodeTextMark") || !strings.Contains(b, "20261105142000") ||
		!strings.Contains(b, "custom-proof") || strings.Contains(b, prepend) || !strings.Contains(b, appendID) {
		t.Fatalf("wrong document candidates: %s %s", a, b)
	}
	previous := targetP
	large := []*editcore.Operation{}
	for i := 0; i < 130; i++ {
		id := fmt.Sprintf("20261105141400-p%06d", i)
		large = append(large, &editcore.Operation{Action: "insert", ID: id, PreviousID: previous, Data: dom(id, fmt.Sprintf("batch%03d", i))})
		previous = id
	}
	run("r04-doc-large-insert", large)
	a, b = actual(docID), actual(target)
	if strings.Contains(a, "batch000") || strings.Count(b, "batch") != 130 || strings.Index(b, "batch000") >= strings.Index(b, "batch129") {
		t.Fatal("large insert did not preserve exact destination and order")
	}
}

func TestR04FlashcardOriginalOperationsShareDocumentAndDeckCommit(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	conf := conf.NewFlashcard()
	for _, page := range []string{pageA, pageB} {
		data := flashMemory{}
		deck, err := riff.LoadDeck(data, flashcard.BuiltinDeckID, conf.RequestRetention, conf.MaximumInterval, conf.Weights, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		deck.Name = page
		deck.AddCard(flashCardID, blockID)
		if err = deck.Save(); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "riff")
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for file, raw := range data {
			if err = os.WriteFile(filepath.Join(dir, file), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	m := open(t, root, flashOptions(root))
	for _, tc := range []struct {
		action string
		want   int
	}{{"removeFlashcards", 0}, {"addFlashcards", 1}} {
		l, e := m.Acquire(ctx, storeID, pageA)
		if e != nil {
			t.Fatal(e)
		}
		p, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: tc.action, DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{blockID}}, {Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"cards-and-doc"}`}})
		l.Release()
		if e != nil || len(p.Changes) < 2 {
			t.Fatal(tc, p, e)
		}
		r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: "r04-" + tc.action, Changes: p.Changes, ReadGuards: p.ReadGuards})
		if e != nil || !r.SourceCommitted {
			t.Fatal(r, e)
		}
		for _, page := range []string{pageA, pageB} {
			l, e = m.Acquire(ctx, storeID, page)
			if e != nil {
				t.Fatal(e)
			}
			cards, e := l.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil)
			l.Release()
			want := tc.want
			if page == pageB {
				want = 1
			}
			if e != nil || len(cards) != want {
				t.Fatal(tc, page, cards, e)
			}
		}
	}
}

func TestR04OriginalAVTwoWayRelationAndSettingsCommitTogether(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const src = "20261105121000-av00001"
	const dst = "20261105121000-av00002"
	for _, page := range []string{pageA, pageB} {
		a, b := layoutFixture(src, page, av.CoverFromNone), layoutFixture(dst, page, av.CoverFromNone)
		a.ViewID = "table"
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "rel", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: dst, BackKeyID: "back", IsTwoWay: true}}})
		b.KeyValues = append(b.KeyValues, &av.KeyValues{Key: &av.Key{ID: "back", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: src, BackKeyID: "rel", IsTwoWay: true}}})
		writeLayoutFixture(t, root, page, a)
		writeLayoutFixture(t, root, page, b)
	}
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	p, e := l.PlanNativeEdits(ctx, []*editcore.Operation{
		{Action: "setAttrs", ID: blockID, Data: `{"custom-r04":"edit with AV"}`},
		{Action: "setAttrViewColWidth", AvID: src, ID: "number", Data: "201px"},
		{Action: "setAttrViewColWrap", AvID: src, ID: "number", Data: true},
		{Action: "updateAttrViewColOptions", AvID: src, KeyID: "select", Data: []*av.SelectOption{{Name: "X", Color: "1"}, {Name: "Y", Color: "4"}}},
		{Action: "updateAttrViewCell", AvID: src, KeyID: "rel", RowID: blockID, Data: map[string]any{"type": "relation", "relation": map[string]any{"blockIDs": []string{blockID, blockID}}}},
		{Action: "setAttrViewViewName", AvID: src, ID: "table", Data: "New table view"},
	})
	l.Release()
	if e != nil || len(p.Changes) != 3 || digest(t, root) != before {
		t.Fatal(len(p.Changes), e)
	}
	r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: "r04-relation-settings", Changes: p.Changes, ReadGuards: p.ReadGuards})
	if e != nil || !r.SourceCommitted {
		t.Fatal(r, e)
	}
	read := func(page, id string) *av.AttributeView {
		raw, e := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", id+".json"))
		if e != nil {
			t.Fatal(e)
		}
		var a av.AttributeView
		if e = json.Unmarshal(raw, &a); e != nil {
			t.Fatal(e)
		}
		return &a
	}
	a, b := read(pageA, src), read(pageA, dst)
	v := a.GetView("table")
	if v.Name != "New table view" || v.Table.Columns[1].Width != "201px" || !v.Table.Columns[1].Wrap {
		t.Fatal(v)
	}
	for _, item := range []*av.Value{a.GetValue("rel", blockID), b.GetValue("back", blockID)} {
		if item == nil || item.Relation == nil || !reflect.DeepEqual(item.Relation.BlockIDs, []string{blockID}) {
			t.Fatal(item)
		}
	}
	if read(pageB, src).GetValue("rel", blockID) != nil || read(pageB, dst).GetValue("back", blockID) != nil {
		t.Fatal("two-way relation escaped page")
	}
}

func TestR04NestedDocumentWithExternalReferenceKeepsLabel(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const child = "20261019090300-doc0003"
	const childBlock = "20261019090301-par0003"
	const external = "20261019090400-par0004"
	parent := filepath.Join(root, "pages", pageB, "data", boxID, docID)
	if e := os.MkdirAll(parent, 0700); e != nil {
		t.Fatal(e)
	}
	raw := []byte(`{"ID":"` + child + `","Type":"NodeDocument","Properties":{"title":"Child reference target"},"Children":[{"ID":"` + childBlock + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"Child reference destination "},{"Type":"NodeTextMark","TextMarkType":"block-ref","TextMarkBlockRefID":"` + external + `","TextMarkBlockRefSubtype":"d","TextMarkTextContent":"Open cross page"}]}]}`)
	file := filepath.Join(parent, child+".docj")
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageB)
	if e != nil {
		t.Fatal(e)
	}
	data := `<div data-node-id="` + childBlock + `" data-type="NodeParagraph" class="p"><div contenteditable="true">Child reference destination <span data-type="block-ref" data-subtype="d" data-id="` + external + `">Open cross page</span> child-owned-edit</div><div class="protyle-attr" contenteditable="false"></div></div>`
	plan, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: "update", ID: childBlock, Data: data}})
	l.Release()
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Changes) != 1 || !strings.Contains(string(plan.Changes[0].Data), "Open cross page") {
		t.Fatal(plan.Changes)
	}
	r, e := m.Apply(ctx, storeID, sourcetx.Request{OperationID: "r04-nested-cross-reference", Changes: plan.Changes, ReadGuards: plan.ReadGuards})
	if e != nil || !r.SourceCommitted {
		t.Fatal(r, e)
	}
	l, e = m.Acquire(ctx, storeID, pageB)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	if _, e = l.PrepareNativeReads(ctx, false); e != nil {
		t.Fatal(e)
	}
}

func TestR04UntouchedOpaqueTableKeepsOriginalSourceSpan(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	name := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	const rawTable = `{"ID":"20261019090503-tbl0002","Type":"NodeTable","TableAligns":[0],"OpaqueTable":{"owner":"table"},"Children":[{"Type":"NodeTableHead","Children":[{"Type":"NodeTableRow","OpaqueRow":{"owner":"anonymous-row"},"Children":[{"Type":"NodeTableCell","Children":[{"Type":"NodeText","Data":"Retain table"}]}]}]}]}`
	ast := []byte(`{"ID":"` + docID + `","Type":"NodeDocument","Properties":{"title":"Untouched table"},"Children":[{"ID":"` + blockID + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"format target"}]},` + rawTable + `]}`)
	if e := os.WriteFile(name, ast, 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	data := `<div data-node-id="` + blockID + `" data-type="NodeParagraph" class="p"><div contenteditable="true"><em>format target</em></div><div class="protyle-attr" contenteditable="false"></div></div>`
	plan, e := l.PlanNativeEdits(ctx, []*editcore.Operation{{Action: "update", ID: blockID, Data: data}})
	l.Release()
	if e != nil || len(plan.Changes) != 1 {
		t.Fatal(plan.Changes, e)
	}
	if raw := string(plan.Changes[0].Data); !strings.Contains(raw, rawTable) || !strings.Contains(raw, `"TextMarkType":"em"`) {
		t.Fatal(raw)
	}
}
