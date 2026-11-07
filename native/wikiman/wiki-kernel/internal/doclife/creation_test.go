package doclife

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"time"
)

type creationHarness struct {
	s                CreationServices
	trees            map[string]*parse.Tree
	order, events    []string
	locs             map[string]Location
	settings         DailySettings
	count, templates int
	parser           *lute.Lute
}

func newCreationHarness() *creationHarness {
	base, locs := lifecycleFixture()
	h := &creationHarness{trees: map[string]*parse.Tree{}, locs: locs}
	oldLookup, oldLoad := base.Lookup, base.Load
	base.Lookup = func(c context.Context, id string) (Location, bool, error) {
		if t := h.trees[id]; t != nil {
			return treeLocation(t), true, nil
		}
		return oldLookup(c, id)
	}
	base.Load = func(c context.Context, box, p string) (*parse.Tree, error) {
		for _, id := range h.order {
			t := h.trees[id]
			if t.Box == box && t.Path == p {
				return t, nil
			}
		}
		return oldLoad(c, box, p)
	}
	base.Exists = func(context.Context, string, string) (bool, error) { return false, nil }
	store := func(t *parse.Tree) {
		if h.trees[t.ID] == nil {
			h.order = append(h.order, t.ID)
		}
		h.trees[t.ID] = t
	}
	load := func(c context.Context, id string) (*parse.Tree, error) {
		if t := h.trees[id]; t != nil {
			return t, nil
		}
		loc, ok, e := oldLookup(c, id)
		if e != nil || !ok {
			return nil, ErrMissing
		}
		return oldLoad(c, loc.Box, loc.Path)
	}
	h.s = CreationServices{Box: base.Box, Language: base.Language, Untitled: func() string { return "Untitled" }, Now: func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.Local) }, NewID: func() string { h.count++; return fmt.Sprintf("20261027220000-c%06d", h.count) },
		Find: func(_ context.Context, mode, box, value, parent string) (Location, bool, error) {
			for _, id := range h.order {
				t := h.trees[id]
				if t.Box == box && (mode == "path" && t.Path == value || (mode == "hpath" || mode == "preferred" && t.ID == parent) && t.HPath == value) {
					return treeLocation(t), true, nil
				}
			}
			for _, l := range locs {
				if l.Box == box && (mode == "path" && l.Path == value || (mode == "hpath" || mode == "preferred" && l.ID == parent) && l.HPath == value) {
					return l, true, nil
				}
			}
			return Location{}, false, nil
		},
		Create: func(c context.Context, box, p, title, dom string) (*parse.Tree, error) {
			h.events = append(h.events, "create")
			plan, e := Prepare(c, Request{Action: "create-dom", Box: box, Path: p, Title: title, DOM: dom}, base)
			if e != nil {
				return nil, e
			}
			store(plan.Tree)
			return plan.Tree, nil
		}, Load: load,
		Parser: func(context.Context) (*lute.Lute, error) { h.parser = lute.New(); return h.parser, nil }, Flush: func(context.Context) error { h.events = append(h.events, "flush"); return nil },
		Attrs: func(c context.Context, id string, attrs map[string]string) error {
			h.events = append(h.events, "attrs")
			t, e := load(c, id)
			if e != nil {
				return e
			}
			for k, v := range attrs {
				t.Root.SetIALAttr(k, v)
			}
			store(t)
			return nil
		},
		Stage: func(_ context.Context, t *parse.Tree, mode string) error {
			h.events = append(h.events, "stage-"+mode)
			store(t)
			return nil
		}, Sort: func(_ context.Context, box, mode, p, id string) error {
			h.events = append(h.events, "sort-"+mode)
			return nil
		}, Sync: func(context.Context) error { h.events = append(h.events, "sync"); return nil },
		Daily: func(context.Context, string) (DailySettings, error) { return h.settings, nil }, RenderPath: func(_ context.Context, s string) (string, error) { return s, nil }, Template: func(context.Context, string, string) (*parse.Tree, string, error) {
			h.templates++
			tpl := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}}
			for k, v := range map[string]string{"name": "Template Name", "custom-key": "keep", "title": "Do not replace title", "tags": "not inherited", "icon": "book"} {
				tpl.Root.SetIALAttr(k, v)
			}
			return tpl, `<div data-type="NodeParagraph" data-node-id="20261027221000-templ01"><div contenteditable="true">Template body</div></div>`, nil
		}, Mirror: func(context.Context, string, string) error { h.events = append(h.events, "mirror"); return nil }, Progress: func(_ context.Context, start bool) error {
			h.events = append(h.events, fmt.Sprint("progress-", start))
			return nil
		}, Strict: true}
	return h
}

func TestPeripheralMarkdownHPathTagsAndClippingOptions(t *testing.T) {
	h := newCreationHarness()
	p, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "markdown", Box: "box", HPath: " /New / Child /Leaf ", ID: newID, Markdown: "Hello $x$ **bold**", Tags: " one， two ,,three ", ClippingHref: "https://ld246.com/article/example"}, h.s)
	if e != nil || len(h.trees) != 3 || p.ID != newID || p.Tree.HPath != "/New/Child/Leaf" || p.Tree.Root.IALAttr("tags") != "one,two,,three" || !h.parser.ParseOptions.InlineMath || !h.parser.ParseOptions.InlineAsterisk {
		t.Fatal(p, e, h.events)
	}
	if h.events[len(h.events)-1] != "sort-configured" {
		t.Fatal(h.events)
	}
	h = newCreationHarness()
	p, e = CreatePeripheral(context.Background(), CreationRequest{Mode: "hpath", Box: "box", HPath: "/Original/Preferred", ParentID: rootID, ID: newID}, h.s)
	if e != nil || p.Path != "/"+rootID+"/"+newID+".docj" || len(h.trees) != 1 {
		t.Fatal(p, e)
	}
	// A reused parent can have a different disk ID from its display path.
	h = newCreationHarness()
	p, e = CreatePeripheral(context.Background(), CreationRequest{Mode: "hpath", Box: "box", HPath: "/Original/Child/Leaf", ID: newID}, h.s)
	if e != nil || p.Path != "/"+rootID+"/"+childID+"/"+newID+".docj" || len(h.trees) != 1 {
		t.Fatal(p, e)
	}
}

func TestPeripheralDailyNewTemplateWhitelistAndExistingNoRewrite(t *testing.T) {
	h := newCreationHarness()
	h.settings = DailySettings{SavePath: "/Daily/Today", TemplatePath: "daily.md"}
	p, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "daily", Box: "box"}, h.s)
	if e != nil || p.Existed || p.Path == "" || p.Tree.Root.IALAttr("name") != "Template Name" || p.Tree.Root.IALAttr("custom-key") != "keep" || p.Tree.Root.IALAttr("title") != "Today" || p.Tree.Root.IALAttr("tags") != "" || p.Tree.Root.IALAttr(DailyNoteAttrPrefix+"20261027") != "20261027" || !strings.Contains(p.Tree.Root.Text(), "Template body") {
		t.Fatal(p, e)
	}
	n := len(h.events)
	again, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "daily", Box: "box"}, h.s)
	if e != nil || !again.Existed || again.ID != p.ID || h.templates != 1 || len(h.events) != n+1 || h.events[n] != "flush" {
		t.Fatal(again, e, h.events[n:])
	}
	h = newCreationHarness()
	h.settings = DailySettings{SavePath: "/Original", TemplatePath: "must-not-render.md"}
	p, e = CreatePeripheral(context.Background(), CreationRequest{Mode: "daily", Box: "box"}, h.s)
	if e != nil || !p.Existed || p.Path != "/"+rootID+".docj" || h.templates != 0 || p.Tree.Root.IALAttr(DailyNoteAttrPrefix+"20261027") == "" {
		t.Fatal(p, e)
	}
}

func TestPeripheralDuplicateInternalRefsAndBindingsBeforeStage(t *testing.T) {
	h := newCreationHarness()
	source, e := h.s.Load(context.Background(), rootID)
	if e != nil {
		t.Fatal(e)
	}
	source.Root.SetIALAttr("scroll", "old")
	source.Root.SetIALAttr("custom-avs", "old-av")
	p := source.Root.FirstChild
	p.SetIALAttr("av-names", "old")
	p.SetIALAttr("custom-sy-av-s-text-1", "old")
	p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: p.ID, TextMarkTextContent: "internal"})
	p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: targetID, TextMarkTextContent: "external"})
	source.Root.AppendChild(&ast.Node{ID: childID, Type: ast.NodeAttributeView, AttributeViewID: "av"})
	stage := h.s.Stage
	h.s.Stage = func(c context.Context, candidate *parse.Tree, mode string) error {
		if candidate.Root.IALAttr("custom-avs") != "" || candidate.Root.FirstChild.IALAttr("av-names") != "" || candidate.Root.FirstChild.IALAttr("custom-sy-av-s-text-1") != "" {
			t.Fatal("bound copy reached persistence")
		}
		return stage(c, candidate, mode)
	}
	r, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "duplicate", Tree: source}, h.s)
	if e != nil || r.ID == rootID || r.Tree.Root.IALAttr("scroll") != "" || !strings.Contains(r.Tree.Root.IALAttr("title"), "(Duplicated 2026-10-27 22:00:00)") || r.Tree.Root.FirstChild.Next.ID == childID {
		t.Fatal(r, e)
	}
	p = r.Tree.Root.FirstChild
	if p.FirstChild.Next.TextMarkBlockRefID != p.ID || p.LastChild.TextMarkBlockRefID != targetID {
		t.Fatal("reference remap", p)
	}
	if strings.Join(h.events, ",") != "progress-true,mirror,stage-create,sort-after,flush,progress-false" {
		t.Fatal(h.events)
	}
}

func TestPeripheralFailureCancelNoFallbackAndProgressCleanup(t *testing.T) {
	h := newCreationHarness()
	sentinel := errors.New("failed")
	h.s.Create = func(context.Context, string, string, string, string) (*parse.Tree, error) { return nil, sentinel }
	if p, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "markdown", Box: "box", HPath: "/Fail", ID: newID}, h.s); !errors.Is(e, sentinel) || p.Tree != nil || strings.Contains(strings.Join(h.events, ","), "attrs") {
		t.Fatal(p, e, h.events)
	}
	h = newCreationHarness()
	h.settings = DailySettings{SavePath: "/New", TemplatePath: "bad"}
	h.s.Template = func(context.Context, string, string) (*parse.Tree, string, error) { return nil, "", sentinel }
	if p, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "daily", Box: "box"}, h.s); !errors.Is(e, sentinel) || p.Tree != nil || strings.Contains(strings.Join(h.events, ","), "sync") {
		t.Fatal(p, e)
	}
	h = newCreationHarness()
	h.s.Stage = func(context.Context, *parse.Tree, string) error { return sentinel }
	if p, e := CreatePeripheral(context.Background(), CreationRequest{Mode: "duplicate", ID: rootID}, h.s); !errors.Is(e, sentinel) || p.Tree != nil || h.events[len(h.events)-1] != "progress-false" || strings.Contains(strings.Join(h.events, ","), "sort-after") {
		t.Fatal(p, e, h.events)
	}
	h = newCreationHarness()
	c, stop := context.WithCancel(context.Background())
	h.s.Flush = func(actual context.Context) error {
		if actual != c {
			t.Fatal("wrong context")
		}
		stop()
		return nil
	}
	h.s.Parser = func(context.Context) (*lute.Lute, error) { t.Fatal("late cancel reached parser"); return nil, nil }
	if p, e := CreatePeripheral(c, CreationRequest{Mode: "markdown", Box: "box"}, h.s); !errors.Is(e, context.Canceled) || p.Tree != nil {
		t.Fatal(p, e)
	}
	if _, e := CreatePeripheral(nil, CreationRequest{}, h.s); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}

func TestCreationTemplateBodyNormalizationAndCancellation(t *testing.T) {
	h := newCreationHarness()
	p := lute.New()
	p.SetKramdownIAL(true)
	p.SetYamlFrontMatter(true)
	body, dom, e := RenderCreationBody(context.Background(), "---\nname: Front Matter\ncustom: keep\n---\n\nTemplate body\n\n{: id=\"20261027220000-root999\" type=\"doc\" icon=\"&amp;\" bookmark=\"Saved\"}\n", p, TemplateBodyServices{NewID: h.s.NewID, Now: h.s.Now})
	if e != nil || body == nil || !strings.Contains(dom, "Template body") || body.Root.IALAttr("icon") != "&" || body.Root.IALAttr("bookmark") != "Saved" || body.Root.IALAttr("custom-custom") != "keep" {
		t.Fatal(body, dom, e)
	}
	text, e := RenderCreationText(context.Background(), "/{{now | date \"2006-01-02\"}}", false, map[string]interface{}{"now": h.s.Now, "date": func(format string, now time.Time) string { return now.Format(format) }}, nil)
	if e != nil || text != "/2026-10-27" {
		t.Fatal(text, e)
	}
	c, stop := context.WithCancel(context.Background())
	stop()
	if _, _, e = RenderCreationBody(c, "ignored", p, TemplateBodyServices{NewID: h.s.NewID, Now: h.s.Now}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
