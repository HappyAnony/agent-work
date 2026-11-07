// SPDX-License-Identifier: AGPL-3.0-or-later
// Original peripheral creation controllers from model/file.go and path.go.
package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"time"
)

const DailyNoteAttrPrefix = "custom-dailynote-"

type CreationRequest struct {
	Mode, Box, HPath, Markdown, DOM, Tags, ParentID, ID, ClippingHref string
	WithMath                                                          bool
	// A detached original-format tree, consumed only by duplicate. Native
	// Workspace entrypoints load their own tree and never accept this provider.
	Tree *parse.Tree
}
type CreationResult struct {
	ID, Path string
	Existed  bool
	Tree     *parse.Tree
}
type DailySettings struct{ SavePath, TemplatePath string }
type CreationServices struct {
	Box        func(context.Context, string) (bool, error)
	Find       func(context.Context, string, string, string, string) (Location, bool, error)
	Create     func(context.Context, string, string, string, string) (*parse.Tree, error)
	Load       func(context.Context, string) (*parse.Tree, error)
	Parser     func(context.Context) (*lute.Lute, error)
	Flush      func(context.Context) error
	Attrs      func(context.Context, string, map[string]string) error
	Stage      func(context.Context, *parse.Tree, string) error
	Sort       func(context.Context, string, string, string, string) error
	Sync       func(context.Context) error
	Daily      func(context.Context, string) (DailySettings, error)
	RenderPath func(context.Context, string) (string, error)
	Template   func(context.Context, string, string) (*parse.Tree, string, error)
	Mirror     func(context.Context, string, string) error
	Progress   func(context.Context, bool) error
	Now        func() time.Time
	NewID      func() string
	Language   func(int) string
	Untitled   func() string
	BoxMissing error
	Warning    func(error)
	// Compatibility may retain original missing-template warning policy;
	// owned services reject dependency/render failures rather than fake content.
	Strict bool
}

func trimCreationPath(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "/")
}
func creationErr(ctx context.Context, e error) error {
	if e != nil {
		return e
	}
	return ctx.Err()
}
func (s CreationServices) warn(e error) {
	if s.Warning != nil {
		s.Warning(e)
	}
}

func CreatePeripheral(ctx context.Context, q CreationRequest, s CreationServices) (ret CreationResult, err error) {
	if ctx == nil || s.Box == nil || s.Language == nil {
		return ret, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return CreationResult{}, err
	}
	if q.Mode != "markdown" && q.Mode != "hpath" && q.Mode != "daily" && q.Mode != "duplicate" {
		return ret, errors.New("document_creation_mode")
	}
	if q.Mode == "duplicate" {
		ret, err = duplicateCreation(ctx, q, s)
	} else {
		ok, e := s.Box(ctx, q.Box)
		if e = creationErr(ctx, e); e != nil {
			return CreationResult{}, e
		}
		if !ok {
			if q.Mode == "daily" && s.BoxMissing != nil {
				return ret, s.BoxMissing
			}
			return ret, errors.New(s.Language(0))
		}
		switch q.Mode {
		case "hpath":
			ret, err = createHPath(ctx, q, s)
		case "markdown":
			if s.Parser == nil || s.Flush == nil || s.Attrs == nil || s.Load == nil || s.Sort == nil {
				return ret, ErrDependency
			}
			if e = creationErr(ctx, s.Flush(ctx)); e != nil {
				return CreationResult{}, e
			}
			parser, e := s.Parser(ctx)
			if e = creationErr(ctx, e); e != nil {
				return CreationResult{}, e
			}
			if parser == nil {
				return ret, ErrDependency
			}
			if q.WithMath {
				parser.SetInlineMath(true)
			}
			parser.SetHTMLTag2TextMark(true)
			if strings.HasPrefix(q.ClippingHref, "https://ld246.com/article/") || strings.HasPrefix(q.ClippingHref, "https://liuyun.io/article/") {
				parser.SetInlineAsterisk(true)
				parser.SetInlineUnderscore(true)
				parser.SetSup(true)
				parser.SetSub(true)
				parser.SetTag(true)
				parser.SetInlineMath(true)
				parser.SetGFMStrikethrough(true)
			}
			q.DOM = parser.Md2BlockDOM(q.Markdown, false)
			ret, err = createHPath(ctx, q, s)
			if err != nil {
				return CreationResult{}, err
			}
			tags := strings.Split(strings.ReplaceAll(strings.TrimSpace(q.Tags), "，", ","), ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
			if err = creationErr(ctx, s.Attrs(ctx, ret.ID, map[string]string{"tags": strings.Join(tags, ",")})); err != nil {
				return CreationResult{}, err
			}
			if err = creationErr(ctx, s.Flush(ctx)); err != nil {
				return CreationResult{}, err
			}
			ret.Tree, err = s.Load(ctx, ret.ID)
			err = creationErr(ctx, err)
			if err != nil {
				return CreationResult{}, err
			}
			if ret.Tree == nil {
				return CreationResult{}, ErrMissing
			}
			ret.Path = ret.Tree.Path
			err = s.Sort(ctx, q.Box, "configured", path.Dir(ret.Path), ret.ID)
		case "daily":
			ret, err = dailyCreation(ctx, q, s)
		}
	}
	if err = creationErr(ctx, err); err != nil {
		return CreationResult{}, err
	}
	return ret, nil
}

func createHPath(ctx context.Context, q CreationRequest, s CreationServices) (ret CreationResult, err error) {
	if s.NewID == nil || s.Find == nil || s.Create == nil {
		return ret, ErrDependency
	}
	ret.ID = q.ID
	if ret.ID == "" {
		ret.ID = s.NewID()
	}
	if !ast.IsNodeIDPattern(ret.ID) {
		return CreationResult{}, errors.New("document_creation_invalid_id")
	}
	hp := trimCreationPath(strings.TrimSuffix(q.HPath, ".docj"))
	if !strings.HasPrefix(hp, "/") || hp == "/" || hp == "" {
		return CreationResult{}, ErrMissing
	}
	if q.ParentID != "" {
		parentHP, name := path.Split(hp)
		parentHP = strings.TrimSuffix(parentHP, "/")
		loc, ok, e := s.Find(ctx, "preferred", q.Box, parentHP, q.ParentID)
		if e = creationErr(ctx, e); e != nil {
			return CreationResult{}, e
		}
		if ok && loc.ID == q.ParentID {
			ret.Tree, err = s.Create(ctx, q.Box, strings.TrimSuffix(loc.Path, ".docj")+"/"+ret.ID+".docj", name, q.DOM)
			if err == nil && ret.Tree != nil {
				ret.Path = ret.Tree.Path
			}
			return
		}
	}
	loc, ok, e := s.Find(ctx, "path", q.Box, hp, "")
	if e = creationErr(ctx, e); e != nil {
		return CreationResult{}, e
	}
	if ok {
		ret.ID, ret.Path, ret.Existed = loc.ID, loc.Path, true
		return ret, nil
	}
	parts := strings.Split(hp, "/")[1:]
	known := map[string]Location{}
	prefix := ""
	for i, part := range parts {
		if i == len(parts)-1 {
			break
		}
		prefix += "/" + part
		loc, ok, e := s.Find(ctx, "hpath", q.Box, prefix, "")
		if e = creationErr(ctx, e); e != nil {
			return CreationResult{}, e
		}
		if !ok {
			break
		}
		known[prefix] = loc
	}
	disk := "/"
	prefix = ""
	for i, part := range parts {
		if err = ctx.Err(); err != nil {
			return CreationResult{}, err
		}
		prefix += "/" + part
		last := i == len(parts)-1
		if loc, ok := known[prefix]; ok {
			disk += loc.ID
			if last {
				disk += ".docj"
			}
		} else {
			id := ret.ID
			if !last {
				id = s.NewID()
			}
			if !ast.IsNodeIDPattern(id) {
				return CreationResult{}, errors.New("document_creation_invalid_id")
			}
			disk += id
			dom := ""
			if last {
				dom = q.DOM
			}
			t, e := s.Create(ctx, q.Box, disk+".docj", part, dom)
			if e = creationErr(ctx, e); e != nil {
				return CreationResult{}, e
			}
			if t == nil {
				return CreationResult{}, ErrMissing
			}
			if last {
				ret.Tree = t
				ret.Path = t.Path
			}
		}
		if !last {
			disk += "/"
		}
	}
	return ret, ctx.Err()
}

func dailyCreation(ctx context.Context, q CreationRequest, s CreationServices) (ret CreationResult, err error) {
	if s.Daily == nil || s.RenderPath == nil || s.Flush == nil || s.Find == nil || s.Load == nil || s.Now == nil || s.Stage == nil || s.Sync == nil {
		return ret, ErrDependency
	}
	settings, e := s.Daily(ctx, q.Box)
	if e = creationErr(ctx, e); e != nil {
		return CreationResult{}, e
	}
	if settings.SavePath == "" || settings.SavePath == "/" {
		return ret, errors.New(s.Language(49))
	}
	hp, e := s.RenderPath(ctx, settings.SavePath)
	if e = creationErr(ctx, e); e != nil {
		return CreationResult{}, e
	}
	if err = creationErr(ctx, s.Flush(ctx)); err != nil {
		return CreationResult{}, err
	}
	hp = trimCreationPath(hp)
	loc, exists, e := s.Find(ctx, "hpath", q.Box, hp, "")
	if e = creationErr(ctx, e); e != nil {
		return CreationResult{}, e
	}
	date := s.Now().Format("20060102")
	if exists {
		ret = CreationResult{ID: loc.ID, Path: loc.Path, Existed: true}
		t, e := s.Load(ctx, loc.ID)
		if e = creationErr(ctx, e); e != nil {
			return CreationResult{}, e
		}
		if t == nil || t.Root == nil {
			return CreationResult{}, ErrMissing
		}
		ret.Tree = t
		ret.Path = t.Path
		if t.Root.IALAttr(DailyNoteAttrPrefix+date) == "" {
			t.Root.SetIALAttr(DailyNoteAttrPrefix+date, date)
			if err = creationErr(ctx, s.Stage(ctx, t, "update")); err != nil {
				return CreationResult{}, err
			}
		}
		return ret, ctx.Err()
	}
	q.HPath, q.DOM = hp, ""
	ret, err = createHPath(ctx, q, s)
	if err != nil {
		return CreationResult{}, err
	}
	if settings.TemplatePath != "" {
		if s.Template == nil {
			return CreationResult{}, ErrDependency
		}
		tpl, dom, e := s.Template(ctx, settings.TemplatePath, ret.ID)
		if e = creationErr(ctx, e); e != nil {
			if s.Strict || ctx.Err() != nil {
				return CreationResult{}, e
			}
			s.warn(e)
		} else if dom != "" {
			if tpl == nil || tpl.Root == nil || s.Parser == nil {
				return CreationResult{}, ErrDependency
			}
			t, e := s.Load(ctx, ret.ID)
			if e = creationErr(ctx, e); e != nil {
				return CreationResult{}, e
			}
			if t == nil || t.Root == nil {
				return CreationResult{}, ErrMissing
			}
			if t.Root.FirstChild != nil {
				t.Root.FirstChild.Unlink()
			}
			parser, e := s.Parser(ctx)
			if e = creationErr(ctx, e); e != nil {
				return CreationResult{}, e
			}
			if parser == nil {
				return CreationResult{}, ErrDependency
			}
			body := parser.BlockDOM2Tree(dom)
			if body == nil || body.Root == nil {
				return CreationResult{}, ErrMissing
			}
			for c := body.Root.FirstChild; c != nil; {
				next := c.Next
				t.Root.AppendChild(c)
				c = next
			}
			for _, kv := range tpl.Root.KramdownIAL {
				if len(kv) != 2 {
					continue
				}
				k, v := kv[0], kv[1]
				if k == "name" || k == "alias" || k == "bookmark" || k == "memo" || k == "icon" || strings.HasPrefix(k, "custom-") {
					t.Root.SetIALAttr(k, v)
				}
			}
			t.Root.SetIALAttr("updated", s.Now().Format("20060102150405"))
			if err = creationErr(ctx, s.Stage(ctx, t, "update")); err != nil {
				return CreationResult{}, err
			}
		}
	}
	if err = creationErr(ctx, s.Sync(ctx)); err != nil {
		return CreationResult{}, err
	}
	if err = creationErr(ctx, s.Flush(ctx)); err != nil {
		return CreationResult{}, err
	}
	ret.Tree, err = s.Load(ctx, ret.ID)
	err = creationErr(ctx, err)
	if err != nil {
		return CreationResult{}, err
	}
	if ret.Tree == nil || ret.Tree.Root == nil {
		return CreationResult{}, ErrMissing
	}
	ret.Path = ret.Tree.Path
	date = s.Now().Format("20060102")
	ret.Tree.Root.SetIALAttr(DailyNoteAttrPrefix+date, date)
	err = s.Stage(ctx, ret.Tree, "update")
	return
}

func duplicateCreation(ctx context.Context, q CreationRequest, s CreationServices) (ret CreationResult, err error) {
	if s.NewID == nil || s.Now == nil || s.Stage == nil || s.Flush == nil || s.Sort == nil || s.Mirror == nil || s.Progress == nil || s.Untitled == nil {
		return ret, ErrDependency
	}
	if err = creationErr(ctx, s.Progress(ctx, true)); err != nil {
		return CreationResult{}, err
	}
	defer func() { s.Progress(context.WithoutCancel(ctx), false) }()
	t := q.Tree
	if t == nil {
		if s.Load == nil {
			return ret, ErrDependency
		}
		t, err = s.Load(ctx, q.ID)
		err = creationErr(ctx, err)
		if err != nil {
			return CreationResult{}, err
		}
	}
	if t == nil || t.Root == nil {
		return ret, ErrMissing
	}
	previous := t.Path
	t.ID = s.NewID()
	if !ast.IsNodeIDPattern(t.ID) {
		return ret, errors.New("document_creation_invalid_id")
	}
	t.Root.ID = t.ID
	title := t.Root.IALAttr("title")
	suffix := ""
	if title != s.Untitled() {
		date, e := time.Parse("20060102150405", t.ID[:14])
		if e != nil {
			date = s.Now()
		}
		suffix = " (Duplicated " + date.Format("2006-01-02 15:04:05") + ")"
	}
	t.Root.SetIALAttr("id", t.ID)
	t.Root.SetIALAttr("title", title+suffix)
	t.Root.RemoveIALAttr("scroll")
	t.Path = path.Join(path.Dir(previous), t.ID) + ".docj"
	t.HPath = t.HPath + " " + suffix
	refs := map[string]string{}
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if enter && (n.IsTextMarkType("block-ref") || n.Type == ast.NodeBlockRef) && n.TextMarkBlockRefID != "" {
			refs[n.TextMarkBlockRefID] = "1"
		}
		return ast.WalkContinue
	})
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if enter && n.Type != ast.NodeDocument && n.IsBlock() && n.ID != "" {
			old := n.ID
			n.ID = s.NewID()
			if !ast.IsNodeIDPattern(n.ID) {
				err = errors.New("document_creation_invalid_id")
				return ast.WalkStop
			}
			if refs[old] == "1" {
				refs[old] = n.ID
			}
			n.SetIALAttr("id", n.ID)
		}
		return ast.WalkContinue
	})
	if err = creationErr(ctx, err); err != nil {
		return CreationResult{}, err
	}
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref") {
			if id := refs[n.TextMarkBlockRefID]; id != "" && id != "1" {
				n.TextMarkBlockRefID = id
			}
		}
		return ast.WalkContinue
	})
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil || err != nil {
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeAttributeView {
			err = s.Mirror(ctx, n.AttributeViewID, n.ID)
		}
		if enter && n.IsBlock() {
			n.RemoveIALAttr("custom-avs")
			n.RemoveIALAttr("av-names")
			n.RemoveIALAttrsByPrefix("custom-sy-av-s-text")
		}
		return ast.WalkContinue
	})
	if err = creationErr(ctx, err); err != nil {
		return CreationResult{}, err
	}
	// Strip binding attributes before create/flush, so the persisted copy is
	// actually unbound, rather than only cleaning the in-memory tree afterwards.
	if err = creationErr(ctx, s.Stage(ctx, t, "create")); err != nil {
		return CreationResult{}, err
	}
	if err = creationErr(ctx, s.Sort(ctx, t.Box, "after", previous, t.ID)); err != nil {
		return CreationResult{}, err
	}
	if err = creationErr(ctx, s.Flush(ctx)); err != nil {
		return CreationResult{}, err
	}
	return CreationResult{ID: t.ID, Path: t.Path, Tree: t}, ctx.Err()
}
