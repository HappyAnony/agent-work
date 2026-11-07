// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/file.go and history.go document lifecycle preparation rules.
package doclife

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrDependency = errors.New("document_lifecycle_dependencies_unbound")
var ErrMissing = errors.New("document_lifecycle_source_missing")

type Location struct {
	Box, Path, HPath, ID string
	Document             bool
}
type Request struct {
	Action, Box, Path, Title, Markdown, DOM, ToBox, ToPath, HistoryPath string
	Paths, Sorts                                                        []string
	Callback                                                            interface{}
}
type Change struct {
	From Location
	Tree *parse.Tree
}
type Plan struct {
	Request             Request
	Tree                *parse.Tree
	Changes             []Change
	IDs, AVIDs, Effects []string
	Working             *Location
	SourceHashes        map[string][32]byte
	Noop                bool
}

// Load/History transfer detached original-format trees to this request. No
// callback may borrow another Context or repair missing source on a read.
type Services struct {
	AllowDeeper   bool
	Box           func(context.Context, string) (bool, error)
	Load          func(context.Context, string, string) (*parse.Tree, error)
	Lookup        func(context.Context, string) (Location, bool, error)
	Children      func(context.Context, string, string) ([]Location, error)
	Exists        func(context.Context, string, string) (bool, error)
	History       func(context.Context, string) (*parse.Tree, error)
	ExistingIDs   func(context.Context, []string, string) (map[string]bool, error)
	Parser        func(context.Context) (*lute.Lute, error)
	Language      func(int) string
	PlainLanguage func(int) string
	Now, NewID    func() string
	Apply         func(context.Context, *Plan) error
	Before        func(context.Context, Request) error
}

func CleanTitle(title string) string {
	// Keep the original placeholder rule, including ZWJ emoji preservation.
	title = strings.ReplaceAll(title, string(gulu.ZWJ), "__@ZWJ@__")
	title = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(title))
	title = strings.ReplaceAll(title, "__@ZWJ@__", string(gulu.ZWJ))
	return strings.TrimSpace(title)
}
func FilterPaths(paths []string, target string, moving bool) []string {
	paths = slices.Clone(paths)
	sort.Slice(paths, func(i, j int) bool { return strings.Count(paths[i], "/") < strings.Count(paths[j], "/") })
	var ret, dirs []string
	for _, p := range paths {
		child := false
		for _, d := range dirs {
			if strings.HasPrefix(p, d) {
				child = true
				break
			}
		}
		if child {
			continue
		}
		d := strings.TrimSuffix(p, ".docj")
		dirs = append(dirs, d)
		if moving && strings.HasPrefix(target, d) {
			continue
		}
		ret = append(ret, p)
	}
	return ret
}
func treeLocation(t *parse.Tree) Location {
	return Location{Box: t.Box, Path: t.Path, HPath: t.HPath, ID: t.Root.ID, Document: true}
}
func language(s Services, code int) error { return errors.New(s.Language(code)) }
func load(ctx context.Context, s Services, box, p string) (*parse.Tree, error) {
	t, e := s.Load(ctx, box, p)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if t == nil || t.Root == nil {
		return nil, ErrMissing
	}
	return t, nil
}
func checkBox(ctx context.Context, s Services, box string) error {
	ok, e := s.Box(ctx, box)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if !ok {
		return language(s, 0)
	}
	return nil
}

func Prepare(ctx context.Context, q Request, s Services) (ret Plan, err error) {
	if ctx == nil || s.Box == nil || s.Load == nil || s.Lookup == nil || s.Language == nil {
		return ret, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	q.Paths = slices.Clone(q.Paths)
	q.Sorts = slices.Clone(q.Sorts)
	ret.Request = q
	switch q.Action {
	case "create", "create-dom":
		if s.Parser == nil || s.Exists == nil || s.NewID == nil || s.Now == nil {
			return Plan{}, ErrDependency
		}
		if err = checkBox(ctx, s, q.Box); err != nil {
			return Plan{}, err
		}
		title := CleanTitle(q.Title)
		if utf8.RuneCountInString(title) > 512 {
			return Plan{}, language(s, 106)
		}
		title = strings.TrimSpace(strings.ReplaceAll(title, "/", ""))
		if title == "" {
			title = s.Language(16)
		}
		base := strings.TrimSpace(path.Base(q.Path))
		id := strings.TrimSuffix(base, ".docj")
		if strings.HasPrefix(base, ".") {
			return Plan{}, language(s, 13)
		}
		if !ast.IsNodeIDPattern(id) || !strings.HasSuffix(base, ".docj") {
			return Plan{}, language(s, 16)
		}
		if strings.Count(q.Path, "/") > 7 && !s.AllowDeeper {
			return Plan{}, language(s, 118)
		}
		hpath := "/" + title
		if folder := path.Dir(q.Path); folder != "/" {
			loc, ok, e := s.Lookup(ctx, path.Base(folder))
			if e != nil {
				return Plan{}, e
			}
			if !ok || loc.Box != q.Box {
				return Plan{}, ErrMissing
			}
			parent, e := load(ctx, s, loc.Box, loc.Path)
			if e != nil {
				return Plan{}, e
			}
			hpath = path.Join(parent.HPath, title)
		}
		exists, e := s.Exists(ctx, q.Box, q.Path)
		if e != nil {
			return Plan{}, e
		}
		if exists {
			return Plan{}, language(s, 1)
		}
		parser, e := s.Parser(ctx)
		if e != nil {
			return Plan{}, e
		}
		if parser == nil {
			return Plan{}, ErrDependency
		}
		dom := q.DOM
		if q.Action == "create" {
			dom = parser.Md2BlockDOM(q.Markdown, false)
		}
		t := parser.BlockDOM2Tree(dom)
		if t == nil || t.Root == nil {
			return Plan{}, ErrMissing
		}
		t.Box, t.Path, t.HPath, t.ID = q.Box, q.Path, hpath, id
		t.Root.ID = id
		t.Root.Spec = "1"
		t.Root.KramdownIAL = [][]string{{"id", id}, {"title", html.EscapeAttrVal(title)}, {"updated", id[:14]}}
		if t.Root.FirstChild == nil {
			p := &ast.Node{Type: ast.NodeParagraph, ID: s.NewID()}
			if !ast.IsNodeIDPattern(p.ID) {
				return Plan{}, errors.New("document_lifecycle_invalid_generated_id")
			}
			p.SetIALAttr("id", p.ID)
			p.SetIALAttr("updated", p.ID[:14])
			t.Root.AppendChild(p)
		}
		var unlinks []*ast.Node
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if !enter || n.Type != ast.NodeParagraph {
				return ast.WalkContinue
			}
			link := n.FirstChild
			if link == nil || !link.IsTextMarkType("a") {
				return ast.WalkContinue
			}
			kind := ""
			typ := ast.NodeAudio
			if strings.HasSuffix(link.TextMarkAHref, ".mp3") {
				kind = "audio"
			} else if strings.HasSuffix(link.TextMarkAHref, ".mp4") {
				kind = "video"
				typ = ast.NodeVideo
			}
			if kind != "" {
				media := &ast.Node{ID: n.ID, Type: typ, Tokens: []byte("<" + kind + " controls=\"controls\" src=\"" + link.TextMarkAHref + "\" data-src=\"" + link.TextMarkAHref + "\"></" + kind + ">")}
				media.SetIALAttr("id", n.ID)
				updated := s.Now()
				if len(n.ID) >= 14 {
					updated = n.ID[:14]
				}
				media.SetIALAttr("updated", updated)
				n.InsertBefore(media)
				unlinks = append(unlinks, n)
			}
			return ast.WalkContinue
		})
		for _, n := range unlinks {
			n.Unlink()
		}
		ret.Tree = t
		ret.Changes = []Change{{Tree: t}}
		ret.Effects = []string{"mkdir", "create-transaction", "flush"}
		if q.Action == "create" {
			ret.Effects = append(ret.Effects, "sort")
		}
	case "rename":
		if s.Now == nil || s.Children == nil {
			return Plan{}, ErrDependency
		}
		if err = checkBox(ctx, s, q.Box); err != nil {
			return Plan{}, err
		}
		t, e := load(ctx, s, q.Box, q.Path)
		if e != nil {
			return Plan{}, e
		}
		title := CleanTitle(q.Title)
		if utf8.RuneCountInString(title) > 512 {
			return Plan{}, language(s, 106)
		}
		if t.Root.IALAttr("title") == title {
			ret.Tree = t
			ret.Noop = true
			break
		}
		if title == "" {
			if s.PlainLanguage == nil {
				return Plan{}, ErrDependency
			}
			title = s.PlainLanguage(16)
		}
		title = strings.ReplaceAll(title, "/", "")
		from := treeLocation(t)
		t.HPath = path.Join(path.Dir(t.HPath), title)
		t.Root.SetIALAttr("title", title)
		t.Root.SetIALAttr("updated", s.Now())
		ret.Tree = t
		ret.Changes = []Change{{From: from, Tree: t}}
		children, e := s.Children(ctx, q.Box, q.Path)
		if e != nil {
			return Plan{}, e
		}
		for _, loc := range children {
			child, e := load(ctx, s, loc.Box, loc.Path)
			if e != nil {
				return Plan{}, e
			}
			before := treeLocation(child)
			child.HPath = t.HPath + strings.TrimPrefix(child.HPath, from.HPath)
			ret.Changes = append(ret.Changes, Change{From: before, Tree: child})
		}
		ret.Effects = []string{"rename-write", "rename-event", "rename-descendants", "rename-references", "sync"}
	case "move", "delete":
		if s.Children == nil {
			return Plan{}, ErrDependency
		}
		if q.Action == "move" {
			if err = checkBox(ctx, s, q.ToBox); err != nil {
				return Plan{}, err
			}
		}
		paths := q.Paths
		if q.Action == "delete" && q.Path != "" {
			paths = []string{q.Path}
		}
		paths = FilterPaths(paths, q.ToPath, q.Action == "move")
		ret.Request.Paths = paths
		for _, p := range paths {
			loc, ok, e := s.Lookup(ctx, strings.TrimSuffix(path.Base(p), ".docj"))
			if e != nil {
				return Plan{}, e
			}
			if q.Action == "delete" && q.Box != "" && q.Path != "" {
				loc = Location{Box: q.Box, Path: q.Path}
				ok = true
			}
			if !ok {
				continue
			}
			// A supplied notebook is a scope assertion, never permission to
			// resolve the same ID from another notebook/workspace.
			if q.Box != "" && loc.Box != q.Box {
				return Plan{}, ErrMissing
			}
			if q.Action == "move" && len(paths) == 1 && loc.Box == q.ToBox {
				dir := path.Dir(loc.Path)
				if q.ToPath == "/" && dir == "/" || dir+".docj" == q.ToPath {
					ret.Noop = true
					continue
				}
			}
			t, e := load(ctx, s, loc.Box, loc.Path)
			if e != nil {
				return Plan{}, e
			}
			loc = treeLocation(t)
			children, e := s.Children(ctx, loc.Box, loc.Path)
			if e != nil {
				return Plan{}, e
			}
			docs := append([]Location{loc}, children...)
			var target *parse.Tree
			if q.Action == "move" && q.ToPath != "/" {
				target, e = load(ctx, s, q.ToBox, q.ToPath)
				if e != nil {
					return Plan{}, e
				}
			}
			for _, d := range docs {
				current := t
				if d.Path != loc.Path {
					current, e = load(ctx, s, d.Box, d.Path)
					if e != nil {
						return Plan{}, e
					}
				}
				before := treeLocation(current)
				if q.Action == "move" {
					folder := "/"
					hp := ""
					if target != nil {
						folder = strings.TrimSuffix(target.Path, ".docj")
						hp = target.HPath
					}
					newRoot := path.Join(folder, t.ID+".docj")
					newPath := newRoot
					if d.Path != loc.Path {
						newPath = strings.TrimSuffix(newRoot, ".docj") + strings.TrimPrefix(d.Path, strings.TrimSuffix(loc.Path, ".docj"))
					}
					if strings.Count(newPath, "/") > 7 && !s.AllowDeeper {
						return Plan{}, language(s, 118)
					}
					current.Box = q.ToBox
					current.Path = newPath
					// Root path titles are independent of descendant disk IDs.
					if d.Path == loc.Path {
						current.HPath = path.Join("/", hp, path.Base(loc.HPath))
					} else {
						current.HPath = path.Join("/", hp, path.Base(loc.HPath)) + strings.TrimPrefix(before.HPath, loc.HPath)
					}
					if d.Path == loc.Path {
						current.Root.RemoveIALAttr("custom-hidden")
					}
				}
				ret.Changes = append(ret.Changes, Change{From: before, Tree: current})
				ret.IDs = append(ret.IDs, current.ID)
			}
		}
		ret.Noop = len(ret.Changes) == 0
		if q.Action == "move" {
			ret.Effects = []string{"move-directory", "move-document", "move-index", "move-sorts", "move-events", "refresh-parent", "clear-doc-cache", "sync"}
		} else {
			ret.Effects = []string{"history-backup", "history-av", "copy-assets", "history-index", "av-unbind", "remove-files", "remove-sorts", "remove-recent", "remove-event", "refresh-parent", "delete-index", "refresh-references"}
		}
	case "restore":
		if s.History == nil || s.ExistingIDs == nil || s.NewID == nil {
			return Plan{}, ErrDependency
		}
		if err = checkBox(ctx, s, q.Box); err != nil {
			return Plan{}, err
		}
		t, e := s.History(ctx, q.HistoryPath)
		if e != nil {
			return Plan{}, e
		}
		if t == nil {
			ret.Noop = true
			break
		}
		if t.Root == nil {
			return Plan{}, ErrMissing
		}
		root := t.Root.ID
		working, ok, e := s.Lookup(ctx, root)
		if e != nil {
			return Plan{}, e
		}
		if ok {
			ret.Working = &working
		}
		parent := path.Base(path.Dir(q.HistoryPath))
		if ok {
			parent = path.Base(path.Dir(working.Path))
		}
		parentLoc, found, e := s.Lookup(ctx, parent)
		if e != nil {
			return Plan{}, e
		}
		folder, hp := "/", ""
		if found && parentLoc.Box == q.Box {
			folder = strings.TrimSuffix(parentLoc.Path, ".docj")
			hp = parentLoc.HPath
		}
		t.Box = q.Box
		t.Path = path.Join(folder, path.Base(q.HistoryPath))
		t.HPath = hp + "/" + t.Root.IALAttr("title")
		nodes := map[string]*ast.Node{}
		var ids []string
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				nodes[n.ID] = n
				ids = append(ids, n.ID)
			}
			if enter && n.Type == ast.NodeAttributeView {
				if !slices.Contains(ret.AVIDs, n.AttributeViewID) {
					ret.AVIDs = append(ret.AVIDs, n.AttributeViewID)
				}
			}
			return ast.WalkContinue
		})
		exclude := ""
		if ok {
			exclude = root
		}
		existing, e := s.ExistingIDs(ctx, ids, exclude)
		if e != nil {
			return Plan{}, e
		}
		used := map[string]bool{}
		for _, id := range ids {
			used[id] = true
		}
		for _, id := range ids {
			if existing[id] {
				n := nodes[id]
				next := s.NewID()
				if !ast.IsNodeIDPattern(next) || used[next] {
					return Plan{}, errors.New("document_lifecycle_id_collision")
				}
				used[next] = true
				n.ID = next
				n.SetIALAttr("id", next)
				if n.IALAttr("updated") < next[:14] {
					n.SetIALAttr("updated", next[:14])
				}
				if n.Type == ast.NodeDocument {
					t.ID = next
					t.Path = path.Join(path.Dir(t.Path), next+".docj")
				}
			}
		}
		ret.Tree = t
		ret.IDs = []string{root}
		ret.Changes = []Change{{Tree: t}}
		ret.Effects = []string{"restore-replace", "restore-av", "restore-registry", "restore-index", "reload-filetree", "reload-protyle", "message", "sync", "reload-av", "restore-refresh"}
	default:
		return Plan{}, errors.New("document_lifecycle_action")
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	return ret, nil
}

func Execute(ctx context.Context, q Request, s Services) (Plan, error) {
	if ctx == nil || s.Apply == nil {
		return Plan{}, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return Plan{}, e
	}
	if s.Before != nil {
		if e := s.Before(ctx, q); e != nil {
			return Plan{}, e
		}
	}
	p, e := Prepare(ctx, q, s)
	if e != nil {
		return Plan{}, e
	}
	if !p.Noop {
		if e = s.Apply(ctx, &p); e != nil {
			return Plan{}, e
		}
	}
	if e = ctx.Err(); e != nil {
		return Plan{}, e
	}
	return p, nil
}
