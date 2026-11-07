package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/gabriel-vasile/mimetype"
	"io"
	"math"
	"path"
	"strings"

	"time"
	"wiki-kernel/conf"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

type NativeAVPaging struct {
	Page     int
	PageSize int
}

func (l *Lease) NativeAttributeViewRender(ctx context.Context, id, viewID, query string, page, pageSize int, groupPaging map[string]NativeAVPaging) (json.RawMessage, error) {
	return l.nativeAttributeViewRender(ctx, id, viewID, query, page, pageSize, groupPaging, false)
}

func validAVPage(p, size int) bool { return p >= 1 && p <= 1000000 && size >= 1 && size <= 1024 }

// One admission owns source/parser/clock, ASTs, candidates and computed groups.
// No legacy loader, source save, UI notifications or global cache is used.
func (l *Lease) nativeAttributeViewRender(ctx context.Context, id, viewID, query string, page, size int, paging map[string]NativeAVPaging, tableOnly bool, initial ...json.RawMessage) (ret json.RawMessage, err error) {
	return l.nativeAttributeViewRead(ctx, id, viewID, query, page, size, paging, tableOnly, false, nil, initial...)
}
func (l *Lease) nativeAttributeViewRead(ctx context.Context, id, viewID, query string, page, size int, paging map[string]NativeAVPaging, tableOnly, allRows bool, inspect func(*av.AttributeView) error, initial ...json.RawMessage) (ret json.RawMessage, err error) {
	if len(initial) > 0 {
		initial = []json.RawMessage{append(json.RawMessage(nil), initial[0]...)}
	}
	defaultHistoryPage := len(initial) > 0 && page == 1 && size == -1
	if (!validAVPage(page, size) && !defaultHistoryPage && !allRows) || len(query) > 4096 || len(paging) > 256 {
		return nil, ErrBudget
	}
	for _, p := range paging {
		if !validAVPage(p.Page, p.PageSize) {
			return nil, ErrBudget
		}
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		now := time.Now()
		duration, _ := r.configurationBusiness.Duration(r.configuration.Lang)
		var raw json.RawMessage
		var e error
		if len(initial) > 0 {
			raw = initial[0]
		} else {
			raw, _, e = readNativeAVSource(ctx, w, id)
			if e != nil {
				return e
			}
		}
		a, e := decodeNativeAV(raw, id, now)
		if e != nil {
			return e
		}
		a.DurationFormatter = duration
		v, e := a.GetCurrentView(viewID)
		if e != nil {
			return e
		}
		if allRows {
			avrender.ProjectTable(v)
			v.LayoutType = av.LayoutTypeTable
			v.Group = nil
			size = 100000
			page = 1
		}
		if tableOnly && v.LayoutType != av.LayoutTypeTable {
			return avrender.ErrUnsupported
		}
		if !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		runtime, e := r.newNativeAVRuntime(ctx, w, a, now, nil)
		if e != nil {
			return e
		}
		render, read, further, cached := runtime.render, runtime.read, runtime.further, runtime.cached
		base, e := render(a, v, query)
		if e != nil {
			return e
		}
		if e = processNativeAV(base, v, a, page, size, further[a.ID], cached); e != nil {
			return e
		}
		initial := false
		if v.LayoutType == av.LayoutTypeKanban && !v.IsGroupView() {
			key := a.GetBlockKey()
			for _, kv := range a.KeyValues {
				if kv.Key.Type == av.KeyTypeSelect {
					key = kv.Key
					break
				}
			}
			v.Group = &av.ViewGroup{Field: key.ID}
			initial = true
		}
		if v.IsGroupView() {
			key := v.GetGroupKey(a)
			if key == nil {
				return avrender.ErrShape
			}
			hasField := false
			for _, f := range base.(av.Collection).GetFields() {
				if f.GetID() == key.ID {
					hasField = true
				}
			}
			if !hasField {
				return avrender.ErrUnsupported
			}
			if e = validateNativeAVGroup(v, key); e != nil {
				return e
			}
			regenerate := key.Type == av.KeyTypeTemplate || key.Type == av.KeyTypeRelation || key.Type == av.KeyTypeRollup || v.Groups == nil || initial || (avrender.IsGroupByDate(v) && time.UnixMilli(v.GroupCreated).Format("2006-01-02") != now.Format("2006-01-02"))
			if regenerate {
				if e = avrender.GenerateGroups(v, a, avrender.GroupEnvironment{MaxGroups: 256, Now: func() time.Time { return now }, Render: render, Read: read, NewView: avrender.NewGroupView}); e != nil {
					if e == avrender.ErrLimit {
						return ErrBudget
					}
					return e
				}
			}
			if len(v.Groups) > 256 {
				return ErrBudget
			}
			if initial && (key.Type == av.KeyTypeSelect || key.Type == av.KeyTypeMSelect) {
				v.Group.Order = av.GroupOrderSelectOption
			}
			if e = avrender.LabelGroups(v, key, func(id int) (string, error) {
				if r.configurationBusiness == nil {
					return "", conf.ErrBusinessUnbound
				}
				s := r.configurationBusiness.Language(r.configuration.Lang, id, false, r.configuration.CloudRegion)
				if s == "" {
					return "", fmt.Errorf("%w: group language %d", conf.ErrBusinessUnbound, id)
				}
				return s, nil
			}); e != nil {
				return e
			}
			avrender.SortGroups(a, v, now)
			groups := []av.Viewable{}
			for _, g := range v.Groups {
				if g == nil || g.LayoutType != v.LayoutType {
					return avrender.ErrShape
				}
				if e = avrender.PrepareGroup(v, g); e != nil {
					return e
				}
				output, e := render(a, g, query)
				if e != nil {
					return e
				}
				p, s := page, size
				if override, ok := paging[g.ID]; ok {
					p, s = override.Page, override.PageSize
				}
				if e = processNativeAV(output, v, a, p, s, further[a.ID], cached); e != nil {
					return e
				}
				avrender.HideEmptyGroups(v, output)
				groups = append(groups, output)
			}
			base.SetGroups(groups)
			base.(av.Collection).SetItems(nil)
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if inspect != nil {
			if e = inspect(a); e != nil {
				return e
			}
		}
		ret, e = json.Marshal(base)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func nativeAVFieldCount(v *av.View) (int, error) {
	if v == nil {
		return 0, avrender.ErrShape
	}
	switch v.LayoutType {
	case av.LayoutTypeTable:
		if v.Table != nil && v.Table.BaseLayout != nil {
			return len(v.Table.Columns), nil
		}
	case av.LayoutTypeGallery:
		if v.Gallery != nil && v.Gallery.BaseLayout != nil {
			return len(v.Gallery.CardFields), nil
		}
	case av.LayoutTypeKanban:
		if v.Kanban != nil && v.Kanban.BaseLayout != nil {
			return len(v.Kanban.Fields), nil
		}
	}
	return 0, avrender.ErrShape
}

func validateNativeAVGroup(v *av.View, key *av.Key) error {
	if v.Group.Method < av.GroupMethodValue || v.Group.Method > av.GroupMethodDateYear {
		return avrender.ErrUnsupported
	}
	if v.Group.Method == av.GroupMethodRangeNum {
		r := v.Group.Range
		if key.Type != av.KeyTypeNumber || r == nil || r.NumStep <= 0 || math.IsNaN(r.NumStep) || math.IsInf(r.NumStep, 0) || math.IsNaN(r.NumStart) || math.IsInf(r.NumStart, 0) || math.IsNaN(r.NumEnd) || math.IsInf(r.NumEnd, 0) || r.NumStart > r.NumEnd || r.NumStart+r.NumStep <= r.NumStart || r.NumEnd+r.NumStep <= r.NumEnd || r.NumEnd-r.NumStart > r.NumStep*10000 {
			return avrender.ErrShape
		}
	}
	if avrender.IsGroupByDate(v) && key.Type != av.KeyTypeDate && key.Type != av.KeyTypeCreated && key.Type != av.KeyTypeUpdated {
		return avrender.ErrShape
	}
	return nil
}

func processNativeAV(out av.Viewable, v *av.View, a *av.AttributeView, page, size int, further map[string]av.Collection, cached map[string]*av.AttributeView) error {
	c := out.(av.Collection)
	if e := av.FilterChecked(out, a, further, cached); e != nil {
		return e
	}
	av.Sort(out, a)
	if a.DurationFormatter == nil {
		for _, f := range c.GetFields() {
			calc := f.GetCalc()
			if calc != nil && calc.Operator == av.CalcOperatorRange && (f.GetType() == av.KeyTypeDate || f.GetType() == av.KeyTypeCreated || f.GetType() == av.KeyTypeUpdated) {
				return av.ErrDurationFormatterUnbound
			}
		}
		if g := out.GetGroupCalc(); g != nil && g.FieldCalc != nil && g.FieldCalc.Operator == av.CalcOperatorRange {
			if k, _ := a.GetKey(g.Field); k != nil && (k.Type == av.KeyTypeDate || k.Type == av.KeyTypeCreated || k.Type == av.KeyTypeUpdated) {
				return av.ErrDurationFormatterUnbound
			}
		}
	}
	av.Calc(out, a)
	items := c.GetItems()
	count := len(items)
	if size < 0 {
		size = v.PageSize
	}
	start := min((page-1)*size, count)
	end := min(start+size, count)
	switch t := out.(type) {
	case *av.Table:
		t.RowCount = count
		t.PageSize = v.PageSize
	case *av.Gallery:
		t.CardCount = count
		t.PageSize = v.PageSize
	case *av.Kanban:
		t.CardCount = count
		t.PageSize = v.PageSize
	}
	c.SetItems(items[start:end])
	return nil
}

type nativeAVOverlay struct {
	Read  func(string) (*av.AttributeView, error)
	Trees func([]string) (map[string]*parse.Tree, error)
}
type nativeAVRuntime struct {
	render  func(*av.AttributeView, *av.View, string) (av.Viewable, error)
	read    func(string) (*av.AttributeView, error)
	further map[string]map[string]av.Collection
	cached  map[string]*av.AttributeView
}

// The read and edit adapters share original render/template/group dependencies.
// A transaction supplies its private SQL bundle and current candidate trees/AVs.
func (r *resources) newNativeAVRuntime(ctx context.Context, w *WorkspaceContext, a *av.AttributeView, now time.Time, overlay *nativeAVOverlay) (out nativeAVRuntime, err error) {
	duration, _ := r.configurationBusiness.Duration(r.configuration.Lang)
	err = func() error {
		settings, e := r.configuredParserOptions(ctx, w)
		if e != nil {
			return e
		}
		trees := map[string]*parse.Tree{}
		attrs := map[string]map[string]string{}
		loadTrees := func(ids []string) (map[string]*parse.Tree, error) {
			if overlay != nil && overlay.Trees != nil {
				supplied, err := overlay.Trees(ids)
				if err != nil {
					return nil, err
				}
				for _, id := range ids {
					tree := supplied[id]
					if tree == nil {
						return nil, ErrDocument
					}
					var node *ast.Node
					ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
						if enter && n.ID == id {
							node = n
							return ast.WalkStop
						}
						return ast.WalkContinue
					})
					if node == nil {
						return nil, ErrDocument
					}
					trees[id] = tree
					attrs[id] = parse.IAL2Map(node.KramdownIAL)
				}
				return trees, ctx.Err()
			}
			var missing []string
			seen := map[string]bool{}
			for _, id := range ids {
				if !documentID.MatchString(id) {
					return nil, ErrDocument
				}
				if trees[id] == nil && !seen[id] {
					missing = append(missing, id)
					seen[id] = true
				}
			}
			db, e := r.native.Database(indexdb.BlockTree)
			if e != nil {
				return nil, e
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: missing})
			if e != nil {
				return nil, e
			}
			docs := map[string]*parse.Tree{}
			for _, b := range rows {
				key := b.BoxID + "/" + b.RootID
				t := docs[key]
				if t == nil {
					t, e = r.locatedTree(ctx, w, b.BoxID, b.RootID)
					if e != nil {
						return nil, e
					}
					docs[key] = t
				}
				found := false
				ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
					if enter && n.ID == b.ID {
						attrs[b.ID] = parse.IAL2Map(n.KramdownIAL)
						found = true
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
				if !found {
					return nil, ErrDocument
				}
				trees[b.ID] = t
			}
			for _, id := range missing {
				if trees[id] == nil {
					return nil, ErrDocument
				}
			}
			return trees, ctx.Err()
		}
		cached := map[string]*av.AttributeView{a.ID: a}
		var resolver *avrender.FieldResolver
		var read func(string) (*av.AttributeView, error)
		read = func(id string) (*av.AttributeView, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if a := cached[id]; a != nil {
				return a, nil
			}
			if len(cached) >= 64 {
				return nil, ErrBudget
			}
			var a *av.AttributeView
			var e error
			if overlay != nil && overlay.Read != nil {
				a, e = overlay.Read(id)
			} else {
				var raw json.RawMessage
				raw, _, e = readNativeAVSource(ctx, w, id)
				if e == nil {
					a, e = decodeNativeAV(raw, id, now)
				}
			}
			if e != nil {
				return nil, e
			}
			a.DurationFormatter = duration
			a.ResolveField = func(key, row string) (*av.Value, error) { return resolver.Resolve(a, key, row) }
			a.ReadAttributeView = read
			cached[id] = a
			return a, nil
		}
		a.ReadAttributeView = read
		mainDB, e := r.native.Database(indexdb.Main)
		if e != nil {
			return e
		}
		funcs := indexdb.TemplateFunctions(ctx, mainDB)
		funcs["statBlock"] = func(id string) (*avrender.BlockStatResult, error) {
			ts, e := loadTrees([]string{id})
			if e != nil {
				return nil, e
			}
			t := ts[id]
			var node *ast.Node
			ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if enter && n.ID == id {
					node = n
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
			if node == nil {
				return nil, ErrDocument
			}
			if node.Type == ast.NodeDocument {
				return avrender.StatTree(t, nativeparser.NewConfiguredIndex(settings), read)
			}
			rc, wc, lc, ic, fc := node.Stat()
			return &avrender.BlockStatResult{RuneCount: rc, WordCount: wc, LinkCount: lc, ImageCount: ic, RefCount: fc, BlockCount: 1}, ctx.Err()
		}
		funcs["getHPathByID"] = func(id string) (string, error) {
			db, e := r.native.Database(indexdb.BlockTree)
			if e != nil {
				return "", e
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: []string{id}})
			if e != nil {
				return "", e
			}
			if len(rows) == 0 {
				return "", nil
			}
			return rows[0].HPath, nil
		}
		templates := avrender.TemplateEnvironment{Context: ctx, MaxOutput: 1 << 20, Now: func() time.Time { return now }, Name: func(a *av.AttributeView) string { return a.Name }, Funcs: avrender.BuiltInTemplateFuncs(func() time.Time { return now }, nativeparser.NewConfiguredIndex(settings), funcs)}
		var render func(*av.AttributeView, *av.View, string) (av.Viewable, error)
		further := map[string]map[string]av.Collection{}
		templates.IAL = func(a *av.AttributeView, row string) (map[string]string, error) {
			p := a.GetValue(a.GetBlockKey().ID, row)
			if p == nil || p.Block == nil {
				return nil, avrender.ErrShape
			}
			if p.IsDetached || p.Block.ID == "" {
				return map[string]string{}, nil
			}
			if _, e := loadTrees([]string{p.Block.ID}); e != nil {
				return nil, e
			}
			return attrs[p.Block.ID], nil
		}
		resolver = &avrender.FieldResolver{Context: ctx, Read: read, MaxNodes: 1000000, MaxDepth: 256, Templates: templates, Prepare: func(a *av.AttributeView, c av.Collection) error {
			ids := []string{}
			for _, item := range c.GetItems() {
				p := item.GetBlockValue()
				if p != nil && p.Block != nil && !p.IsDetached && p.Block.ID != "" {
					ids = append(ids, p.Block.ID)
				}
			}
			if _, e := loadTrees(ids); e != nil {
				return e
			}
			avrender.FillBasicAutoValues(a, c, attrs, func() time.Time { return now })
			return ctx.Err()
		}}
		a.ResolveField = func(key, row string) (*av.Value, error) { return resolver.Resolve(a, key, row) }

		env := avrender.Environment{
			Trees: loadTrees,
			Cover: avrender.CoverServices{Parser: nativeparser.NewConfiguredIndex(settings), IsImage: func(p string) (bool, error) {
				ext := strings.ToLower(path.Ext(p))
				if ext == "" {
					if !strings.HasPrefix(p, "assets/") {
						return false, ErrDocument
					}
					f, e := openNativeFile(w, p)
					if e != nil {
						return false, e
					}
					defer f.Close()
					m, e := mimetype.DetectReader(io.LimitReader(f, 3072))
					if e != nil {
						return false, e
					}
					ext = m.Extension()
				}
				return avrender.IsImageExtension(ext), ctx.Err()
			}},
			RemoveMissingField: func(*av.AttributeView, *av.View, string) error { return avrender.ErrShape },
			FilterItems: func(items map[string][]*av.KeyValues) error {
				db, e := r.native.Database(indexdb.BlockTree)
				if e != nil {
					return e
				}
				ids := []string{}
				for row, values := range items {
					b := av.GetKeyBlockValue(values)
					if b == nil || b.Block == nil {
						delete(items, row)
						continue
					}
					if !b.IsDetached && b.Block.ID != "" {
						if !documentID.MatchString(b.Block.ID) {
							return ErrDocument
						}
						ids = append(ids, b.Block.ID)
					}
				}
				rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
				if e != nil {
					return e
				}
				exists := map[string]bool{}
				for _, b := range rows {
					exists[b.ID] = true
				}
				for row, values := range items {
					b := av.GetKeyBlockValue(values)
					if b != nil && !b.IsDetached && !exists[b.Block.ID] {
						delete(items, row)
					}
				}
				return ctx.Err()
			},
			BlockAttrs: func(ids []string) (map[string]map[string]string, error) { _, e := loadTrees(ids); return attrs, e },
			AutoValues: func(a *av.AttributeView, c av.Collection, ials map[string]map[string]string) error {
				avrender.FillBasicAutoValues(a, c, ials, func() time.Time { return now })
				if e := ctx.Err(); e != nil {
					return e
				}
				return ctx.Err()
			},
			Templates: func(a *av.AttributeView, v *av.View, c av.Collection, ials map[string]map[string]string) error {
				if e := ctx.Err(); e != nil {
					return e
				}
				return resolver.Fill(a, c)
			},
		}
		budget := 1000000
		active := map[string]bool{}
		depth := 0
		render = func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
			key := a.ID + "/" + v.ID
			if active[key] || depth >= 7 {
				return nil, avrender.ErrLimit
			}
			active[key] = true
			depth++
			defer func() { delete(active, key); depth-- }()
			n, e := nativeAVFieldCount(v)
			if e != nil {
				return nil, e
			}
			work := len(a.GetBlockKeyValues().Values) * n
			budget -= work
			if n < 1 || n > 512 || work > 100000 || budget < 0 {
				return nil, ErrBudget
			}
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			switch v.LayoutType {
			case av.LayoutTypeTable:
				return avrender.Table(a, v, q, env)
			case av.LayoutTypeGallery:
				return avrender.Gallery(a, v, q, env)
			case av.LayoutTypeKanban:
				return avrender.Kanban(a, v, q, env)
			}
			return nil, avrender.ErrUnsupported
		}

		out = nativeAVRuntime{render: render, read: read, further: further, cached: cached}
		return ctx.Err()
	}()
	return out, err
}
