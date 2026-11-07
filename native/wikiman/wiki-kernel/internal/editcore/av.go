// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	bytes "bytes"
	context "context"
	errors "errors"
	gulu "github.com/88250/gulu"
	ast "github.com/88250/lute/ast"
	parse "github.com/88250/lute/parse"
	copier "github.com/jinzhu/copier"
	math "math"
	slices "slices"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"
	utf8 "unicode/utf8"
	av "wiki-kernel/internal/avmodel"
	avmodel "wiki-kernel/internal/avmodel"
	avrender "wiki-kernel/internal/avrender"
	blockview "wiki-kernel/internal/blockview"
	treenode "wiki-kernel/internal/docview"
	util "wiki-kernel/internal/markup"
)

type AttributeViewTextEnvironment struct {
	Ctx        context.Context
	JobContext context.Context
	Legacy     bool
	// read includes this owner’s staged candidates, including related-field refreshes.
	Read       func(string) (*av.AttributeView, error)
	Clone      func(*av.AttributeView) (*av.AttributeView, error)
	Text       func(*ast.Node, string) (string, string, error)
	Stage      func(*av.AttributeView) error
	Reload     func(string) error
	Related    func(string) error
	Complete   func() error
	Diagnostic func(level, format string, args ...any)
}

type AttributeViewUpdateEnvironment struct {
	StrictDependencies bool
	AsyncRefresh       func([]string) error
	Complete           func() error
	Ctx                context.Context
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Upsert             func(string, string) (bool, error)
	Sources            func(string) ([]string, error)
	Groups             func(*av.AttributeView) error
	Stage              func(*av.AttributeView) error
	Reload             func(string) error
	Attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	Tree               func(*parse.Tree) error
	Diagnostic         func(level, format string, args ...any)
}

type Block = blockview.Block

type AttributeViewOrderEnvironment struct {
	Complete   func() error
	Read       func(string) (*av.AttributeView, error)
	Stage      func(*av.AttributeView) error
	View       func(*av.AttributeView, string) (*av.View, error)
	Groups     func(*av.AttributeView) error
	CloneKey   func(*av.Key) (*av.Key, error)
	Defaults   *AttributeViewDefaultEnvironment
	Diagnostic func(level, format string, args ...any)
}

type GroupState = avrender.GroupState

type AttributeViewManagementEnvironment struct {
	OwnerGuard         func() error
	StrictDependencies bool
	Complete           func() error
	Ctx                context.Context
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	View               func(*av.AttributeView, string) (*av.View, error)
	Groups             func(*av.AttributeView) error
	Count              func(*av.AttributeView, *av.View, *av.View) (int, error)
	SortGroups         func(*av.AttributeView, *av.View) error
	MakeView           func(av.LayoutType) *av.View
	Language           func(string) string
	CloneView          func(*av.View) (*av.View, error)
	Mirrors            func(string) ([]string, error)
	Node               func(string) (*ast.Node, *parse.Tree, error)
	Attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	MirrorAttrs        func(*ast.Node, *parse.Tree, map[string]string) error
	FlushTrees         func([]*parse.Tree) error
	Reload             func(string) error
	Diagnostic         func(level, format string, args ...any)
}

type AttributeViewBoundEnvironment struct {
	OwnerGuard         func() error
	StrictDependencies bool
	Complete           func() error
	Ctx                context.Context
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	Groups             func(*av.AttributeView) error
	Exists             func(string) (bool, error)
	Load               func(string) (*parse.Tree, error)
	IsMirror           func(string) (bool, error)
	RemoveMirror       func(string, string) (bool, error)
	Names              func(string) (string, error)
	Attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	Cache              func(string, map[string]string) error
	Notify             func(map[string]string, *ast.Node) error
	Tree               func(*parse.Tree) error
	Reload             func(string) error
	Diagnostic         func(level, format string, args ...any)
}

type AttributeViewRowEnvironment struct {
	Cell         *AttributeViewCellEnvironment
	Defaults     *AttributeViewDefaultEnvironment
	View         func(*av.AttributeView, string) (*av.View, error)
	Text         func(*ast.Node) (string, string, error)
	OptionalNode func(string) (*ast.Node, *parse.Tree, error)
	Exists       func(string) (bool, error)
	Duplicate    func(*av.AttributeView) error
	History      func(*av.AttributeView) error
	Diagnostic   func(level, format string, args ...any)
}

const AttrAvNameTpl = `<span data-av-id="${avID}" data-popover-url="/api/av/getMirrorDatabaseBlocks" class="popover__block">${avName}</span>`

type AttributeViewCellEnvironment struct {
	StrictDependencies bool
	Complete           func() error
	Legacy             bool
	Ctx                context.Context
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	Groups             func(*av.AttributeView) error
	Node               func(string) (*ast.Node, *parse.Tree, error)
	BlockText          func(*ast.Node) (string, error)
	Names              func(string) (string, error)
	Attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	Related            func(string) error
	Now                func() time.Time
	NewID              func() string
	Color              func() string
	Diagnostic         func(level, format string, args ...any)
}

type AttributeViewEnvironment struct {
	StrictDependencies bool
	Complete           func() error
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	Groups             func(*av.AttributeView) error
	RemoveRelation     func(string, string) error
	UpsertRelation     func(string, string) error
	Reload             func(string) error
	Now                func() time.Time
	NewID              func() string
	Diagnostic         func(level, format string, args ...any)
}

var ErrInvalidAVOrderSource = errors.New("invalid_av_order_source")

type AttributeViewFieldEnvironment struct {
	StrictDependencies bool
	Complete           func() error
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	Groups             func(*av.AttributeView) error
	Sources            func(string) ([]string, error)
	RemoveRelation     func(string, string) error
	Reload             func(string) error
	Diagnostic         func(level, format string, args ...any)
}

type AttributeViewSettingsEnvironment struct {
	Complete   func() error
	Read       func(string) (*av.AttributeView, error)
	View       func(*av.AttributeView, string) (*av.View, error)
	Stage      func(*av.AttributeView) error
	Diagnostic func(level, format string, args ...any)
}

type AttributeViewOptionEnvironment struct {
	Complete   func() error
	Read       func(string) (*av.AttributeView, error)
	Stage      func(*av.AttributeView) error
	Groups     func(*av.AttributeView) error
	Diagnostic func(level, format string, args ...any)
}

const GroupValueDefault = "_@default@_" // 默认分组值（值为空的默认分组）

type AttributeViewDefaultEnvironment struct {
	Ctx          context.Context
	Near         func(*av.AttributeView, *av.View, *av.View, string) (av.Item, error)
	TemplateKeys func(*av.AttributeView, *av.Key) ([]*av.Key, error)
	Reverse      func(*av.AttributeView, *av.Key, *av.Value) error
	Now          func() time.Time
	NewID        func() string
	Diagnostic   func(level, format string, args ...any)
}

var AttributeViewManagementActions = []string{
	"sortAttrViewGroup", "removeAttrViewGroup", "syncAttrViewTableColWidth", "hideAttrViewGroup", "hideAttrViewAllGroups", "foldAttrViewGroup", "setAttrViewGroup",
	"setAttrViewCardAspectRatio", "setAttrViewBlockView", "changeAttrViewLayout", "setAttrViewWrapField", "setAttrViewShowIcon", "setAttrViewFitImage", "setAttrViewDisplayFieldName", "setAttrViewFillColBackgroundColor", "setAttrViewCardSize", "setAttrViewCoverFromAssetKeyID", "setAttrViewCoverFrom", "hideAttrViewName", "addAttrViewView",
	"sortAttrViewView", "removeAttrViewView", "duplicateAttrViewView", "setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc",
}

type AttributeViewNameEnvironment struct {
	OwnerGuard         func() error
	StrictDependencies bool
	Complete           func() error
	Names              func(string) (string, error)
	Ctx                context.Context
	Legacy             bool
	Read               func(string) (*av.AttributeView, error)
	Stage              func(*av.AttributeView) error
	Exists             func(string) (bool, error)
	Node               func(string) (*ast.Node, *parse.Tree, error)
	Name               func(string) (string, error)
	Untitled           func() string
	Tree               func(*parse.Tree) error
	Cache              func(string, map[string]string) error
	Notify             func(map[string]string, *ast.Node) error
	Diagnostic         func(level, format string, args ...any)
}

var AttributeViewFieldSettingOperations = map[string]func(*Operation, *AttributeViewSettingsEnvironment) error{
	"setAttrViewColWidth":                SetAttributeViewColWidthWithEnvironment,
	"setAttrViewColWrap":                 SetAttributeViewColWrapWithEnvironment,
	"setAttrViewColHidden":               SetAttributeViewColHiddenWithEnvironment,
	"setAttrViewColPin":                  SetAttributeViewColPinWithEnvironment,
	"setAttrViewColIcon":                 SetAttributeViewColIconWithEnvironment,
	"setAttrViewColDesc":                 SetAttributeViewColDescWithEnvironment,
	"updateAttrViewColNumberFormat":      UpdateAttributeViewColNumberFormatWithEnvironment,
	"setAttrViewColCalc":                 SetAttributeViewColumnCalcWithEnvironment,
	"setAttrViewColDateFillCreated":      SetAttributeViewColDateFillCreatedWithEnvironment,
	"setAttrViewColDateFillSpecificTime": SetAttrViewColDateFillSpecificTimeWithEnvironment,
	"setAttrViewCreatedIncludeTime":      SetAttrViewCreatedIncludeTimeWithEnvironment,
	"setAttrViewUpdatedIncludeTime":      SetAttrViewUpdatedIncludeTimeWithEnvironment,
}

type AttributeViewBindingEnvironment struct {
	StrictDependencies bool
	Ctx                context.Context
	Legacy             bool
	Node               func(string) (*ast.Node, *parse.Tree, error)
	Names              func(string) (string, error)
	Attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	Diagnostic         func(level, format string, args ...any)
}

type Operation struct {
	Action     string      `json:"action"`
	Data       interface{} `json:"data"`
	ID         string      `json:"id"`
	ParentID   string      `json:"parentID"`
	PreviousID string      `json:"previousID"`
	NextID     string      `json:"nextID"`
	RetData    interface{} `json:"retData"`
	BlockIDs   []string    `json:"blockIDs"`
	BlockID    string      `json:"blockID"`

	DeckID string `json:"deckID"` // 用于添加/删除闪卡

	AvID              string                   `json:"avID"`              // 属性视图 ID
	SrcIDs            []string                 `json:"srcIDs"`            // 用于从属性视图中删除行
	Srcs              []map[string]interface{} `json:"srcs"`              // 用于添加属性视图行（包括绑定块）{id, content, isDetached}
	IsDetached        bool                     `json:"isDetached"`        // 用于标识是否未绑定块，仅存在于属性视图中
	Name              string                   `json:"name"`              // 属性视图列名
	Typ               string                   `json:"type"`              // 属性视图列类型
	Format            string                   `json:"format"`            // 属性视图列格式化
	KeyID             string                   `json:"keyID"`             // 属性视图字段 ID
	RowID             string                   `json:"rowID"`             // 属性视图行 ID
	IsTwoWay          bool                     `json:"isTwoWay"`          // 属性视图关联列是否是双向关系
	BackRelationKeyID string                   `json:"backRelationKeyID"` // 属性视图关联列回链关联列的 ID
	RemoveDest        bool                     `json:"removeDest"`        // 属性视图删除关联目标
	Layout            av.LayoutType            `json:"layout"`            // 属性视图布局类型
	GroupID           string                   `json:"groupID"`           // 属性视图分组视图 ID
	TargetGroupID     string                   `json:"targetGroupID"`     // 属性视图目标分组视图 ID
	ViewID            string                   `json:"viewID"`            // 属性视图视图 ID
	IgnoreDefaultFill bool                     `json:"ignoreDefaultFill"` // 是否忽略默认填充

	Context map[string]interface{} `json:"context"` // 上下文信息
}

var ErrBlockNotFound = errors.New("block not found")

func RemoveAttributeViewGroup0(view *av.View) {
	view.Group, view.Groups, view.GroupCreated = nil, nil, 0
}

func GetAttrViewGroupStates(v *av.View) map[string]*GroupState { return avrender.GroupStates(v) }

func SetAttrViewGroupStates(v *av.View, s map[string]*GroupState) { avrender.RestoreGroupStates(v, s) }

func GetKanbanPreferredGroupKey(attrView *av.AttributeView) (ret *av.Key) {
	for _, kv := range attrView.KeyValues {
		if av.KeyTypeSelect == kv.Key.Type {
			ret = kv.Key
			break
		}
	}
	if nil == ret {
		ret = attrView.GetBlockKey()
	}
	return
}

func AttributeViewNamesWithServices(avIDs string, lookup func(string) (string, error), fallback func() string, ignoreErrors bool) (ret string, err error) {
	if "" == avIDs {
		return
	}
	avNames := bytes.Buffer{}
	nodeAvIDs := strings.Split(avIDs, ",")
	for _, nodeAvID := range nodeAvIDs {
		nodeAvName, getErr := lookup(nodeAvID)
		if nil != getErr {
			if !ignoreErrors {
				return "", getErr
			}
			continue
		}
		if "" == nodeAvName {
			nodeAvName = fallback()
		}

		tpl := strings.ReplaceAll(AttrAvNameTpl, "${avID}", nodeAvID)
		tpl = strings.ReplaceAll(tpl, "${avName}", nodeAvName)
		avNames.WriteString(tpl)
		avNames.WriteString("&nbsp;")
	}
	if 0 < avNames.Len() {
		avNames.Truncate(avNames.Len() - 6)
		ret = avNames.String()
	}
	return
}

func UpdateAttributeViewCellWithEnvironment(avID, keyID, itemID string, valueData interface{}, e *AttributeViewCellEnvironment) (val *av.Value, err error) {
	if err = e.Validate(); err != nil {
		return
	}
	attrView, err := e.Read(avID)
	if err != nil {
		return
	}

	val, err = UpdateAttributeViewValueWithEnvironment(e, attrView, keyID, itemID, valueData)
	if nil != err {
		return
	}
	if e.Complete != nil {
		err = e.Complete()
	}
	return
}

func CellAttributeViewBindingEnvironment(e *AttributeViewCellEnvironment) *AttributeViewBindingEnvironment {
	if e == nil {
		return nil
	}
	return &AttributeViewBindingEnvironment{StrictDependencies: e.StrictDependencies, Ctx: e.Ctx, Legacy: e.Legacy, Node: e.Node, Names: e.Names, Attrs: e.Attrs}
}

func (e *AttributeViewBindingEnvironment) Validate(load bool) error {
	if e == nil || e.Ctx == nil || e.Names == nil || e.Attrs == nil || (load && e.Node == nil) {
		return errors.New("av_binding_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func (e *AttributeViewBindingEnvironment) Guard(fn func() error) error {
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.Ctx.Err()
}

func BindBlockAvWithEnvironment(avID, blockID string, bind bool, e *AttributeViewBindingEnvironment) error {
	if err := e.Validate(true); err != nil {
		return err
	}
	if (!e.Legacy || e.StrictDependencies) && (avID == "" || blockID == "") {
		return errors.New("invalid_av_binding_identity")
	}
	var node *ast.Node
	var tree *parse.Tree
	err := e.Guard(func() (err error) { node, tree, err = e.Node(blockID); return })
	if err != nil || node == nil || tree == nil {

		if e.Legacy && (!e.StrictDependencies || errors.Is(err, ErrBlockNotFound) || (err == nil && (node == nil || tree == nil))) && e.Ctx.Err() == nil {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("av_cell_block_not_found")
	}
	if (!e.Legacy || e.StrictDependencies) && node.ID != blockID {
		return errors.New("invalid_av_binding_identity")
	}
	return BindBlockAv0WithEnvironment(avID, node, tree, bind, e)
}

func BindBlockAv0WithEnvironment(avID string, node *ast.Node, tree *parse.Tree, bind bool, e *AttributeViewBindingEnvironment) error {
	if err := e.Validate(false); err != nil {
		return err
	}
	if node == nil || tree == nil {
		if e.Legacy && !e.StrictDependencies {
			return nil
		}
		return errors.New("av_cell_block_not_found")
	}
	if (!e.Legacy || e.StrictDependencies) && (avID == "" || node.ID == "" || tree.Root == nil || tree.ID == "" || tree.Root.ID != tree.ID || tree.Box == "" || tree.Path == "" || treenode.GetNodeInTree(tree, node.ID) != node) {
		return errors.New("invalid_av_binding_identity")
	}
	attrs, err := CellBindingAttrs(avID, node, bind, func(ids string) (names string, err error) {
		err = e.Guard(func() (err error) { names, err = e.Names(ids); return })
		return
	})
	if err != nil {
		return err
	}
	if attrs == nil {
		return e.Ctx.Err()
	}
	return e.Guard(func() error { return e.Attrs(node, tree, attrs) })
}

func (e *AttributeViewBoundEnvironment) Validate(mode string) error {
	if e == nil || e.Ctx == nil || e.Exists == nil || e.Load == nil {
		return errors.New("av_bound_dependencies_unbound")
	}
	if mode != "collect" && (e.Read == nil || e.Names == nil || (mode == "insert" && (e.Attrs == nil || e.Tree == nil))) {
		return errors.New("av_bound_dependencies_unbound")
	}
	if mode == "delete" && (e.Stage == nil || e.Groups == nil || e.IsMirror == nil || e.RemoveMirror == nil || e.Cache == nil || e.Notify == nil || e.Tree == nil || e.Reload == nil) {
		return errors.New("av_bound_delete_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func (e *AttributeViewBoundEnvironment) Guard(fn func() error) error {
	if e.OwnerGuard != nil {
		if err := e.OwnerGuard(); err != nil {
			return err
		}
	}
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	if e.OwnerGuard != nil {
		return e.OwnerGuard()
	}
	return e.Ctx.Err()
}

func CollectAttributeViewBoundNodes(a *av.AttributeView, e *AttributeViewBoundEnvironment) (map[string]*parse.Tree, []*ast.Node, error) {
	if err := e.Validate("collect"); err != nil {
		return nil, nil, err
	}
	if err := ValidateCellSource(a); err != nil {
		return nil, nil, err
	}
	primary := a.GetBlockKeyValues()
	if primary == nil {
		return nil, nil, errors.New("av_bound_primary_missing")
	}
	trees, owned := map[string]*parse.Tree{}, map[string]*parse.Tree{}
	var nodes []*ast.Node
	for _, value := range primary.Values {
		if err := e.Ctx.Err(); err != nil {
			return nil, nil, err
		}
		if value.IsDetached {
			continue
		}
		id := value.Block.ID
		var exists bool
		if err := e.Guard(func() (err error) { exists, err = e.Exists(id); return }); err != nil {
			return nil, nil, err
		}
		if !exists {
			continue
		}
		tree := trees[id]
		if tree == nil {
			if err := e.Guard(func() (err error) { tree, err = e.Load(id); return }); err != nil {
				if e.Legacy && !e.StrictDependencies && e.Ctx.Err() == nil {
					continue
				}
				return nil, nil, err
			}
		}
		if tree == nil || tree.Root == nil || tree.ID == "" || tree.Box == "" || tree.Path == "" {
			if e.Legacy {
				continue
			}
			return nil, nil, errors.New("invalid_av_bound_tree")
		}
		if first := owned[tree.ID]; first != nil {
			if first.Box != tree.Box || first.Path != tree.Path {
				return nil, nil, errors.New("invalid_av_bound_tree_identity")
			}
			tree = first
		} else {
			owned[tree.ID] = tree
		}
		trees[id] = tree
		if node := treenode.GetNodeInTree(tree, id); node != nil {
			nodes = append(nodes, node)
		}
	}
	if err := e.Ctx.Err(); err != nil {
		return nil, nil, err
	}
	return trees, nodes, nil
}

func DeleteAttributeViewBindings(node *ast.Node, nodeTree *parse.Tree, e *AttributeViewBoundEnvironment) error {
	if err := e.Validate("delete"); err != nil {
		return err
	}
	if node == nil || nodeTree == nil || nodeTree.Root == nil {
		return errors.New("invalid_av_bound_delete")
	}
	var walked []*ast.Node
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering {
			walked = append(walked, n)
		}
		return ast.WalkContinue
	})
	var changed []string
	read := func(id string) (a *av.AttributeView, err error) {
		err = e.Guard(func() (err error) { a, err = e.Read(id); return })
		if err != nil {
			return nil, err
		}
		if err = ValidateCellSource(a); err != nil {
			return nil, err
		}
		if a.ID != id || a.GetBlockKeyValues() == nil {
			return nil, errors.New("invalid_av_bound_source")
		}
		return
	}

	for _, n := range walked {
		if err := e.Ctx.Err(); err != nil {
			return err
		}
		if !n.IsBlock() || n.IALAttr(av.NodeAttrNameAvs) == "" {
			continue
		}
		for _, id := range strings.Split(n.IALAttr(av.NodeAttrNameAvs), ",") {
			a, err := read(id)
			if err != nil {
				if e.Legacy && !e.StrictDependencies && e.Ctx.Err() == nil {
					continue
				}
				return err
			}
			primary := a.GetBlockKeyValues()
			for i, v := range primary.Values {
				if v.Block.ID != n.ID {
					continue
				}
				primary.Values = append(primary.Values[:i], primary.Values[i+1:]...)
				if err = e.Guard(func() error { return e.Groups(a) }); err != nil {
					return err
				}
				if err = e.Guard(func() error { return e.Stage(a) }); err != nil {
					return err
				}
				changed = append(changed, id)
				break
			}
		}
	}
	for _, n := range walked {
		if err := e.Ctx.Err(); err != nil {
			return err
		}
		if n.Type != ast.NodeAttributeView {
			continue
		}
		id := n.AttributeViewID
		var mirror, removed bool
		if err := e.Guard(func() (err error) { mirror, err = e.IsMirror(id); return }); err != nil {
			return err
		}
		if err := e.Guard(func() (err error) { removed, err = e.RemoveMirror(id, n.ID); return }); err != nil {
			return err
		}
		if removed {
			changed = append(changed, id)
		}
		if mirror {
			continue
		}
		a, err := read(id)
		if err != nil {
			if e.Legacy && !e.StrictDependencies && e.Ctx.Err() == nil {
				continue
			}
			return err
		}
		trees, nodes, err := CollectAttributeViewBoundNodes(a, e)
		if err != nil {
			return err
		}
		var changedTrees []*parse.Tree
		seen := map[string]bool{}
		for _, target := range nodes {
			old := parse.IAL2Map(target.KramdownIAL)
			if ids := target.IALAttr(av.NodeAttrNameAvs); ids != "" {
				ids := gulu.Str.RemoveElem(strings.Split(ids, ","), id)
				if len(ids) == 0 {
					target.RemoveIALAttr(av.NodeAttrNameAvs)
				} else {
					target.SetIALAttr(av.NodeAttrNameAvs, strings.Join(ids, ","))
				}
			}
			var names string
			if err = e.Guard(func() (err error) { names, err = e.Names(target.IALAttr(av.NodeAttrNameAvs)); return }); err != nil {
				return err
			}
			target.SetIALAttr(av.NodeAttrViewNames, names)
			if err = e.Guard(func() error { return e.Cache(target.ID, parse.IAL2Map(target.KramdownIAL)) }); err != nil {
				return err
			}
			if err = e.Guard(func() error { return e.Notify(old, target) }); err != nil {
				return err
			}
			tree := trees[target.ID]
			if !seen[tree.ID] {
				seen[tree.ID] = true
				changedTrees = append(changedTrees, tree)
			}
		}
		for _, tree := range changedTrees {
			if e.Legacy && tree.ID == nodeTree.ID {
				continue
			}
			if err = e.Guard(func() error { return e.Tree(tree) }); err != nil {
				return err
			}
		}
	}
	for _, id := range gulu.Str.RemoveDuplicatedElem(changed) {
		if err := e.Guard(func() error { return e.Reload(id) }); err != nil {
			return err
		}
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return e.Ctx.Err()
}

func (e *AttributeViewCellEnvironment) Validate() error {
	if e == nil || e.Ctx == nil || e.Read == nil || e.Stage == nil || e.Groups == nil || e.Node == nil || e.BlockText == nil || e.Names == nil || e.Attrs == nil || e.Related == nil || e.Now == nil || e.NewID == nil || e.Color == nil {
		return errors.New("transaction_av_cell_dependencies_unbound")
	}
	return e.Ctx.Err()
}

// Preserve original binding/attribute algorithms for both adapters; legacy
// wrappers keep best-effort behavior, scoped callers must propagate errors.
func CellBindingAttrs(avID string, n *ast.Node, bind bool, names func(string) (string, error)) (map[string]string, error) {
	attrs := parse.IAL2Map(n.KramdownIAL)
	if !bind && attrs[av.NodeAttrNameAvs] == "" {
		return nil, nil
	}
	ids := []string{}
	if attrs[av.NodeAttrNameAvs] != "" {
		ids = strings.Split(attrs[av.NodeAttrNameAvs], ",")
	}
	var next []string
	seen := map[string]bool{}
	if bind {
		ids = append(ids, avID)
	}
	for _, id := range ids {
		if !bind && id == avID {
			continue
		}
		if bind && seen[id] {
			continue
		}
		seen[id] = true
		next = append(next, id)
	}
	attrs[av.NodeAttrNameAvs] = strings.Join(next, ",")
	s, err := names(attrs[av.NodeAttrNameAvs])
	if err != nil {
		return nil, err
	}
	if s != "" {
		attrs[av.NodeAttrViewNames] = s
	}
	return attrs, nil
}

func (e *AttributeViewCellEnvironment) Binding(avID, blockID string, bind bool) error {
	binding := CellAttributeViewBindingEnvironment(e)

	if binding != nil && binding.Legacy && !e.StrictDependencies && binding.Attrs != nil {
		binding.Attrs = func(n *ast.Node, t *parse.Tree, attrs map[string]string) error {
			_ = e.Attrs(n, t, attrs)
			return nil
		}
	}
	return BindBlockAvWithEnvironment(avID, blockID, bind, binding)
}

func (e *AttributeViewCellEnvironment) StaticText(n *ast.Node, t *parse.Tree, avID, text string) error {
	if n == nil {
		if e.Legacy && !e.StrictDependencies {
			return nil
		}
		return errors.New("av_cell_block_not_found")
	}
	attrs := parse.IAL2Map(n.KramdownIAL)
	attrs[av.NodeAttrViewStaticText+"-"+avID] = text
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	err := e.Attrs(n, t, attrs)
	if e.Legacy && !e.StrictDependencies {
		return nil
	}
	return err
}

func ValidateCellValue(a *av.AttributeView, keyID, itemID string, data interface{}) error {
	if a == nil || keyID == "" || itemID == "" {
		return errors.New("invalid_av_cell_identity")
	}
	if err := ValidateCellSource(a); err != nil {
		return err
	}
	key, err := a.GetKey(keyID)
	if err != nil || key == nil {
		return errors.New("av_cell_key_not_found")
	}
	if data == nil {
		return errors.New("invalid_av_cell_value")
	}
	return nil
}

func ValidateCellSource(a *av.AttributeView) error {
	if a == nil {
		return errors.New("invalid_av_cell_source")
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return errors.New("invalid_av_cell_source")
		}
		for _, option := range kv.Key.Options {
			if option == nil {
				return errors.New("invalid_av_cell_source")
			}
		}
		for _, v := range kv.Values {
			if v == nil || (kv.Key.Type == av.KeyTypeBlock && v.Block == nil) {
				return errors.New("invalid_av_cell_source")
			}
		}
	}
	return nil
}

func AttrViewKeyType(a *av.AttributeView, id string) av.KeyType {
	k, _ := a.GetKey(id)
	if k == nil {
		return ""
	}
	return k.Type
}

func UpdateAttributeViewValueWithEnvironment(e *AttributeViewCellEnvironment, attrView *av.AttributeView, keyID, itemID string, valueData interface{}) (val *av.Value, err error) {
	if err = e.Validate(); err != nil {
		return
	}
	if err = ValidateCellValue(attrView, keyID, itemID, valueData); err != nil {
		return
	}
	avID := attrView.ID
	var blockVal *av.Value
	for _, kv := range attrView.KeyValues {
		if av.KeyTypeBlock == kv.Key.Type {
			for _, v := range kv.Values {
				if itemID == v.BlockID {
					blockVal = v
					break
				}
			}
			break
		}
	}

	now := e.Now().UnixMilli()
	oldIsDetached := true
	var oldBoundBlockID string
	if nil != blockVal {
		oldIsDetached = blockVal.IsDetached
		oldBoundBlockID = blockVal.Block.ID
	}
	for _, keyValues := range attrView.KeyValues {
		if keyID != keyValues.Key.ID {
			continue
		}

		for _, value := range keyValues.Values {
			if itemID == value.BlockID {
				val = value
				val.Type = keyValues.Key.Type
				break
			}
		}

		if nil == val {
			val = &av.Value{ID: e.NewID(), KeyID: keyID, BlockID: itemID, Type: keyValues.Key.Type, CreatedAt: now, UpdatedAt: now}
			keyValues.Values = append(keyValues.Values, val)
		}
		break
	}

	if val == nil {
		return nil, errors.New("av_cell_key_not_found")
	}
	isUpdatingBlockKey := av.KeyTypeBlock == val.Type
	var oldRelationBlockIDs []string
	if av.KeyTypeRelation == val.Type {
		if nil != val.Relation {
			for _, bID := range val.Relation.BlockIDs {
				oldRelationBlockIDs = append(oldRelationBlockIDs, bID)
			}
		}
	}
	data, err := gulu.JSON.MarshalJSON(valueData)
	if err != nil {
		e.
			Report("error", "marshal value [%+v] failed: %s", valueData, err)
		return
	}
	if err = gulu.JSON.UnmarshalJSON(data, &val); err != nil {
		e.
			Report("error", "unmarshal data [%s] failed: %s", data, err)
		return
	}

	if val == nil || (!e.Legacy && (val.Type != AttrViewKeyType(attrView, keyID) || val.KeyID != keyID || val.BlockID != itemID)) {
		return nil, errors.New("invalid_av_cell_value")
	}
	if (val.Type == av.KeyTypeBlock && val.Block == nil) || (val.Type == av.KeyTypeRelation && val.Relation == nil) {
		return nil, errors.New("invalid_av_cell_value")
	}
	key, _ := attrView.GetKey(keyID)

	if av.KeyTypeNumber == val.Type {
		if nil != val.Number {
			if !val.Number.IsNotEmpty {
				val.Number.Content = 0
				val.Number.FormattedContent = ""
			} else {
				val.Number.FormatNumber()
			}
		}
	} else if av.KeyTypeDate == val.Type {
		if nil != val.Date && !val.Date.IsNotEmpty {
			val.Date.Content = 0
			val.Date.FormattedContent = ""
		}
	} else if av.KeyTypeSelect == val.Type || av.KeyTypeMSelect == val.Type {
		if nil != key && 0 < len(val.MSelect) {
			var tmp []*av.ValueSelect

			for _, v := range val.MSelect {
				if v == nil {
					return nil, errors.New("invalid_av_cell_option")
				}
				if "" != v.Content {
					tmp = append(tmp, v)
				}
			}
			val.MSelect = tmp

			if 1 > len(val.MSelect) {
				return
			}

			for _, valOpt := range val.MSelect {
				if opt := key.GetOption(valOpt.Content); nil == opt {

					color := valOpt.Color
					if "" == color {
						color = e.Color()
					}
					opt = &av.SelectOption{Name: valOpt.Content, Color: color}
					key.Options = append(key.Options, opt)
				} else {

					valOpt.Color = opt.Color
				}
			}
		}
	}

	relationChangeMode := 0
	if av.KeyTypeRelation == val.Type {

		val.Relation.Contents = nil
		val.Relation.BlockIDs = gulu.Str.RemoveDuplicatedElem(val.Relation.BlockIDs)

		if !slices.Equal(oldRelationBlockIDs, val.Relation.BlockIDs) {
			if len(oldRelationBlockIDs) > len(val.Relation.BlockIDs) {
				relationChangeMode = 2
			} else {
				relationChangeMode = 1
			}
		}
	}

	if isUpdatingBlockKey {
		if oldIsDetached {

			if !val.IsDetached {
				if err = e.Binding(avID, val.Block.ID, true); err != nil {
					return
				}
			}
		} else {

			if val.IsDetached {
				if err = e.Binding(avID, val.Block.ID, false); err != nil {
					return
				}
				val.Block.ID = ""
			} else {

				if oldBoundBlockID != val.Block.ID {

					if err = e.Binding(avID, oldBoundBlockID, false); err != nil {
						return
					}
					if err = e.Binding(avID, val.Block.ID, true); err != nil {
						return
					}
					val.Block.Content = util.UnescapeHTML(val.Block.Content)
				} else {
					content := strings.TrimSpace(val.Block.Content)
					node, tree, loadErr := e.Node(val.Block.ID)
					if (!e.Legacy || e.StrictDependencies) && (loadErr != nil || node == nil || tree == nil) {
						if loadErr != nil {
							return nil, loadErr
						}
						return nil, errors.New("av_cell_block_not_found")
					}
					if err = e.Ctx.Err(); err != nil {
						return
					}
					blockText, textErr := e.BlockText(node)
					if textErr != nil {
						return nil, textErr
					}
					if "" == content {

						val.Block.Content = util.UnescapeHTML(blockText)
						if err = e.StaticText(node, tree, avID, ""); err != nil {
							return
						}
					} else {
						val.Block.Content = content
						if err = e.StaticText(node, tree, avID, content); err != nil {
							return
						}
					}
				}
			}
		}
	}

	if nil != blockVal {
		blockVal.Block.Updated = now
		blockVal.SetUpdatedAt(now)
		if isUpdatingBlockKey {
			blockVal.IsDetached = val.IsDetached
		}
	}
	val.SetUpdatedAt(now)

	if nil != key && av.KeyTypeRelation == key.Type && nil != key.Relation && key.Relation.IsTwoWay {

		if err = UpdateTwoWayRelationDestWithEnvironment(e, attrView, key, val, relationChangeMode, oldRelationBlockIDs); err != nil {
			return
		}
	}

	if err = e.Ctx.Err(); err != nil {
		return
	}
	if err = e.Groups(attrView); err != nil {
		return
	}
	if err = e.Ctx.Err(); err != nil {
		return
	}
	if err = e.Stage(attrView); err != nil {
		return
	}
	if err = e.Ctx.Err(); err != nil {
		return
	}
	if err = e.Related(avID); err != nil {
		return
	}
	err = e.Ctx.Err()
	return
}

func UpdateTwoWayRelationDestWithEnvironment(e *AttributeViewCellEnvironment, attrView *av.AttributeView, relKey *av.Key, val *av.Value, relationChangeMode int, oldRelationBlockIDs []string) (err error) {
	var destAv *av.AttributeView
	if attrView.ID == relKey.Relation.AvID {
		destAv = attrView
	} else {
		if err = e.Ctx.Err(); err != nil {
			return
		}
		destAv, err = e.Read(relKey.Relation.AvID)
		if err != nil {
			if e.Legacy && !e.StrictDependencies {
				return nil
			}
			return
		}
	}

	if nil == destAv {
		if e.Legacy && !e.StrictDependencies {
			return nil
		}
		return errors.New("av_cell_relation_target_not_found")
	}
	if !e.Legacy || e.StrictDependencies {
		if err = e.Ctx.Err(); err != nil {
			return
		}
		if err = ValidateCellSource(destAv); err != nil {
			return
		}
		key, ke := destAv.GetKey(relKey.Relation.BackKeyID)
		if ke != nil || key == nil || key.Type != av.KeyTypeRelation {
			return errors.New("av_cell_back_key_not_found")
		}
	}

	now := e.Now().UnixMilli()
	if 1 == relationChangeMode {
		addBlockIDs := val.Relation.BlockIDs
		for _, bID := range oldRelationBlockIDs {
			addBlockIDs = gulu.Str.RemoveElem(addBlockIDs, bID)
		}

		for _, blockID := range addBlockIDs {
			for _, keyValues := range destAv.KeyValues {
				if keyValues.Key.ID != relKey.Relation.BackKeyID {
					continue
				}

				destVal := keyValues.GetValue(blockID)
				if nil == destVal {
					destVal = &av.Value{ID: e.NewID(), KeyID: keyValues.Key.ID, BlockID: blockID, Type: keyValues.Key.Type, Relation: &av.ValueRelation{}, CreatedAt: now, UpdatedAt: now + 1000}
					keyValues.Values = append(keyValues.Values, destVal)
				}

				if destVal.Relation == nil {
					return errors.New("invalid_av_cell_back_relation")
				}
				destVal.Relation.BlockIDs = append(destVal.Relation.BlockIDs, val.BlockID)
				destVal.Relation.BlockIDs = gulu.Str.RemoveDuplicatedElem(destVal.Relation.BlockIDs)
				break
			}
		}
	} else if 2 == relationChangeMode {
		removeBlockIDs := oldRelationBlockIDs
		for _, bID := range val.Relation.BlockIDs {
			removeBlockIDs = gulu.Str.RemoveElem(removeBlockIDs, bID)
		}

		for _, blockID := range removeBlockIDs {
			for _, keyValues := range destAv.KeyValues {
				if keyValues.Key.ID != relKey.Relation.BackKeyID {
					continue
				}

				for _, value := range keyValues.Values {
					if value.BlockID == blockID {
						if value.Relation == nil {
							return errors.New("invalid_av_cell_back_relation")
						}
						value.Relation.BlockIDs = gulu.Str.RemoveElem(value.Relation.BlockIDs, val.BlockID)
						value.SetUpdatedAt(now)
						break
					}
				}
			}
		}
	}

	if destAv != attrView {
		if err = e.Ctx.Err(); err != nil {
			return
		}
		if err = e.Groups(destAv); err != nil {
			return
		}
		if err = e.Ctx.Err(); err != nil {
			return
		}
		err = e.Stage(destAv)
		if e.Legacy && !e.StrictDependencies {
			return nil
		}
		if err != nil {
			return
		}
	}
	return e.Ctx.Err()
}

func AttributeViewDefaultValues(attrView *av.AttributeView, view, groupView *av.View, previousItemID, addingItemID string, isCreate bool, env *AttributeViewDefaultEnvironment) (ret map[string]*av.Value, err error) {
	if err = env.Validate(); err != nil {
		return nil, err
	}
	if view == nil || attrView == nil {
		return nil, ErrInvalidAVOrderSource
	}
	if view.IsGroupView() && (groupView == nil || groupView.GroupVal == nil) {
		return nil, ErrInvalidAVOrderSource
	}
	if groupView != nil && groupView.GroupVal != nil && !ValidAttributeViewDefaultFilter(&av.ViewFilter{Value: groupView.GroupVal}) {
		return nil, ErrInvalidAVOrderSource
	}
	ret = map[string]*av.Value{}

	if 1 > len(view.Filters) && !view.IsGroupView() {

		return
	}
	for _, f := range view.Filters {
		if f == nil || !ValidAttributeViewDefaultFilter(f) {
			return nil, ErrInvalidAVOrderSource
		}
	}

	nearItem, err := env.Near(attrView, view, groupView, previousItemID)
	if err != nil {
		return nil, err
	}

	templateRelevantKeys, rollupRelevantKeys := map[string][]*av.Key{}, map[string]*av.Key{}
	for _, keyValues := range attrView.KeyValues {
		if av.KeyTypeTemplate == keyValues.Key.Type {
			tplRelevantKeys, readErr := env.TemplateKeys(attrView, keyValues.Key)
			if readErr != nil {
				return nil, readErr
			}
			if 0 < len(tplRelevantKeys) {
				for _, k := range tplRelevantKeys {
					templateRelevantKeys[keyValues.Key.ID] = append(templateRelevantKeys[keyValues.Key.ID], k)
				}
			}
		} else if av.KeyTypeRollup == keyValues.Key.Type {
			if nil != keyValues.Key.Rollup {
				relKey, _ := attrView.GetKey(keyValues.Key.Rollup.RelationKeyID)
				if nil != relKey && nil != relKey.Relation {
					if attrView.ID == relKey.Relation.AvID {
						if k, _ := attrView.GetKey(keyValues.Key.Rollup.KeyID); nil != k {
							rollupRelevantKeys[k.ID] = k
						}
					}
				}
			}
		}
	}

	filterKeyIDs := map[string]bool{}
	for _, filter := range view.Filters {
		if filter == nil || !ValidAttributeViewDefaultFilter(filter) {
			return nil, ErrInvalidAVOrderSource
		}
		filterKeyIDs[filter.Column] = true
		keyValues, _ := attrView.GetKeyValues(filter.Column)
		if nil == keyValues {
			continue
		}

		if av.KeyTypeTemplate == keyValues.Key.Type && nil != nearItem {
			if keys := templateRelevantKeys[keyValues.Key.ID]; 0 < len(keys) {
				for _, k := range keys {
					if nil == ret[k.ID] {
						ret[k.ID] = env.NearValue(nearItem, k, addingItemID)
					}
				}
			}
			continue
		}

		if av.KeyTypeRollup == keyValues.Key.Type && nil != nearItem {
			if relKey, ok := rollupRelevantKeys[keyValues.Key.ID]; ok {
				if nil == ret[relKey.ID] {
					ret[relKey.ID] = env.NearValue(nearItem, relKey, addingItemID)
				}
			}
			continue
		}

		if av.KeyTypeMAsset == keyValues.Key.Type {
			if nil != nearItem {
				if _, ok := ret[keyValues.Key.ID]; !ok {
					ret[keyValues.Key.ID] = env.NearValue(nearItem, keyValues.Key, addingItemID)
				}
			}
			return
		}

		newValue := filter.GetAffectValueWithServices(keyValues.Key, addingItemID, env.Now, env.NewID)
		if nil == newValue {
			newValue = env.NearValue(nearItem, keyValues.Key, addingItemID)
		}
		if nil != newValue {
			if av.KeyTypeDate == keyValues.Key.Type {
				if newValue.Date == nil {
					return nil, ErrInvalidAVOrderSource
				}
				if nil != nearItem {
					nearValue := env.NearValue(nearItem, keyValues.Key, addingItemID)
					if nearValue == nil || nearValue.Date == nil {
						return nil, ErrInvalidAVOrderSource
					}
					newValue.Date.IsNotTime = nearValue.Date.IsNotTime
				}

				if nil != keyValues.Key.Date && keyValues.Key.Date.AutoFillNow {
					newValue.Date.Content = env.Now().UnixMilli()
					newValue.Date.IsNotEmpty = true
				}
			}

			ret[keyValues.Key.ID] = newValue
		}
	}

	groupKey := view.GetGroupKey(attrView)
	if nil == groupKey {
		return
	}

	keyValues, _ := attrView.GetKeyValues(groupKey.ID)
	if nil == keyValues {
		return
	}

	newValue := env.NearValue(nearItem, groupKey, addingItemID)
	if av.KeyTypeSelect == groupKey.Type || av.KeyTypeMSelect == groupKey.Type {

		if opt := groupKey.GetOption(groupView.GetGroupValue()); nil != opt && GroupValueDefault != groupView.GetGroupValue() {
			if nil == newValue {
				newValue = ret[groupKey.ID]
			}
			if nil == newValue {
				newValue = keyValues.GetValue(addingItemID)
			}

			if nil != newValue {
				if !av.MSelectExistOption(newValue.MSelect, groupView.GetGroupValue()) {
					if av.KeyTypeSelect == groupKey.Type {

						newValue.MSelect = []*av.ValueSelect{{Content: opt.Name, Color: opt.Color}}
					} else if 1 > len(newValue.MSelect) || av.KeyTypeMSelect == groupKey.Type {
						newValue.MSelect = append(newValue.MSelect, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
					}
				} else {
					var vals []*av.ValueSelect
					if isCreate {
						vals = append(vals, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
					} else {
						existingVal := keyValues.GetValue(addingItemID)
						if nil != existingVal {
							if !av.MSelectExistOption(existingVal.MSelect, opt.Name) {
								existingVal.MSelect = append(existingVal.MSelect, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
							}
							vals = existingVal.MSelect
						} else {
							vals = append(vals, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
						}
					}

					if nil != ret[groupKey.ID] {
						for _, v := range ret[groupKey.ID].MSelect {
							if !av.MSelectExistOption(vals, v.Content) {
								vals = append(vals, v)
							}
						}
					}
					newValue.MSelect = vals
				}
			} else {
				newValue = avmodel.GetAttributeViewDefaultValueWithServices(env.NewID(), groupKey.ID, addingItemID, groupKey.Type, false, env.Now, env.NewID)
				newValue.MSelect = append(newValue.MSelect, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
			}
		}

		if nil != newValue {
			ret[groupKey.ID] = newValue
		}
		return
	}

	if av.KeyTypeTemplate == keyValues.Key.Type && nil != nearItem {
		if keys := templateRelevantKeys[keyValues.Key.ID]; 0 < len(keys) {
			for _, k := range keys {
				if nil == ret[k.ID] {
					ret[k.ID] = env.NearValue(nearItem, k, addingItemID)
				}
			}
		}
		return
	}

	if av.KeyTypeRollup == keyValues.Key.Type && nil != nearItem {
		if relKey, ok := rollupRelevantKeys[keyValues.Key.ID]; ok {
			if nil == ret[relKey.ID] {
				ret[relKey.ID] = env.NearValue(nearItem, relKey, addingItemID)
			}
		}
		return
	}

	if nil != nearItem && filterKeyIDs[groupKey.ID] {

		newValue = env.NearValue(nearItem, groupKey, addingItemID)
		ret[groupKey.ID] = newValue

		if nil != keyValues.Key.Date && keyValues.Key.Date.AutoFillNow {
			newValue.Date.Content = env.Now().UnixMilli()
			newValue.Date.IsNotEmpty = true
		}
		return
	}

	if nil == nearItem && !filterKeyIDs[groupKey.ID] {

		newValue = avmodel.GetAttributeViewDefaultValueWithServices(env.NewID(), groupKey.ID, addingItemID, groupKey.Type, false, env.Now, env.NewID)
		if av.KeyTypeText == groupView.GroupVal.Type {
			content := groupView.GroupVal.Text.Content
			if GroupValueDefault == content {
				content = ""
			}

			switch newValue.Type {
			case av.KeyTypeBlock:
				newValue.Block.Content = content
			case av.KeyTypeText:
				newValue.Text.Content = content
			case av.KeyTypeNumber:
				num, _ := strconv.ParseFloat(strings.Split(content, " - ")[0], 64)
				newValue.Number.Content = num
				newValue.Number.IsNotEmpty = true
			case av.KeyTypeURL:
				newValue.URL.Content = content
			case av.KeyTypeEmail:
				newValue.Email.Content = content
			case av.KeyTypePhone:
				newValue.Phone.Content = content
			}
		} else if av.KeyTypeCheckbox == groupView.GroupVal.Type {
			newValue.Checkbox.Checked = groupView.GroupVal.Checkbox.Checked
		}

		ret[groupKey.ID] = newValue
		return
	}

	if nil != newValue && !filterKeyIDs[groupKey.ID] {
		ret[groupKey.ID] = newValue

		if nil != keyValues.Key.Date && keyValues.Key.Date.AutoFillNow {
			newValue.Date.Content = env.Now().UnixMilli()
			newValue.Date.IsNotEmpty = true
		}
	}
	return
}

func FillAttributeViewDefaultValues(attrView *av.AttributeView, view, groupView *av.View, previousItemID, addingItemID string, isCreate bool, env *AttributeViewDefaultEnvironment) error {
	defaultValues, err := AttributeViewDefaultValues(attrView, view, groupView, previousItemID, addingItemID, isCreate, env)
	if err != nil {
		return err
	}
	for keyID, newValue := range defaultValues {
		if err = env.Ctx.Err(); err != nil {
			return err
		}
		if newValue == nil {
			return ErrInvalidAVOrderSource
		}
		newValue.BlockID = addingItemID
		keyValues, getErr := attrView.GetKeyValues(keyID)
		if nil != getErr {
			continue
		}

		if av.KeyTypeRollup == newValue.Type {

			continue
		}

		if (av.KeyTypeSelect == newValue.Type || av.KeyTypeMSelect == newValue.Type) && 1 > len(newValue.MSelect) && GroupValueDefault != groupView.GetGroupValue() {

			if opt := keyValues.Key.GetOption(groupView.GetGroupValue()); nil != opt {
				newValue.MSelect = append(newValue.MSelect, &av.ValueSelect{Content: opt.Name, Color: opt.Color})
			}
		}

		if av.KeyTypeRelation == newValue.Type && nil != keyValues.Key.Relation && keyValues.Key.Relation.IsTwoWay {
			if newValue.Relation == nil {
				return ErrInvalidAVOrderSource
			}

			if err = env.Reverse(attrView, keyValues.Key, newValue); err != nil {
				return err
			}
		}

		existingVal := keyValues.GetValue(addingItemID)
		if nil == existingVal {
			newValue.IsRenderAutoFill = false
			keyValues.Values = append(keyValues.Values, newValue)
		} else {
			newValueRaw := newValue.GetValByType(keyValues.Key.Type)
			if av.KeyTypeBlock != existingVal.Type || (av.KeyTypeBlock == existingVal.Type && existingVal.IsDetached) {

				existingVal.IsRenderAutoFill = false
				existingVal.SetValByType(keyValues.Key.Type, newValueRaw)
			}
		}
	}
	return nil
}

func ValidAttributeViewDefaultFilter(f *av.ViewFilter) bool {
	v := f.Value
	if v == nil {
		return true
	}
	switch v.Type {
	case av.KeyTypeText:
		return v.Text != nil
	case av.KeyTypeBlock:
		return v.Block != nil
	case av.KeyTypeNumber:
		return v.Number != nil
	case av.KeyTypeURL:
		return v.URL != nil
	case av.KeyTypeEmail:
		return v.Email != nil
	case av.KeyTypePhone:
		return v.Phone != nil
	case av.KeyTypeCheckbox:
		return v.Checkbox != nil
	case av.KeyTypeSelect, av.KeyTypeMSelect:
		for _, s := range v.MSelect {
			if s == nil {
				return false
			}
		}
	}
	return true
}

func (e *AttributeViewEnvironment) Validate() error {
	if e == nil || e.Read == nil || e.Stage == nil || e.Groups == nil || e.RemoveRelation == nil || e.UpsertRelation == nil || e.Reload == nil || e.Now == nil || e.NewID == nil {
		return errors.New("transaction_av_dependencies_unbound")
	}
	return nil
}

func UpdateAttributeViewColTemplateWithEnvironment(operation *Operation, env *AttributeViewEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	colType := av.KeyType(operation.Typ)
	switch colType {
	case av.KeyTypeTemplate:
		for _, keyValues := range attrView.KeyValues {
			if keyValues.Key.ID == operation.ID && av.KeyTypeTemplate == keyValues.Key.Type {
				keyValues.Key.Template = operation.Data.(string)
				break
			}
		}
	}

	if err = env.Groups(attrView); err != nil {
		return
	}
	err = env.Stage(attrView)
	return
}

func UpdateAttributeViewColRelationWithEnvironment(operation *Operation, env *AttributeViewEnvironment) (err error) {

	srcAv, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	destAv, err := env.Read(operation.ID)
	if err != nil {
		return
	}

	isSameAv := srcAv.ID == destAv.ID
	if isSameAv {
		destAv = srcAv
	}

	for _, keyValues := range srcAv.KeyValues {
		if keyValues.Key.ID != operation.KeyID {
			continue
		}

		srcRel := keyValues.Key.Relation

		if nil != srcRel {
			if srcRel.IsTwoWay {
				oldDestAv, readErr := env.Read(srcRel.AvID)
				if readErr != nil && (!env.Legacy || env.StrictDependencies) {
					return readErr
				}
				if nil != oldDestAv {
					isOldSameAv := oldDestAv.ID == destAv.ID
					if isOldSameAv {
						oldDestAv = destAv
					}

					oldDestKey, _ := oldDestAv.GetKey(srcRel.BackKeyID)
					if nil != oldDestKey && nil != oldDestKey.Relation && oldDestKey.Relation.AvID == srcAv.ID && oldDestKey.Relation.IsTwoWay {
						oldDestKey.Relation.IsTwoWay = false
						oldDestKey.Relation.BackKeyID = ""
					}

					if !isOldSameAv {
						err = env.Stage(oldDestAv)
						if err != nil {
							return
						}
					}
				}
			}

			if err = env.RemoveRelation(srcAv.ID, srcRel.AvID); err != nil {
				return
			}
		}

		srcRel = &av.Relation{
			AvID:     operation.ID,
			IsTwoWay: operation.IsTwoWay,
		}
		if operation.IsTwoWay {
			srcRel.BackKeyID = operation.BackRelationKeyID
		} else {
			srcRel.BackKeyID = ""
		}
		keyValues.Key.Relation = srcRel
		keyValues.Key.Name = operation.Format

		break
	}

	destAdded := false
	backRelKey, _ := destAv.GetKey(operation.BackRelationKeyID)
	if nil != backRelKey {
		backRelKey.Relation = &av.Relation{
			AvID:      operation.AvID,
			IsTwoWay:  operation.IsTwoWay,
			BackKeyID: operation.KeyID,
		}
		destAdded = true
		if operation.IsTwoWay {
			name := strings.TrimSpace(operation.Name)
			if "" == name {
				name = srcAv.Name + " " + operation.Format
			}
			backRelKey.Name = strings.TrimSpace(name)
		} else {
			backRelKey.Relation.BackKeyID = ""
		}
	}

	if !destAdded && operation.IsTwoWay {

		name := strings.TrimSpace(operation.Name)
		if "" == name {
			name = srcAv.Name + " " + operation.Format
			name = strings.TrimSpace(name)
		}

		destKeyValues := &av.KeyValues{
			Key: &av.Key{
				ID:       operation.BackRelationKeyID,
				Name:     name,
				Type:     av.KeyTypeRelation,
				Relation: &av.Relation{AvID: operation.AvID, IsTwoWay: operation.IsTwoWay, BackKeyID: operation.KeyID},
			},
		}
		destAv.KeyValues = append(destAv.KeyValues, destKeyValues)

		for _, v := range destAv.Views {
			switch v.LayoutType {
			case av.LayoutTypeTable:
				v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: operation.BackRelationKeyID}})
			case av.LayoutTypeGallery:
				v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: operation.BackRelationKeyID}})
			case av.LayoutTypeKanban:
				v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: operation.BackRelationKeyID}})
			}
		}

		now := env.Now().UnixMilli()

		for _, keyValues := range srcAv.KeyValues {
			if keyValues.Key.ID != operation.KeyID {
				continue
			}

			for _, srcVal := range keyValues.Values {
				for _, blockID := range srcVal.Relation.BlockIDs {
					destVal := destAv.GetValue(destKeyValues.Key.ID, blockID)
					if nil == destVal {
						destVal = &av.Value{ID: env.NewID(), KeyID: destKeyValues.Key.ID, BlockID: blockID, Type: keyValues.Key.Type, Relation: &av.ValueRelation{}, CreatedAt: now, UpdatedAt: now + 1000}
					} else {
						destVal.Type = keyValues.Key.Type
						if nil == destVal.Relation {
							destVal.Relation = &av.ValueRelation{}
						}
						destVal.UpdatedAt = now
						destVal.IsRenderAutoFill = false
					}
					destVal.Relation.BlockIDs = append(destVal.Relation.BlockIDs, srcVal.BlockID)
					destVal.Relation.BlockIDs = gulu.Str.RemoveDuplicatedElem(destVal.Relation.BlockIDs)
					destKeyValues.Values = append(destKeyValues.Values, destVal)
				}
			}
		}
	}

	if err = env.Groups(srcAv); err != nil {
		return
	}
	err = env.Stage(srcAv)
	if err != nil {
		return
	}
	if !isSameAv {
		if err = env.Groups(destAv); err != nil {
			return
		}
		err = env.Stage(destAv)
		if err != nil {
			return
		}
		if err = env.Reload(destAv.ID); err != nil {
			return
		}
	}

	if err = env.UpsertRelation(srcAv.ID, destAv.ID); err != nil {
		return
	}
	if operation.IsTwoWay && !isSameAv {
		if err = env.UpsertRelation(destAv.ID, srcAv.ID); err != nil {
			return
		}
	}
	return
}

func UpdateAttributeViewColRollupWithEnvironment(operation *Operation, env *AttributeViewEnvironment) (err error) {

	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	rollUpKey, _ := attrView.GetKey(operation.ID)
	if nil == rollUpKey {
		return
	}

	rollUpKey.Rollup = &av.Rollup{
		RelationKeyID: operation.ParentID,
		KeyID:         operation.KeyID,
	}

	if nil == operation.Data {
		return
	}

	data := operation.Data.(map[string]interface{})
	if nil != data["calc"] {
		calcData, jsonErr := gulu.JSON.MarshalJSON(data["calc"])
		if nil != jsonErr {
			err = jsonErr
			return
		}
		if jsonErr = gulu.JSON.UnmarshalJSON(calcData, &rollUpKey.Rollup.Calc); nil != jsonErr {
			err = jsonErr
			return
		}
	}

	for _, view := range attrView.Views {
		for i, filter := range view.Filters {
			if filter.Column != rollUpKey.ID {
				continue
			}

			view.Filters = append(view.Filters[:i], view.Filters[i+1:]...)
		}
	}

	err = env.Stage(attrView)
	return
}

func IsAttributeViewFieldAction(action string) bool {
	return action == "addAttrViewCol" || action == "updateAttrViewCol" || action == "removeAttrViewCol"
}

func (e *AttributeViewFieldEnvironment) Validate(action string) error {
	if e == nil || e.Read == nil || e.Stage == nil ||
		(action != "addAttrViewCol" && (e.Groups == nil || e.Sources == nil || e.Reload == nil)) ||
		(action == "removeAttrViewCol" && e.RemoveRelation == nil) {
		return errors.New("transaction_av_field_dependencies_unbound")
	}
	return nil
}

func ValidateAttributeViewFieldInput(op *Operation) error {
	if op == nil || !IsAttributeViewFieldAction(op.Action) {
		return errors.New("invalid_av_field_operation")
	}
	if op.Action == "addAttrViewCol" && op.Data != nil {
		if _, ok := op.Data.(string); !ok {
			return errors.New("invalid_av_field_icon")
		}
	}
	return nil
}

// Validate structural dereferences, not conversion policy: original type changes
// retain the old value payload and a newly added relation may be unconfigured.
func ValidateAttributeViewFieldSource(a *av.AttributeView) error {
	bad := errors.New("invalid_av_field_source")
	if a == nil {
		return bad
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return bad
		}
		for _, v := range kv.Values {
			if v == nil {
				return bad
			}
		}
	}
	for _, v := range a.Views {
		if v == nil {
			return bad
		}
		if (v.LayoutType == av.LayoutTypeTable && v.Table == nil) ||
			(v.LayoutType == av.LayoutTypeGallery && v.Gallery == nil) ||
			(v.LayoutType == av.LayoutTypeKanban && v.Kanban == nil) {
			return bad
		}
		if v.Table != nil {
			for _, f := range v.Table.Columns {
				if f == nil || f.BaseField == nil {
					return bad
				}
			}
		}
		if v.Gallery != nil {
			for _, f := range v.Gallery.CardFields {
				if f == nil || f.BaseField == nil {
					return bad
				}
			}
		}
		if v.Kanban != nil {
			for _, f := range v.Kanban.Fields {
				if f == nil || f.BaseField == nil {
					return bad
				}
			}
		}
	}
	return nil
}

func RunAttributeViewField(ctx context.Context, op *Operation, e *AttributeViewFieldEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateAttributeViewFieldInput(op); err != nil {
		return err
	}
	if err := e.Validate(op.Action); err != nil {
		return err
	}
	local := *e

	loaded := map[string]*av.AttributeView{}
	local.Read = func(id string) (*av.AttributeView, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if a := loaded[id]; a != nil {
			return a, nil
		}
		a, err := e.Read(id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = ValidateAttributeViewFieldSource(a); err != nil {
			return nil, err
		}
		a.RenderedViewables = map[string]av.Viewable{}
		loaded[id] = a
		return a, nil
	}
	wrap := func(fn func(*av.AttributeView) error) func(*av.AttributeView) error {
		return func(a *av.AttributeView) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(a); err != nil {
				return err
			}
			return ctx.Err()
		}
	}
	local.Stage = wrap(e.Stage)
	if e.Groups != nil {
		local.Groups = wrap(e.Groups)
	}
	local.Sources = func(id string) ([]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ids, err := e.Sources(id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	local.Reload = func(id string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.Reload(id); err != nil {
			return err
		}
		return ctx.Err()
	}
	local.RemoveRelation = func(a, b string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.RemoveRelation(a, b); err != nil {
			return err
		}
		return ctx.Err()
	}
	var err error
	switch op.Action {
	case "addAttrViewCol":
		icon, _ := op.Data.(string)
		err = AddAttributeViewKeyWithEnvironment(op.AvID, op.ID, op.Name, op.Typ, icon, op.PreviousID, &local)
	case "updateAttrViewCol":
		err = UpdateAttributeViewColumnWithEnvironment(op, &local)
	case "removeAttrViewCol":
		err = RemoveAttributeViewKeyWithEnvironment(op.AvID, op.ID, op.RemoveDest, &local)
	}
	if err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

// Original field algorithms; effects are supplied by the transaction owner.
func AddAttributeViewKeyWithEnvironment(avID, keyID, keyName, keyType, keyIcon, previousKeyID string, env *AttributeViewFieldEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	currentView, err := attrView.GetCurrentView(attrView.ViewID)
	if nil != err {
		return
	}

	keyTyp := av.KeyType(keyType)
	switch keyTyp {
	case av.KeyTypeText, av.KeyTypeNumber, av.KeyTypeDate, av.KeyTypeSelect, av.KeyTypeMSelect, av.KeyTypeURL, av.KeyTypeEmail,
		av.KeyTypePhone, av.KeyTypeMAsset, av.KeyTypeTemplate, av.KeyTypeCreated, av.KeyTypeUpdated, av.KeyTypeCheckbox,
		av.KeyTypeRelation, av.KeyTypeRollup, av.KeyTypeLineNumber:

		key := av.NewKey(keyID, keyName, keyIcon, keyTyp)
		if av.KeyTypeRollup == keyTyp {
			key.Rollup = &av.Rollup{Calc: &av.RollupCalc{Operator: av.CalcOperatorNone}}
		}

		attrView.KeyValues = append(attrView.KeyValues, &av.KeyValues{Key: key})

		for _, view := range attrView.Views {
			newField := &av.BaseField{ID: key.ID}
			if nil != view.Table {
				newField.Wrap = view.Table.WrapField

				if "" == previousKeyID {
					if av.LayoutTypeGallery == currentView.LayoutType || av.LayoutTypeKanban == currentView.LayoutType {

						view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: newField})
					} else {
						view.Table.Columns = append([]*av.ViewTableColumn{{BaseField: newField}}, view.Table.Columns...)
					}
				} else {
					added := false
					for i, column := range view.Table.Columns {
						if column.ID == previousKeyID {
							view.Table.Columns = append(view.Table.Columns[:i+1], append([]*av.ViewTableColumn{{BaseField: newField}}, view.Table.Columns[i+1:]...)...)
							added = true
							break
						}
					}
					if !added {
						view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: newField})
					}
				}
			}

			if nil != view.Gallery {
				newField.Wrap = view.Gallery.WrapField

				if "" == previousKeyID {
					view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: newField})
				} else {
					added := false
					for i, field := range view.Gallery.CardFields {
						if field.ID == previousKeyID {
							view.Gallery.CardFields = append(view.Gallery.CardFields[:i+1], append([]*av.ViewGalleryCardField{{BaseField: newField}}, view.Gallery.CardFields[i+1:]...)...)
							added = true
							break
						}
					}
					if !added {
						view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: newField})
					}
				}
			}

			if nil != view.Kanban {
				newField.Wrap = view.Kanban.WrapField

				if "" == previousKeyID {
					view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: newField})
				} else {
					added := false
					for i, field := range view.Kanban.Fields {
						if field.ID == previousKeyID {
							view.Kanban.Fields = append(view.Kanban.Fields[:i+1], append([]*av.ViewKanbanField{{BaseField: newField}}, view.Kanban.Fields[i+1:]...)...)
							added = true
							break
						}
					}
					if !added {
						view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: newField})
					}
				}
			}
		}
	}

	err = env.Stage(attrView)
	return
}

func UpdateAttributeViewColumnWithEnvironment(operation *Operation, env *AttributeViewFieldEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	colType := av.KeyType(operation.Typ)
	changeType := false
	switch colType {
	case av.KeyTypeBlock, av.KeyTypeText, av.KeyTypeNumber, av.KeyTypeDate, av.KeyTypeSelect, av.KeyTypeMSelect, av.KeyTypeURL, av.KeyTypeEmail,
		av.KeyTypePhone, av.KeyTypeMAsset, av.KeyTypeTemplate, av.KeyTypeCreated, av.KeyTypeUpdated, av.KeyTypeCheckbox,
		av.KeyTypeRelation, av.KeyTypeRollup, av.KeyTypeLineNumber:
		for _, keyValues := range attrView.KeyValues {
			if keyValues.Key.ID == operation.ID {
				keyValues.Key.Name = strings.TrimSpace(operation.Name)

				changeType = keyValues.Key.Type != colType
				keyValues.Key.Type = colType

				for _, value := range keyValues.Values {
					value.Type = colType
				}

				break
			}
		}
	}

	if changeType {
		for _, view := range attrView.Views {
			if nil != view.Group {
				if groupKey := view.GetGroupKey(attrView); nil != groupKey && groupKey.ID == operation.ID {
					RemoveAttributeViewGroup0(view)
				}
			}
		}
	}

	if err = env.Stage(attrView); nil != err {
		return
	}

	if changeType {
		relatedAvIDs, sourceErr := env.Sources(attrView.ID)
		if sourceErr != nil {
			return sourceErr
		}
		for _, relatedAvID := range relatedAvIDs {
			destAv, readErr := env.Read(relatedAvID)
			if readErr != nil && (!env.Legacy || env.StrictDependencies) {
				return readErr
			}
			if nil == destAv {
				continue
			}

			for _, keyValues := range destAv.KeyValues {
				if av.KeyTypeRollup == keyValues.Key.Type && keyValues.Key.Rollup == nil {
					return errors.New("invalid_av_field_rollup")
				}
				if av.KeyTypeRollup == keyValues.Key.Type && keyValues.Key.Rollup.KeyID == operation.ID {

					for _, val := range keyValues.Values {
						if val.Rollup == nil {
							return errors.New("invalid_av_field_rollup_value")
						}
						val.Rollup.Contents = nil
					}
					keyValues.Key.Rollup.Calc = &av.RollupCalc{Operator: av.CalcOperatorNone}
				}
			}

			if err = env.Groups(destAv); err != nil {
				return
			}
			if err = env.Stage(destAv); err != nil {
				if !env.Legacy || env.StrictDependencies {
					return
				}
				err = nil
			}
			if err = env.Reload(destAv.ID); err != nil {
				return
			}
		}
	}
	return
}

func RemoveAttributeViewKeyWithEnvironment(avID, keyID string, removeRelationDest bool, env *AttributeViewFieldEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	var removedKey *av.Key
	for i, keyValues := range attrView.KeyValues {
		if keyValues.Key.ID == keyID {
			attrView.KeyValues = append(attrView.KeyValues[:i], attrView.KeyValues[i+1:]...)
			removedKey = keyValues.Key
			break
		}
	}

	if nil != removedKey && av.KeyTypeRelation == removedKey.Type && nil != removedKey.Relation {
		if removedKey.Relation.IsTwoWay {
			var destAv *av.AttributeView
			if avID == removedKey.Relation.AvID {
				destAv = attrView
			} else {
				destAv, err = env.Read(removedKey.Relation.AvID)
				if err != nil && (!env.Legacy || env.StrictDependencies) {
					return
				}
				err = nil
			}

			if nil != destAv {
				oldDestKey, _ := destAv.GetKey(removedKey.Relation.BackKeyID)
				if nil != oldDestKey && nil != oldDestKey.Relation && oldDestKey.Relation.AvID == attrView.ID && oldDestKey.Relation.IsTwoWay {
					oldDestKey.Relation.IsTwoWay = false
					oldDestKey.Relation.BackKeyID = ""
				}

				destAvRelSrcAv := false
				for i := 0; i < len(destAv.KeyValues); i++ {
					keyValues := destAv.KeyValues[i]
					if keyValues.Key.ID == removedKey.Relation.BackKeyID {
						if removeRelationDest {
							destAv.KeyValues = append(destAv.KeyValues[:i], destAv.KeyValues[i+1:]...)
							i--
						} else if keyValues.Key.Type == av.KeyTypeRelation && keyValues.Key.Relation != nil && keyValues.Key.Relation.AvID == attrView.ID {

							destAvRelSrcAv = true
						}
						continue
					}

					if av.KeyTypeRelation == keyValues.Key.Type && keyValues.Key.Relation != nil && keyValues.Key.Relation.AvID == attrView.ID {
						destAvRelSrcAv = true
					}
				}

				if removeRelationDest {
					for _, view := range destAv.Views {
						if view.Group != nil && view.Group.Field == removedKey.Relation.BackKeyID {
							RemoveAttributeViewGroup0(view)
						}
						switch view.LayoutType {
						case av.LayoutTypeTable:
							for i, column := range view.Table.Columns {
								if column.ID == removedKey.Relation.BackKeyID {
									view.Table.Columns = append(view.Table.Columns[:i], view.Table.Columns[i+1:]...)
									break
								}
							}
						case av.LayoutTypeGallery:
							for i, field := range view.Gallery.CardFields {
								if field.ID == removedKey.Relation.BackKeyID {
									view.Gallery.CardFields = append(view.Gallery.CardFields[:i], view.Gallery.CardFields[i+1:]...)
									break
								}
							}
						case av.LayoutTypeKanban:
							for i, field := range view.Kanban.Fields {
								if field.ID == removedKey.Relation.BackKeyID {
									view.Kanban.Fields = append(view.Kanban.Fields[:i], view.Kanban.Fields[i+1:]...)
									break
								}
							}
						}
					}
				}

				if destAv != attrView {
					if err = env.Stage(destAv); err != nil {
						if !env.Legacy || env.StrictDependencies {
							return
						}
						err = nil
					}
					if err = env.Reload(destAv.ID); err != nil {
						return
					}
				}

				if !destAvRelSrcAv {
					if err = env.RemoveRelation(destAv.ID, attrView.ID); err != nil {
						return
					}
				}
			}

		}
		srcAvRelDestAv := false
		for _, keyValues := range attrView.KeyValues {
			if av.KeyTypeRelation == keyValues.Key.Type && nil != keyValues.Key.Relation && keyValues.Key.Relation.AvID == removedKey.Relation.AvID {
				srcAvRelDestAv = true
			}
		}
		if !srcAvRelDestAv {
			if err = env.RemoveRelation(attrView.ID, removedKey.Relation.AvID); err != nil {
				return
			}
		}
	}

	for _, view := range attrView.Views {
		if nil != view.Table {
			for i, column := range view.Table.Columns {
				if column.ID == keyID {
					view.Table.Columns = append(view.Table.Columns[:i], view.Table.Columns[i+1:]...)
					break
				}
			}
		}

		if nil != view.Gallery {
			for i, field := range view.Gallery.CardFields {
				if field.ID == keyID {
					view.Gallery.CardFields = append(view.Gallery.CardFields[:i], view.Gallery.CardFields[i+1:]...)
					break
				}
			}
		}

		if nil != view.Kanban {
			for i, field := range view.Kanban.Fields {
				if field.ID == keyID {
					view.Kanban.Fields = append(view.Kanban.Fields[:i], view.Kanban.Fields[i+1:]...)
					break
				}
			}
		}
	}

	for _, view := range attrView.Views {
		if nil != view.Group {
			if view.Group.Field == keyID {
				RemoveAttributeViewGroup0(view)
			}
		}
	}

	if err = env.Stage(attrView); nil != err {
		return
	}

	relatedAvIDs, sourceErr := env.Sources(avID)
	if sourceErr != nil {
		return sourceErr
	}
	for _, relatedAvID := range relatedAvIDs {
		destAv, readErr := env.Read(relatedAvID)
		if readErr != nil && (!env.Legacy || env.StrictDependencies) {
			return readErr
		}
		if nil == destAv {
			continue
		}

		for _, keyValues := range destAv.KeyValues {
			if av.KeyTypeRollup == keyValues.Key.Type && keyValues.Key.Rollup == nil {
				return errors.New("invalid_av_field_rollup")
			}
			if av.KeyTypeRollup == keyValues.Key.Type && keyValues.Key.Rollup.KeyID == keyID {

				for _, val := range keyValues.Values {
					if val.Rollup == nil {
						return errors.New("invalid_av_field_rollup_value")
					}
					val.Rollup.Contents = nil
				}
			}
		}

		if err = env.Groups(destAv); err != nil {
			return
		}
		if err = env.Stage(destAv); err != nil {
			if !env.Legacy || env.StrictDependencies {
				return
			}
			err = nil
		}
		if err = env.Reload(destAv.ID); err != nil {
			return
		}
	}
	return
}

func IsAttributeViewFieldSettingAction(action string) bool {
	return AttributeViewFieldSettingOperations[action] != nil
}

func AttributeViewFieldSettingNeedsView(action string) bool {
	switch action {
	case "setAttrViewColWidth", "setAttrViewColWrap", "setAttrViewColHidden", "setAttrViewColPin", "setAttrViewColCalc":
		return true
	}
	return false
}

func ValidateAttributeViewFieldSettingInput(op *Operation) error {
	if op == nil || !IsAttributeViewFieldSettingAction(op.Action) {
		return errors.New("invalid_av_field_setting_operation")
	}
	switch op.Action {
	case "setAttrViewColWidth", "setAttrViewColIcon", "setAttrViewColDesc":
		if _, ok := op.Data.(string); !ok {
			return errors.New("invalid_av_field_setting_string")
		}
	case "setAttrViewColWrap", "setAttrViewColHidden", "setAttrViewColPin", "setAttrViewColDateFillCreated", "setAttrViewColDateFillSpecificTime", "setAttrViewCreatedIncludeTime", "setAttrViewUpdatedIncludeTime":
		if _, ok := op.Data.(bool); !ok {
			return errors.New("invalid_av_field_setting_bool")
		}
	case "setAttrViewColCalc":

		if op.Data == nil {
			return errors.New("invalid_av_field_setting_calc")
		}
	}
	return nil
}

func RunAttributeViewFieldSetting(ctx context.Context, op *Operation, e *AttributeViewSettingsEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateAttributeViewFieldSettingInput(op); err != nil {
		return err
	}
	if e == nil || e.Read == nil || e.Stage == nil || (AttributeViewFieldSettingNeedsView(op.Action) && e.View == nil) {
		return errors.New("transaction_av_field_setting_dependencies_unbound")
	}
	local := *e
	local.Read = func(id string) (*av.AttributeView, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := e.Read(id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = ValidateAttributeViewFieldSource(a); err != nil {
			return nil, err
		}
		a.RenderedViewables = map[string]av.Viewable{}
		return a, nil
	}
	local.View = func(a *av.AttributeView, id string) (*av.View, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, err := e.View(a, id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if v == nil {
			return nil, errors.New("invalid_av_field_setting_view")
		}
		return v, nil
	}
	local.Stage = func(a *av.AttributeView) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.Stage(a); err != nil {
			return err
		}
		return ctx.Err()
	}
	if err := AttributeViewFieldSettingOperations[op.Action](op, &local); err != nil {
		return err
	}
	return ctx.Err()
}

func SetAttributeViewColWidthWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		for _, column := range view.Table.Columns {
			if column.ID == operation.ID {
				column.Width = operation.Data.(string)
				break
			}
		}
	case av.LayoutTypeGallery, av.LayoutTypeKanban:
		return
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColWrapWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	newWrap := operation.Data.(bool)
	allFieldWrap := true
	switch view.LayoutType {
	case av.LayoutTypeTable:
		for _, column := range view.Table.Columns {
			if column.ID == operation.ID {
				column.Wrap = newWrap
			}
			allFieldWrap = allFieldWrap && column.Wrap
		}
		view.Table.WrapField = allFieldWrap
	case av.LayoutTypeGallery:
		for _, field := range view.Gallery.CardFields {
			if field.ID == operation.ID {
				field.Wrap = newWrap
			}
			allFieldWrap = allFieldWrap && field.Wrap
		}
		view.Gallery.WrapField = allFieldWrap
	case av.LayoutTypeKanban:
		for _, field := range view.Kanban.Fields {
			if field.ID == operation.ID {
				field.Wrap = newWrap
			}
			allFieldWrap = allFieldWrap && field.Wrap
		}
		view.Kanban.WrapField = allFieldWrap
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColHiddenWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		for _, column := range view.Table.Columns {
			if column.ID == operation.ID {
				column.Hidden = operation.Data.(bool)
				break
			}
		}
	case av.LayoutTypeGallery:
		for _, field := range view.Gallery.CardFields {
			if field.ID == operation.ID {
				field.Hidden = operation.Data.(bool)
				break
			}
		}
	case av.LayoutTypeKanban:
		for _, field := range view.Kanban.Fields {
			if field.ID == operation.ID {
				field.Hidden = operation.Data.(bool)
				break
			}
		}
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColPinWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		for _, column := range view.Table.Columns {
			if column.ID == operation.ID {
				column.Pin = operation.Data.(bool)
				break
			}
		}
	case av.LayoutTypeGallery, av.LayoutTypeKanban:
		return
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColIconWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	for _, keyValues := range attrView.KeyValues {
		if keyValues.Key.ID == operation.ID {
			keyValues.Key.Icon = operation.Data.(string)
			break
		}
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColDescWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	for _, keyValues := range attrView.KeyValues {
		if keyValues.Key.ID == operation.ID {
			keyValues.Key.Desc = operation.Data.(string)
			break
		}
	}

	err = env.Stage(attrView)
	return
}

func UpdateAttributeViewColNumberFormatWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	colType := av.KeyType(operation.Typ)
	switch colType {
	case av.KeyTypeNumber:
		for _, keyValues := range attrView.KeyValues {
			if keyValues.Key.ID == operation.ID && av.KeyTypeNumber == keyValues.Key.Type {
				keyValues.Key.NumberFormat = av.NumberFormat(operation.Format)
				break
			}
		}
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColumnCalcWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	operationData := operation.Data.(interface{})
	data, err := gulu.JSON.MarshalJSON(operationData)
	if err != nil {
		return
	}

	calc := &av.FieldCalc{}
	switch view.LayoutType {
	case av.LayoutTypeTable:
		if err = gulu.JSON.UnmarshalJSON(data, calc); err != nil {
			return
		}

		for _, column := range view.Table.Columns {
			if column.ID == operation.ID {
				column.Calc = calc
				break
			}
		}
	case av.LayoutTypeGallery, av.LayoutTypeKanban:
		return
	}

	err = env.Stage(attrView)
	return
}

func SetAttributeViewColDateFillCreatedWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	keyID := operation.ID
	key, _ := attrView.GetKey(keyID)
	if nil == key || av.KeyTypeDate != key.Type {
		return
	}

	if nil == key.Date {
		key.Date = &av.Date{}
	}

	key.Date.AutoFillNow = operation.Data.(bool)
	err = env.Stage(attrView)
	return
}

func SetAttrViewColDateFillSpecificTimeWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	keyID := operation.ID
	dateValues, _ := attrView.GetKeyValues(keyID)
	if nil == dateValues || av.KeyTypeDate != dateValues.Key.Type {
		return
	}

	if nil == dateValues.Key.Date {
		dateValues.Key.Date = &av.Date{}
	}

	dateValues.Key.Date.FillSpecificTime = operation.Data.(bool)
	for _, v := range dateValues.Values {
		if !v.IsEmpty() {
			continue
		}
		if nil == v.Date {
			v.Date = &av.ValueDate{}
		}
		v.Date.IsNotTime = !dateValues.Key.Date.FillSpecificTime
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewCreatedIncludeTimeWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	key, _ := attrView.GetKey(operation.ID)
	if nil == key {
		return
	}

	if nil == key.Created {
		key.Created = &av.Created{}
	}

	key.Created.IncludeTime = operation.Data.(bool)
	err = env.Stage(attrView)
	return
}

func SetAttrViewUpdatedIncludeTimeWithEnvironment(operation *Operation, env *AttributeViewSettingsEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	key, _ := attrView.GetKey(operation.ID)
	if nil == key {
		return
	}

	if nil == key.Updated {
		key.Updated = &av.Updated{}
	}

	key.Updated.IncludeTime = operation.Data.(bool)
	err = env.Stage(attrView)
	return
}

func IsAttributeViewManagementAction(action string) bool {
	for _, a := range AttributeViewManagementActions {
		if a == action {
			return true
		}
	}
	return false
}

func ValidateAttributeViewManagementInput(op *Operation) error {
	if op == nil || !IsAttributeViewManagementAction(op.Action) || op.AvID == "" {
		return errors.New("invalid_av_management_operation")
	}
	bad := errors.New("invalid_av_management_data")
	switch op.Action {
	case "setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc":
		if _, ok := op.Data.(string); !ok {
			return bad
		}
	case "hideAttrViewName", "hideAttrViewAllGroups", "foldAttrViewGroup", "setAttrViewWrapField", "setAttrViewShowIcon", "setAttrViewFitImage", "setAttrViewDisplayFieldName", "setAttrViewFillColBackgroundColor":
		if _, ok := op.Data.(bool); !ok {
			return bad
		}
	case "hideAttrViewGroup", "setAttrViewCardAspectRatio", "setAttrViewCardSize", "setAttrViewCoverFrom":
		n, ok := op.Data.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < -math.Exp2(63) || n >= math.Exp2(63) {
			return bad
		}
	case "setAttrViewGroup":
		if op.Data == nil {
			return bad
		}
		switch v := op.Data.(type) {
		case *av.ViewGroup:
			if v == nil {
				return bad
			}
		case map[string]interface{}:
		default:
			return bad
		}
	case "changeAttrViewLayout":
		if !ValidAttributeViewManagementLayout(op.Layout) {
			return av.ErrWrongLayoutType
		}
	case "addAttrViewView":
		if op.ID == "" || (op.Layout != "" && !ValidAttributeViewManagementLayout(op.Layout)) {
			return bad
		}
	case "duplicateAttrViewView":
		if op.ID == "" || op.ID == op.PreviousID {
			return bad
		}
	}
	return nil
}

func ValidAttributeViewManagementLayout(t av.LayoutType) bool {
	return t == av.LayoutTypeTable || t == av.LayoutTypeGallery || t == av.LayoutTypeKanban
}

func (e *AttributeViewManagementEnvironment) Validate(action string) error {
	if e == nil || e.Ctx == nil || e.Read == nil || e.Stage == nil || e.View == nil {
		return errors.New("transaction_av_management_dependencies_unbound")
	}
	structural := action == "addAttrViewView" || action == "duplicateAttrViewView" || action == "changeAttrViewLayout"
	if structural && (e.MakeView == nil || e.Language == nil || e.Groups == nil || e.Count == nil || e.Node == nil || e.Attrs == nil) {
		return errors.New("transaction_av_management_structure_unbound")
	}
	if action == "setAttrViewBlockView" && (e.Node == nil || e.Attrs == nil) {
		return errors.New("transaction_av_management_block_unbound")
	}
	if action == "duplicateAttrViewView" && e.CloneView == nil {
		return errors.New("transaction_av_management_clone_unbound")
	}
	if action == "removeAttrViewView" && (e.Mirrors == nil || e.Node == nil || e.MirrorAttrs == nil || e.FlushTrees == nil) {
		return errors.New("transaction_av_management_mirrors_unbound")
	}
	if action == "changeAttrViewLayout" && e.Mirrors == nil {
		return errors.New("transaction_av_management_mirrors_unbound")
	}
	if action == "setAttrViewGroup" && (e.Groups == nil || e.Count == nil) {
		return errors.New("transaction_av_management_group_unbound")
	}
	if action == "sortAttrViewGroup" && e.SortGroups == nil {
		return errors.New("transaction_av_management_sort_unbound")
	}
	if (action == "setAttrViewGroup" || action == "changeAttrViewLayout" || action == "syncAttrViewTableColWidth") && e.Reload == nil {
		return errors.New("transaction_av_management_reload_unbound")
	}
	return e.Ctx.Err()
}

func AttributeViewManagementError(err error) error {
	if err != nil {
		return err
	}
	return av.ErrViewNotFound
}

func CloneAttributeViewManagementValue(v *av.Value) *av.Value {
	if v == nil {
		return nil
	}
	return v.Clone()
}

func RunAttributeViewManagement(op *Operation, e *AttributeViewManagementEnvironment) error {
	if err := ValidateAttributeViewManagementInput(op); err != nil {
		return err
	}
	if err := e.Validate(op.Action); err != nil {
		return err
	}
	local := *e
	ctx := e.Ctx
	guard := func(fn func() error) error {
		if e.OwnerGuard != nil {
			if err := e.OwnerGuard(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		if e.OwnerGuard != nil {
			return e.OwnerGuard()
		}
		return ctx.Err()
	}
	local.Read = func(id string) (a *av.AttributeView, err error) {
		err = guard(func() error { a, err = e.Read(id); return err })
		if err != nil {
			return
		}
		if err = ValidateAttributeViewFieldSource(a); err != nil {
			return
		}
		if err = ValidateCellSource(a); err != nil {
			return
		}
		if a.ID != id || len(a.Views) == 0 || a.GetBlockKeyValues() == nil {
			return nil, av.ErrViewNotFound
		}
		for _, v := range a.Views {
			if !ValidAttributeViewManagementLayout(v.LayoutType) {
				return nil, av.ErrWrongLayoutType
			}
			if (v.Table != nil && v.Table.BaseLayout == nil) || (v.Gallery != nil && v.Gallery.BaseLayout == nil) || (v.Kanban != nil && v.Kanban.BaseLayout == nil) {
				return nil, ErrInvalidAVOrderSource
			}
			for _, g := range v.Groups {
				if g == nil {
					return nil, ErrInvalidAVOrderSource
				}
			}
			for _, f := range v.Filters {
				if f == nil || !ValidAttributeViewDefaultFilter(f) {
					return nil, ErrInvalidAVOrderSource
				}
			}
			for _, s := range v.Sorts {
				if s == nil {
					return nil, ErrInvalidAVOrderSource
				}
			}
		}
		a.RenderedViewables = map[string]av.Viewable{}
		return
	}
	local.View = func(a *av.AttributeView, id string) (v *av.View, err error) {
		err = guard(func() error { v, err = e.View(a, id); return err })
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, av.ErrViewNotFound
		}
		return
	}
	local.SortGroups = func(a *av.AttributeView, v *av.View) error { return guard(func() error { return e.SortGroups(a, v) }) }
	local.Stage = func(a *av.AttributeView) error { return guard(func() error { return e.Stage(a) }) }
	local.Groups = func(a *av.AttributeView) error { return guard(func() error { return e.Groups(a) }) }
	local.Count = func(a *av.AttributeView, v, g *av.View) (n int, err error) {
		err = guard(func() error { n, err = e.Count(a, v, g); return err })
		return
	}
	local.Node = func(id string) (n *ast.Node, t *parse.Tree, err error) {
		err = guard(func() error { n, t, err = e.Node(id); return err })
		return
	}
	local.Attrs = func(n *ast.Node, t *parse.Tree, a map[string]string) error {
		return guard(func() error { return e.Attrs(n, t, a) })
	}
	local.MirrorAttrs = func(n *ast.Node, t *parse.Tree, a map[string]string) error {
		return guard(func() error { return e.MirrorAttrs(n, t, a) })
	}
	local.FlushTrees = func(t []*parse.Tree) error { return guard(func() error { return e.FlushTrees(t) }) }
	local.Reload = func(id string) error { return guard(func() error { return e.Reload(id) }) }
	local.Mirrors = func(id string) (ids []string, err error) {
		err = guard(func() error { ids, err = e.Mirrors(id); return err })
		return
	}
	local.CloneView = func(v *av.View) (ret *av.View, err error) {
		err = guard(func() error { ret, err = e.CloneView(v); return err })
		return
	}
	local.MakeView = func(t av.LayoutType) *av.View {
		if ctx.Err() != nil {
			return nil
		}
		v := e.MakeView(t)
		if ctx.Err() != nil || v == nil || v.LayoutType != t {
			return nil
		}
		if (t == av.LayoutTypeTable && (v.Table == nil || v.Table.BaseLayout == nil)) || (t == av.LayoutTypeGallery && (v.Gallery == nil || v.Gallery.BaseLayout == nil)) || (t == av.LayoutTypeKanban && (v.Kanban == nil || v.Kanban.BaseLayout == nil)) {
			return nil
		}
		return v
	}
	if op.Action == "addAttrViewView" || op.Action == "duplicateAttrViewView" {
		a, err := local.Read(op.AvID)
		if err != nil {
			return err
		}
		if a.GetView(op.ID) != nil {
			return errors.New("av_view_id_exists")
		}
	}
	var err error
	switch op.Action {
	case "sortAttrViewGroup":
		err = SortAttributeViewGroupWithEnvironment(op.AvID, op.BlockID, op.PreviousID, op.ID, &local)
	case "removeAttrViewGroup":
		err = RemoveAttributeViewGroupWithEnvironment(op.AvID, op.BlockID, &local)
	case "syncAttrViewTableColWidth":
		err = SyncAttrViewTableColWidthWithEnvironment(op, &local)
	case "hideAttrViewGroup":
		err = HideAttributeViewGroupWithEnvironment(op.AvID, op.BlockID, op.ID, int(op.Data.(float64)), &local)
	case "hideAttrViewAllGroups":
		err = HideAttributeViewAllGroupsWithEnvironment(op.AvID, op.BlockID, op.Data.(bool), &local)
	case "foldAttrViewGroup":
		err = FoldAttrViewGroupWithEnvironment(op.AvID, op.BlockID, op.ID, op.Data.(bool), &local)
	case "setAttrViewGroup":
		raw, encodeErr := gulu.JSON.MarshalJSON(op.Data)
		if encodeErr != nil {
			return encodeErr
		}
		var group *av.ViewGroup
		if err = gulu.JSON.UnmarshalJSON(raw, &group); err != nil {
			return err
		}
		err = ConfigureAttributeViewGroupWithEnvironment(op.AvID, op.BlockID, group, &local)
	case "setAttrViewCardAspectRatio":
		err = SetAttrViewCardAspectRatioWithEnvironment(op, &local)
	case "setAttrViewBlockView":
		err = SetDatabaseBlockViewWithEnvironment(op.BlockID, op.AvID, op.ID, &local)
	case "changeAttrViewLayout":
		err = ChangeAttrViewLayoutWithEnvironment(op.BlockID, op.AvID, op.Layout, &local)
	case "setAttrViewWrapField":
		err = SetAttrViewWrapFieldWithEnvironment(op, &local)
	case "setAttrViewShowIcon":
		err = SetAttrViewShowIconWithEnvironment(op, &local)
	case "setAttrViewFitImage":
		err = SetAttrViewFitImageWithEnvironment(op, &local)
	case "setAttrViewDisplayFieldName":
		err = SetAttrViewDisplayFieldNameWithEnvironment(op, &local)
	case "setAttrViewFillColBackgroundColor":
		err = SetAttrViewFillColBackgroundColorWithEnvironment(op, &local)
	case "setAttrViewCardSize":
		err = SetAttrViewCardSizeWithEnvironment(op, &local)
	case "setAttrViewCoverFromAssetKeyID":
		err = SetAttrViewCoverFromAssetKeyIDWithEnvironment(op, &local)
	case "setAttrViewCoverFrom":
		err = SetAttrViewCoverFromWithEnvironment(op, &local)
	case "hideAttrViewName":
		err = HideAttrViewNameWithEnvironment(op, &local)
	case "addAttrViewView":
		err = AddAttrViewViewWithEnvironment(op.AvID, op.ID, op.BlockID, op.Layout, &local)
	case "sortAttrViewView":
		err = DoSortAttrViewViewWithEnvironment(op, &local)
	case "removeAttrViewView":
		err = DoRemoveAttrViewViewWithEnvironment(op, &local)
	case "duplicateAttrViewView":
		err = DoDuplicateAttrViewViewWithEnvironment(op, &local)
	case "setAttrViewViewName":
		err = DoSetAttrViewViewNameWithEnvironment(op, &local)
	case "setAttrViewViewIcon":
		err = DoSetAttrViewViewIconWithEnvironment(op, &local)
	case "setAttrViewViewDesc":
		err = DoSetAttrViewViewDescWithEnvironment(op, &local)
	}
	if err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

func UpdateAttributeViewManagementMirrors(e *AttributeViewManagementEnvironment, a *av.AttributeView, old string, v *av.View) error {
	ids, err := e.Mirrors(a.ID)
	if err != nil {
		return err
	}
	trees := []*parse.Tree{}
	owned := map[string]*parse.Tree{}
	for _, id := range ids {
		n, t, err := e.Node(id)
		if err != nil || n == nil || t == nil {
			if e.Legacy && !e.StrictDependencies {
				continue
			}
			if err != nil {
				return err
			}
			return ErrBlockNotFound
		}
		if existing := owned[t.ID]; existing != nil {
			t = existing
			n = treenode.GetNodeInTree(t, id)
			if n == nil {
				if e.Legacy && !e.StrictDependencies {
					continue
				}
				return ErrBlockNotFound
			}
		} else {
			owned[t.ID] = t
			trees = append(trees, t)
		}
		attrs := parse.IAL2Map(n.KramdownIAL)
		if attrs[av.NodeAttrView] != old {
			continue
		}
		attrs[av.NodeAttrView] = v.ID
		n.AttributeViewType = string(v.LayoutType)
		if err = e.MirrorAttrs(n, t, attrs); err != nil {
			if e.Legacy && !e.StrictDependencies {
				continue
			}
			return err
		}
	}
	return e.FlushTrees(trees)
}

func SortAttributeViewGroupWithEnvironment(avID, blockID, previousGroupID, groupID string, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return err
	}

	if view.Group == nil {
		return av.ErrViewNotFound
	}
	if err = env.SortGroups(attrView, view); err != nil {
		return err
	}

	var groupView *av.View
	var index, previousIndex int
	for i, g := range view.Groups {
		if g.ID == groupID {
			groupView = g
			index = i
			break
		}
	}
	if nil == groupView {
		return
	}
	view.Group.Order = av.GroupOrderMan

	view.Groups = append(view.Groups[:index], view.Groups[index+1:]...)
	for i, g := range view.Groups {
		if g.ID == previousGroupID {
			previousIndex = i + 1
			break
		}
	}
	view.Groups = util.InsertElem(view.Groups, previousIndex, groupView)

	for i, g := range view.Groups {
		g.GroupSort = i
	}

	err = env.Stage(attrView)
	return
}

func RemoveAttributeViewGroupWithEnvironment(avID, blockID string, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return err
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return err
	}

	RemoveAttributeViewGroup0(view)
	err = env.Stage(attrView)
	if err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return err
	}
	return nil
}

func SyncAttrViewTableColWidthWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view := attrView.GetView(operation.ID)
	if nil == view {
		err = av.ErrViewNotFound
		env.
			Report("error", "view [%s] not found in attribute view [%s]", operation.ID, operation.AvID)
		return
	}

	var width string
	switch view.LayoutType {
	case av.LayoutTypeTable:
		for _, column := range view.Table.Columns {
			if column.ID == operation.KeyID {
				width = column.Width
				break
			}
		}
	case av.LayoutTypeGallery, av.LayoutTypeKanban:
		return
	}

	for _, v := range attrView.Views {
		if av.LayoutTypeTable == v.LayoutType {
			for _, column := range v.Table.Columns {
				if column.ID == operation.KeyID {
					column.Width = width
					break
				}
			}
		}
	}

	err = env.Stage(attrView)
	if err == nil {
		err = env.Reload(attrView.ID)
	}
	return
}

func HideAttributeViewGroupWithEnvironment(avID, blockID, groupID string, hidden int, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return
	}

	for _, group := range view.Groups {
		if group.ID == groupID {
			group.GroupHidden = hidden
			break
		}
	}

	err = env.Stage(attrView)
	if err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return
	}
	return
}

func HideAttributeViewAllGroupsWithEnvironment(avID, blockID string, hidden bool, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return
	}

	for _, group := range view.Groups {
		if hidden {
			group.GroupHidden = 2
		} else {
			group.GroupHidden = 0
		}
	}

	err = env.Stage(attrView)
	if err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return
	}
	return
}

func FoldAttrViewGroupWithEnvironment(avID, blockID, groupID string, folded bool, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return err
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return err
	}

	if !view.IsGroupView() {
		return
	}

	for _, group := range view.Groups {
		if group.ID == groupID {
			group.GroupFolded = folded
			break
		}
	}

	err = env.Stage(attrView)
	if err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return err
	}
	return nil
}

func ConfigureAttributeViewGroupWithEnvironment(avID, blockID string, group *av.ViewGroup, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return err
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return err
	}

	if err = SetAttributeViewGroupWithEnvironment(attrView, view, group, env); err != nil {
		return err
	}

	err = env.Stage(attrView)
	if err == nil {
		err = env.Reload(avID)
	}
	return
}

func SetAttrViewCardAspectRatioWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.CardAspectRatio = av.CardAspectRatio(operation.Data.(float64))
	case av.LayoutTypeKanban:
		view.Kanban.CardAspectRatio = av.CardAspectRatio(operation.Data.(float64))
	}

	err = env.Stage(attrView)
	return
}

func ChangeAttrViewLayoutWithEnvironment(blockID, avID string, newLayout av.LayoutType, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return
	}

	if newLayout == view.LayoutType {
		return
	}

	fresh := env.MakeView(newLayout)
	if fresh == nil {
		return av.ErrWrongLayoutType
	}
	oldLayout := view.LayoutType
	view.LayoutType = newLayout

	switch newLayout {
	case av.LayoutTypeTable:
		if view.Name == env.Language("gallery") || view.Name == env.Language("kanban") {
			view.Name = env.Language("table")
		}

		if nil != view.Table {
			break
		}

		view.Table = fresh.Table
		switch oldLayout {
		case av.LayoutTypeGallery:
			for _, field := range view.Gallery.CardFields {
				view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: field.ID}})
			}
		case av.LayoutTypeKanban:
			for _, field := range view.Kanban.Fields {
				view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: field.ID}})
			}
		}
	case av.LayoutTypeGallery:
		if view.Name == env.Language("table") || view.Name == env.Language("kanban") {
			view.Name = env.Language("gallery")
		}

		if nil != view.Gallery {
			break
		}

		view.Gallery = fresh.Gallery
		switch oldLayout {
		case av.LayoutTypeTable:
			for _, col := range view.Table.Columns {
				view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: col.ID}})
			}
		case av.LayoutTypeKanban:
			for _, field := range view.Kanban.Fields {
				view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: field.ID}})
			}
		}
	case av.LayoutTypeKanban:
		if view.Name == env.Language("table") || view.Name == env.Language("gallery") {
			view.Name = env.Language("kanban")
		}

		if nil != view.Kanban {
			break
		}

		view.Kanban = fresh.Kanban
		switch oldLayout {
		case av.LayoutTypeTable:
			for _, col := range view.Table.Columns {
				view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: col.ID}})
			}
		case av.LayoutTypeGallery:
			for _, field := range view.Gallery.CardFields {
				view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: field.ID}})
			}
		}

		if !view.IsGroupView() {
			preferredGroupKey := GetKanbanPreferredGroupKey(attrView)
			group := &av.ViewGroup{Field: preferredGroupKey.ID}
			if err = SetAttributeViewGroupWithEnvironment(attrView, view, group, env); err != nil {
				return err
			}
		}
	}

	blockIDs, mirrorErr := env.Mirrors(avID)
	if mirrorErr != nil {
		return mirrorErr
	}
	for _, bID := range blockIDs {
		node, tree, nodeErr := env.Node(bID)
		if nodeErr != nil && (!env.Legacy || env.StrictDependencies) {
			return nodeErr
		}
		if nil == node || nil == tree {
			if !env.Legacy || env.StrictDependencies {
				return ErrBlockNotFound
			}
			continue
		}

		changed := false
		attrs := parse.IAL2Map(node.KramdownIAL)
		if blockID == bID {
			attrs[av.NodeAttrView] = view.ID
			node.AttributeViewType = string(view.LayoutType)
			attrView.ViewID = view.ID
			changed = true
		} else {
			if view.ID == attrs[av.NodeAttrView] {

				node.AttributeViewType = string(view.LayoutType)
				changed = true
			}
		}

		if changed {
			err = env.Attrs(node, tree, attrs)
			if err != nil {
				env.
					Report("warn", "set node [%s] attrs failed: %s", bID, err)
				return
			}
		}
	}

	if err = env.Groups(attrView); err != nil {
		return err
	}

	if err = env.Stage(attrView); nil != err {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return
	}

	if err == nil {
		err = env.Reload(avID)
	}
	return
}

func SetAttrViewWrapFieldWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	allFieldWrap := operation.Data.(bool)
	switch view.LayoutType {
	case av.LayoutTypeTable:
		view.Table.WrapField = allFieldWrap
		for _, col := range view.Table.Columns {
			col.Wrap = allFieldWrap
		}
	case av.LayoutTypeGallery:
		view.Gallery.WrapField = allFieldWrap
		for _, field := range view.Gallery.CardFields {
			field.Wrap = allFieldWrap
		}
	case av.LayoutTypeKanban:
		view.Kanban.WrapField = allFieldWrap
		for _, field := range view.Kanban.Fields {
			field.Wrap = allFieldWrap
		}
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewShowIconWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		view.Table.ShowIcon = operation.Data.(bool)
	case av.LayoutTypeGallery:
		view.Gallery.ShowIcon = operation.Data.(bool)
	case av.LayoutTypeKanban:
		view.Kanban.ShowIcon = operation.Data.(bool)
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewFitImageWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.FitImage = operation.Data.(bool)
	case av.LayoutTypeKanban:
		view.Kanban.FitImage = operation.Data.(bool)
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewDisplayFieldNameWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.DisplayFieldName = operation.Data.(bool)
	case av.LayoutTypeKanban:
		view.Kanban.DisplayFieldName = operation.Data.(bool)
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewFillColBackgroundColorWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		return
	case av.LayoutTypeKanban:
		view.Kanban.FillColBackgroundColor = operation.Data.(bool)
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewCardSizeWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.CardSize = av.CardSize(operation.Data.(float64))
	case av.LayoutTypeKanban:
		view.Kanban.CardSize = av.CardSize(operation.Data.(float64))
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewCoverFromAssetKeyIDWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.CoverFromAssetKeyID = operation.KeyID
	case av.LayoutTypeKanban:
		view.Kanban.CoverFromAssetKeyID = operation.KeyID
	}

	err = env.Stage(attrView)
	return
}

func SetAttrViewCoverFromWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	switch view.LayoutType {
	case av.LayoutTypeTable:
		return
	case av.LayoutTypeGallery:
		view.Gallery.CoverFrom = av.CoverFrom(operation.Data.(float64))
	case av.LayoutTypeKanban:
		view.Kanban.CoverFrom = av.CoverFrom(operation.Data.(float64))
	}

	err = env.Stage(attrView)
	return
}

func HideAttrViewNameWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", operation.AvID, err)
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if nil == view {
		env.
			Report("error", "get view [%s] failed: %s", operation.BlockID, err)
		return
	}

	view.HideAttrViewName = operation.Data.(bool)
	err = env.Stage(attrView)
	return
}

func AddAttrViewViewWithEnvironment(avID, viewID, blockID string, layout av.LayoutType, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return
	}

	if 1 > len(attrView.Views) {
		return av.ErrViewNotFound
	}

	firstView := attrView.Views[0]
	if nil == firstView {
		env.
			Report("error", "get first view failed: %s", avID)
		return
	}

	if "" == layout {
		layout = av.LayoutTypeTable
	}

	var view *av.View
	switch layout {
	case av.LayoutTypeTable:
		view = env.MakeView(av.LayoutTypeTable)
		if view == nil {
			return av.ErrWrongLayoutType
		}
		switch firstView.LayoutType {
		case av.LayoutTypeTable:
			for _, col := range firstView.Table.Columns {
				view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: col.ID}, Width: col.Width})
			}
		case av.LayoutTypeGallery:
			for _, field := range firstView.Gallery.CardFields {
				view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: field.ID}})
			}
		case av.LayoutTypeKanban:
			for _, field := range firstView.Kanban.Fields {
				view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: field.ID}})
			}
		}
	case av.LayoutTypeGallery:
		view = env.MakeView(av.LayoutTypeGallery)
		if view == nil {
			return av.ErrWrongLayoutType
		}
		switch firstView.LayoutType {
		case av.LayoutTypeTable:
			for _, col := range firstView.Table.Columns {
				view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: col.ID}})
			}
		case av.LayoutTypeGallery:
			for _, field := range firstView.Gallery.CardFields {
				view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: field.ID}})
			}
		case av.LayoutTypeKanban:
			for _, field := range firstView.Kanban.Fields {
				view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: field.ID}})
			}
		}
	case av.LayoutTypeKanban:
		view = env.MakeView(av.LayoutTypeKanban)
		if view == nil {
			return av.ErrWrongLayoutType
		}
		switch firstView.LayoutType {
		case av.LayoutTypeTable:
			for _, col := range firstView.Table.Columns {
				view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: col.ID}})
			}
		case av.LayoutTypeGallery:
			for _, field := range firstView.Gallery.CardFields {
				view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: field.ID}})
			}
		case av.LayoutTypeKanban:
			for _, field := range firstView.Kanban.Fields {
				view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: field.ID}})
			}
		}
	default:
		err = av.ErrWrongLayoutType
		env.
			Report("error", "wrong layout type [%s] for attribute view [%s]", layout, avID)
		return
	}

	if view == nil {
		return av.ErrWrongLayoutType
	}
	view.ItemIDs = slices.Clone(firstView.ItemIDs)
	attrView.ViewID = viewID
	view.ID = viewID
	attrView.Views = append(attrView.Views, view)

	if av.LayoutTypeKanban == layout {
		preferredGroupKey := GetKanbanPreferredGroupKey(attrView)
		group := &av.ViewGroup{Field: preferredGroupKey.ID}
		if err = SetAttributeViewGroupWithEnvironment(attrView, view, group, env); err != nil {
			return err
		}
	}

	node, tree, nodeErr := env.Node(blockID)
	if nodeErr != nil {
		return nodeErr
	}
	if nil == node || tree == nil {
		return ErrBlockNotFound
	}

	node.AttributeViewType = string(view.LayoutType)
	attrs := parse.IAL2Map(node.KramdownIAL)
	attrs[av.NodeAttrView] = viewID
	err = env.Attrs(node, tree, attrs)
	if err != nil {
		env.
			Report("warn", "set node [%s] attrs failed: %s", blockID, err)
		return
	}

	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return
	}
	return
}

func SetDatabaseBlockViewWithEnvironment(blockID, avID, viewID string, env *AttributeViewManagementEnvironment) (err error) {
	attrView, err := env.Read(avID)
	if nil != err {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return
	}
	if attrView.GetView(viewID) == nil {
		return av.ErrViewNotFound
	}
	if attrView.ViewID != viewID {
		attrView.ViewID = viewID
		if err = env.Stage(attrView); err != nil {
			return
		}
	}

	view := attrView.GetView(viewID)
	if nil == view {
		err = av.ErrViewNotFound
		env.
			Report("error", "view [%s] not found in attribute view [%s]", viewID, avID)
		return
	}

	node, tree, err := env.Node(blockID)
	if err != nil {
		return
	}

	if node == nil || tree == nil {
		return ErrBlockNotFound
	}
	node.AttributeViewType = string(view.LayoutType)
	attrs := parse.IAL2Map(node.KramdownIAL)
	attrs[av.NodeAttrView] = viewID
	err = env.Attrs(node, tree, attrs)
	if err != nil {
		env.
			Report("warn", "set node [%s] attrs failed: %s", blockID, err)
		return
	}
	return
}

func DoSortAttrViewViewWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", operation.AvID, err)
		return AttributeViewManagementError(err)
	}

	view := attrView.GetView(operation.ID)
	if nil == view {
		env.
			Report("error", "get view failed: %s", operation.BlockID)
		return av.ErrViewNotFound
	}
	viewID := view.ID
	previousViewID := operation.PreviousID
	if viewID == previousViewID {
		return
	}

	var index, previousIndex int
	for i, v := range attrView.Views {
		if v.ID == viewID {
			view = v
			index = i
			break
		}
	}

	attrView.Views = append(attrView.Views[:index], attrView.Views[index+1:]...)
	for i, v := range attrView.Views {
		if v.ID == previousViewID {
			previousIndex = i + 1
			break
		}
	}
	attrView.Views = util.InsertElem(attrView.Views, previousIndex, view)

	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}
	return
}

func DoRemoveAttrViewViewWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	if 1 >= len(attrView.Views) {
		env.
			Report("warn", "can't remove last view [%s] of attribute view [%s]", operation.AvID, avID)
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if nil == view {
		env.
			Report("warn", "get view failed: %s", operation.BlockID)
		return
	}

	viewID := view.ID
	var index int
	for i, view := range attrView.Views {
		if viewID == view.ID {
			attrView.Views = append(attrView.Views[:i], attrView.Views[i+1:]...)
			index = i - 1
			break
		}
	}
	if 0 > index {
		index = 0
	}

	view = attrView.Views[index]
	attrView.ViewID = view.ID
	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	if err = UpdateAttributeViewManagementMirrors(env, attrView, viewID, view); err != nil {
		return err
	}

	operation.RetData = view.LayoutType
	return
}

func DoDuplicateAttrViewViewWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	masterView := attrView.GetView(operation.PreviousID)
	if nil == masterView {
		env.
			Report("error", "get master view failed: %s", avID)
		return AttributeViewManagementError(err)
	}

	node, tree, nodeErr := env.Node(operation.BlockID)
	if nodeErr != nil {
		return nodeErr
	}
	if nil == node || tree == nil {
		env.
			Report("error", "get node by block ID [%s] failed", operation.BlockID)
		return AttributeViewManagementError(err)
	}

	attrs := parse.IAL2Map(node.KramdownIAL)
	attrs[av.NodeAttrView] = operation.ID
	node.AttributeViewType = string(masterView.LayoutType)
	err = env.Attrs(node, tree, attrs)
	if err != nil {
		env.
			Report("warn", "set node [%s] attrs failed: %s", operation.BlockID, err)
		return
	}

	var view *av.View
	switch masterView.LayoutType {
	case av.LayoutTypeTable:
		view = env.MakeView(av.LayoutTypeTable)
		if view == nil {
			return av.ErrWrongLayoutType
		}
	case av.LayoutTypeGallery:
		view = env.MakeView(av.LayoutTypeGallery)
		if view == nil {
			return av.ErrWrongLayoutType
		}
	case av.LayoutTypeKanban:
		view = env.MakeView(av.LayoutTypeKanban)
		if view == nil {
			return av.ErrWrongLayoutType
		}
	}

	if view == nil {
		return av.ErrWrongLayoutType
	}
	view.ID = operation.ID
	attrView.Views = append(attrView.Views, view)
	attrView.ViewID = view.ID

	view.Icon = masterView.Icon
	view.Name = util.GetDuplicateName(masterView.Name)
	view.HideAttrViewName = masterView.HideAttrViewName
	view.Desc = masterView.Desc
	view.LayoutType = masterView.LayoutType
	view.PageSize = masterView.PageSize

	for _, filter := range masterView.Filters {
		view.Filters = append(view.Filters, &av.ViewFilter{
			Column:        filter.Column,
			Qualifier:     filter.Qualifier,
			Operator:      filter.Operator,
			Value:         CloneAttributeViewManagementValue(filter.Value),
			RelativeDate:  filter.RelativeDate,
			RelativeDate2: filter.RelativeDate2,
		})
	}

	for _, s := range masterView.Sorts {
		view.Sorts = append(view.Sorts, &av.ViewSort{
			Column: s.Column,
			Order:  s.Order,
		})
	}

	switch masterView.LayoutType {
	case av.LayoutTypeTable:
		for _, col := range masterView.Table.Columns {
			view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{
				BaseField: &av.BaseField{
					ID:     col.ID,
					Wrap:   col.Wrap,
					Hidden: col.Hidden,
					Desc:   col.Desc,
				},
				Pin:   col.Pin,
				Width: col.Width,
				Calc:  col.Calc,
			})
		}

		view.Table.ShowIcon = masterView.Table.ShowIcon
		view.Table.WrapField = masterView.Table.WrapField
	case av.LayoutTypeGallery:
		for _, field := range masterView.Gallery.CardFields {
			view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{
				BaseField: &av.BaseField{
					ID:     field.ID,
					Wrap:   field.Wrap,
					Hidden: field.Hidden,
					Desc:   field.Desc,
				},
			})
		}

		view.Gallery.CoverFrom = masterView.Gallery.CoverFrom
		view.Gallery.CoverFromAssetKeyID = masterView.Gallery.CoverFromAssetKeyID
		view.Gallery.CardSize = masterView.Gallery.CardSize
		view.Gallery.FitImage = masterView.Gallery.FitImage
		view.Gallery.DisplayFieldName = masterView.Gallery.DisplayFieldName
		view.Gallery.ShowIcon = masterView.Gallery.ShowIcon
		view.Gallery.WrapField = masterView.Gallery.WrapField
	case av.LayoutTypeKanban:
		for _, field := range masterView.Kanban.Fields {
			view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{
				BaseField: &av.BaseField{
					ID:     field.ID,
					Wrap:   field.Wrap,
					Hidden: field.Hidden,
					Desc:   field.Desc,
				},
			})
		}

		view.Kanban.CoverFrom = masterView.Kanban.CoverFrom
		view.Kanban.CoverFromAssetKeyID = masterView.Kanban.CoverFromAssetKeyID
		view.Kanban.CardSize = masterView.Kanban.CardSize
		view.Kanban.FitImage = masterView.Kanban.FitImage
		view.Kanban.DisplayFieldName = masterView.Kanban.DisplayFieldName
		view.Kanban.FillColBackgroundColor = masterView.Kanban.FillColBackgroundColor
		view.Kanban.ShowIcon = masterView.Kanban.ShowIcon
		view.Kanban.WrapField = masterView.Kanban.WrapField
	}

	view.ItemIDs = slices.Clone(masterView.ItemIDs)

	if nil != masterView.Group {
		view.Group = &av.ViewGroup{}
		if copyErr := copier.Copy(view.Group, masterView.Group); nil != copyErr {
			env.
				Report("error", "copy group failed: %s", copyErr)
			return copyErr
		}

		view.GroupItemIDs = masterView.GroupItemIDs

	}

	cloned, cloneErr := env.CloneView(view)
	if cloneErr != nil {
		return cloneErr
	}
	if cloned == nil {
		return errors.New("invalid_av_view_clone")
	}
	view = cloned
	attrView.Views[len(attrView.Views)-1] = view
	if view.Group != nil {
		if err = env.Groups(attrView); err != nil {
			return err
		}
	}

	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}
	return
}

func DoSetAttrViewViewNameWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	viewID := operation.ID
	view := attrView.GetView(viewID)
	if nil == view {
		env.
			Report("error", "get view [%s] failed: %s", viewID, err)
		return AttributeViewManagementError(err)
	}

	view.Name = strings.TrimSpace(operation.Data.(string))
	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}
	return
}

func DoSetAttrViewViewIconWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	viewID := operation.ID
	view := attrView.GetView(viewID)
	if nil == view {
		env.
			Report("error", "get view [%s] failed: %s", viewID, err)
		return AttributeViewManagementError(err)
	}

	view.Icon = operation.Data.(string)
	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}
	return
}

func DoSetAttrViewViewDescWithEnvironment(operation *Operation, env *AttributeViewManagementEnvironment) (err error) {
	avID := operation.AvID
	attrView, err := env.Read(avID)
	if err != nil {
		env.
			Report("error", "parse attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}

	viewID := operation.ID
	view := attrView.GetView(viewID)
	if nil == view {
		env.
			Report("error", "get view [%s] failed: %s", viewID, err)
		return AttributeViewManagementError(err)
	}

	view.Desc = strings.TrimSpace(operation.Data.(string))
	if err = env.Stage(attrView); err != nil {
		env.
			Report("error", "save attribute view [%s] failed: %s", avID, err)
		return AttributeViewManagementError(err)
	}
	return
}

func SetAttributeViewGroupWithEnvironment(attrView *av.AttributeView, view *av.View, group *av.ViewGroup, env *AttributeViewManagementEnvironment) (err error) {
	if group == nil {
		return errors.New("invalid_av_group")
	}
	var oldHideEmpty, firstInit, changeGroupField bool
	if nil != view.Group {
		oldHideEmpty = view.Group.HideEmpty
		changeGroupField = group.Field != view.Group.Field
	} else {
		firstInit = true
	}

	groupStates := GetAttrViewGroupStates(view)
	view.Group = group
	if err = env.Groups(attrView); err != nil {
		return err
	}
	SetAttrViewGroupStates(view, groupStates)

	if view.Group.HideEmpty != oldHideEmpty {
		if !oldHideEmpty && view.Group.HideEmpty {
			for _, g := range view.Groups {
				count, countErr := env.Count(attrView, view, g)
				if countErr != nil {
					return countErr
				}
				if g.GroupHidden == 0 && 1 > count {
					g.GroupHidden = 1
				}
			}
		}
		if oldHideEmpty && !view.Group.HideEmpty {
			for _, g := range view.Groups {
				count, countErr := env.Count(attrView, view, g)
				if countErr != nil {
					return countErr
				}
				if g.GroupHidden == 1 && 1 > count {
					g.GroupHidden = 0
				}
			}
		}
	}

	if firstInit || changeGroupField {
		if groupKey := view.GetGroupKey(attrView); nil != groupKey {
			if av.KeyTypeSelect == groupKey.Type || av.KeyTypeMSelect == groupKey.Type {

				view.Group.Order = av.GroupOrderSelectOption
				SortGroupsBySelectOption(view, groupKey)
			} else if av.KeyTypeCheckbox == groupKey.Type {

				view.Group.Order = av.GroupOrderMan
				checked := view.GetGroupByGroupValue(av.CheckboxCheckedStr)
				unchecked := view.GetGroupByGroupValue("")
				view.Groups = nil
				view.Groups = append(view.Groups, checked, unchecked)
			}

		}

		for i, g := range view.Groups {
			if g == nil {
				return errors.New("invalid_av_generated_group")
			}
			g.GroupSort = i
		}
	}
	return nil
}

func ValidateAttributeViewNameInput(op *Operation) error {
	if op == nil || op.Action != "setAttrViewName" || op.ID == "" {
		return errors.New("invalid_av_name_operation")
	}
	if _, ok := op.Data.(string); !ok {
		return errors.New("invalid_av_name_data")
	}
	return nil
}

func (e *AttributeViewNameEnvironment) Validate() error {
	if e == nil || e.Ctx == nil || e.Read == nil || e.Stage == nil || e.Exists == nil || e.Node == nil || e.Name == nil || e.Untitled == nil || e.Tree == nil || e.Cache == nil || e.Notify == nil {
		return errors.New("transaction_av_name_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func RunAttributeViewName(op *Operation, e *AttributeViewNameEnvironment) error {
	if err := ValidateAttributeViewNameInput(op); err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	ctx := e.Ctx
	guard := func(fn func() error) error {
		if e.OwnerGuard != nil {
			if err := e.OwnerGuard(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		if e.OwnerGuard != nil {
			return e.OwnerGuard()
		}
		return ctx.Err()
	}
	var a *av.AttributeView
	if err := guard(func() (err error) { a, err = e.Read(op.ID); return }); err != nil {
		return err
	}
	if err := ValidateCellSource(a); err != nil {
		return err
	}
	if a.ID != op.ID || a.GetBlockKeyValues() == nil {
		return errors.New("invalid_av_name_source")
	}
	a.Name = strings.TrimSpace(op.Data.(string))
	a.Name = strings.ReplaceAll(a.Name, "\n", " ")
	if utf8.RuneCountInString(a.Name) > 512 {
		a.Name = gulu.Str.SubStr(a.Name, 512)
	}
	if err := guard(func() error { return e.Stage(a) }); err != nil {
		return err
	}
	owned := map[string]*parse.Tree{}
	var trees []*parse.Tree
	for _, v := range a.GetBlockKeyValues().Values {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.IsDetached {
			continue
		}
		id := v.Block.ID
		var exists bool
		if err := guard(func() (err error) { exists, err = e.Exists(id); return }); err != nil {
			return err
		}
		if !exists {
			continue
		}
		var n *ast.Node
		var t *parse.Tree
		err := guard(func() (err error) { n, t, err = e.Node(id); return })
		if err != nil {
			if ctx.Err() != nil || !e.Legacy || e.StrictDependencies {
				return err
			}
			continue
		}
		if t == nil || t.Root == nil || t.ID == "" || t.Box == "" || t.Path == "" {
			if e.Legacy && !e.StrictDependencies {
				continue
			}
			return errors.New("invalid_av_name_tree")
		}
		if first := owned[t.ID]; first != nil {
			if first.Box != t.Box || first.Path != t.Path {
				return errors.New("invalid_av_name_tree_identity")
			}
			t = first
			n = treenode.GetNodeInTree(t, id)
		} else {
			owned[t.ID] = t
			trees = append(trees, t)
		}

		if n == nil {
			continue
		}
		if n.ID != id {
			return errors.New("invalid_av_name_node_identity")
		}
		var names string
		if e.Names != nil {
			err = guard(func() error {
				var readErr error
				names, readErr = e.Names(n.IALAttr(av.NodeAttrNameAvs))
				return readErr
			})
		} else {
			names, err = AttributeViewNamesWithServices(n.IALAttr(av.NodeAttrNameAvs), func(id string) (name string, err error) {
				err = guard(func() error {
					if id == a.ID {
						name = a.Name
						return nil
					}
					name, err = e.Name(id)
					return err
				})
				return
			}, e.Untitled, e.Legacy)
		}

		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		old := parse.IAL2Map(n.KramdownIAL)
		n.SetIALAttr(av.NodeAttrViewNames, names)
		if err = guard(func() error { return e.Cache(n.ID, parse.IAL2Map(n.KramdownIAL)) }); err != nil {
			return err
		}
		if err = guard(func() error { return e.Notify(old, n) }); err != nil {
			return err
		}
	}
	for _, t := range trees {
		if err := guard(func() error { return e.Tree(t) }); err != nil {
			return err
		}
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

func (e *AttributeViewOptionEnvironment) Validate(action string) error {
	if e == nil || e.Read == nil || e.Stage == nil || (action != "setAttrViewColOptionDesc" && e.Groups == nil) {
		return errors.New("transaction_av_option_dependencies_unbound")
	}
	return nil
}

func ValidateAttributeViewOptionInput(op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_option_operation")
	}
	switch op.Action {
	case "updateAttrViewColOptions":

	case "removeAttrViewColOption":
		if _, ok := op.Data.(string); !ok {
			return errors.New("invalid_av_option_name")
		}
	case "updateAttrViewColOption", "setAttrViewColOptionDesc":
		m, ok := op.Data.(map[string]interface{})
		if !ok {
			return errors.New("invalid_av_option_data")
		}
		keys := []string{"name", "desc"}
		if op.Action == "updateAttrViewColOption" {
			keys = []string{"oldName", "newName", "newDesc", "newColor"}
		}
		for _, key := range keys {
			if _, ok = m[key].(string); !ok {
				return errors.New("invalid_av_option_data")
			}
		}
	default:
		return errors.New("invalid_av_option_operation")
	}
	return nil
}

func ValidateAttributeViewOptionSource(a *av.AttributeView, op *Operation) error {
	if a == nil {
		return errors.New("invalid_av_option_source")
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return errors.New("invalid_av_option_source")
		}
		if kv.Key.ID != op.ID {
			continue
		}
		for _, option := range kv.Key.Options {
			if option == nil {
				return errors.New("invalid_av_option_source")
			}
		}
		if op.Action == "removeAttrViewColOption" || op.Action == "updateAttrViewColOption" {
			for _, value := range kv.Values {
				if value != nil {
					for _, option := range value.MSelect {
						if option == nil {
							return errors.New("invalid_av_option_source")
						}
					}
				}
			}
		}
	}
	if op.Action == "setAttrViewColOptionDesc" {
		return nil
	}
	for _, v := range a.Views {
		if v == nil {
			return errors.New("invalid_av_option_source")
		}
		for _, f := range v.Filters {
			if f == nil {
				return errors.New("invalid_av_option_source")
			}
			if f.Column == op.ID && f.Value != nil && (f.Value.Type == av.KeyTypeSelect || f.Value.Type == av.KeyTypeMSelect) {
				for _, option := range f.Value.MSelect {
					if option == nil {
						return errors.New("invalid_av_option_source")
					}
				}
			}
		}
	}
	return nil
}

func RunAttributeViewOption(ctx context.Context, op *Operation, e *AttributeViewOptionEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateAttributeViewOptionInput(op); err != nil {
		return err
	}
	if err := e.Validate(op.Action); err != nil {
		return err
	}
	local := *e
	local.Read = func(id string) (*av.AttributeView, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := e.Read(id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = ValidateAttributeViewOptionSource(a, op); err != nil {
			return nil, err
		}

		a.RenderedViewables = map[string]av.Viewable{}
		return a, nil
	}
	local.Stage = func(a *av.AttributeView) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.Stage(a); err != nil {
			return err
		}
		return ctx.Err()
	}
	local.Groups = func(a *av.AttributeView) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Groups == nil {
			return errors.New("transaction_av_option_dependencies_unbound")
		}
		if err := e.Groups(a); err != nil {
			return err
		}
		return ctx.Err()
	}
	var err error
	switch op.Action {
	case "updateAttrViewColOptions":
		err = UpdateAttributeViewColumnOptionsWithEnvironment(op, &local)
	case "removeAttrViewColOption":
		err = RemoveAttributeViewColumnOptionWithEnvironment(op, &local)
	case "updateAttrViewColOption":
		err = UpdateAttributeViewColumnOptionWithEnvironment(op, &local)
	case "setAttrViewColOptionDesc":
		err = SetAttributeViewColumnOptionDescWithEnvironment(op, &local)
	}
	if err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

func UpdateAttributeViewColumnOptionsWithEnvironment(operation *Operation, e *AttributeViewOptionEnvironment) (err error) {
	attrView, err := e.Read(operation.AvID)
	if err != nil {
		return
	}

	jsonData, err := gulu.JSON.MarshalJSON(operation.Data)
	if err != nil {
		return
	}

	options := []*av.SelectOption{}
	if err = gulu.JSON.UnmarshalJSON(jsonData, &options); err != nil {
		return
	}

	// 移除空选项 https://github.com/siyuan-note/siyuan/issues/15533
	var tmp []*av.SelectOption
	for _, opt := range options {
		if opt == nil {
			return errors.New("invalid_av_option_data")
		}
		if "" != opt.Name {
			tmp = append(tmp, opt)
		}
	}
	options = tmp
	if 1 > len(options) {
		return
	}

	optionSorts := map[string]int{}
	for i, opt := range options {
		optionSorts[opt.Name] = i
	}

	addNew := false
	selectKey, _ := attrView.GetKey(operation.ID)
	if nil == selectKey {
		return
	}
	existingOptions := map[string]*av.SelectOption{}
	for _, opt := range selectKey.Options {
		existingOptions[opt.Name] = opt
	}
	for _, opt := range options {
		if existingOpt, exists := existingOptions[opt.Name]; exists {

			existingOpt.Color = opt.Color
			existingOpt.Desc = opt.Desc
		} else {

			selectKey.Options = append(selectKey.Options, &av.SelectOption{
				Name:  opt.Name,
				Color: opt.Color,
				Desc:  opt.Desc,
			})
			addNew = true
		}
	}

	if !addNew {
		sort.SliceStable(selectKey.Options, func(i, j int) bool {
			return optionSorts[selectKey.Options[i].Name] < optionSorts[selectKey.Options[j].Name]
		})
	}

	if err = e.Groups(attrView); err != nil {
		return
	}
	err = e.Stage(attrView)
	return
}

func RemoveAttributeViewColumnOptionWithEnvironment(operation *Operation, e *AttributeViewOptionEnvironment) (err error) {
	attrView, err := e.Read(operation.AvID)
	if err != nil {
		return
	}

	optName := operation.Data.(string)

	key, err := attrView.GetKey(operation.ID)
	if err != nil {
		return
	}

	for i, opt := range key.Options {
		if optName == opt.Name {
			key.Options = append(key.Options[:i], key.Options[i+1:]...)
			break
		}
	}

	for _, keyValues := range attrView.KeyValues {
		if keyValues.Key.ID != operation.ID {
			continue
		}

		for _, value := range keyValues.Values {
			if nil == value || nil == value.MSelect {
				continue
			}

			for i, opt := range value.MSelect {
				if optName == opt.Content {
					value.MSelect = append(value.MSelect[:i], value.MSelect[i+1:]...)
					break
				}
			}
		}
		break
	}

	for _, view := range attrView.Views {

		for _, filter := range slices.Clone(view.Filters) {
			if filter.Column != operation.ID {
				continue
			}

			if nil != filter.Value && (av.KeyTypeSelect == filter.Value.Type || av.KeyTypeMSelect == filter.Value.Type) {
				if av.FilterOperatorIsEmpty == filter.Operator || av.FilterOperatorIsNotEmpty == filter.Operator {
					continue
				}

				for i, opt := range filter.Value.MSelect {
					if optName == opt.Content {
						filter.Value.MSelect = append(filter.Value.MSelect[:i], filter.Value.MSelect[i+1:]...)
						break
					}
				}
				if 1 > len(filter.Value.MSelect) {

					for i, f := range view.Filters {
						if f.Column == operation.ID && f.Value == filter.Value {
							view.Filters = append(view.Filters[:i], view.Filters[i+1:]...)
							break
						}
					}
				}
			}
		}
	}

	if err = e.Groups(attrView); err != nil {
		return
	}
	err = e.Stage(attrView)
	return
}

func UpdateAttributeViewColumnOptionWithEnvironment(operation *Operation, e *AttributeViewOptionEnvironment) (err error) {
	attrView, err := e.Read(operation.AvID)
	if err != nil {
		return
	}

	key, err := attrView.GetKey(operation.ID)
	if err != nil {
		return
	}

	data := operation.Data.(map[string]interface{})

	rename := false
	oldName := strings.TrimSpace(data["oldName"].(string))
	newName := strings.TrimSpace(data["newName"].(string))
	newDesc := strings.TrimSpace(data["newDesc"].(string))
	newColor := data["newColor"].(string)

	found := false
	if oldName != newName {
		rename = true

		for _, opt := range key.Options {
			if newName == opt.Name {
				found = true
				newColor = opt.Color
				newDesc = opt.Desc
				break
			}
		}
	}

	if !found {
		for i, opt := range key.Options {
			if oldName == opt.Name {
				key.Options[i].Name = newName
				key.Options[i].Color = newColor
				key.Options[i].Desc = newDesc
				break
			}
		}
	}

	for _, keyValues := range attrView.KeyValues {
		if keyValues.Key.ID != operation.ID {
			continue
		}

		for _, value := range keyValues.Values {
			if nil == value || nil == value.MSelect {
				continue
			}

			found = false
			for _, opt := range value.MSelect {
				if newName == opt.Content {
					found = true
					break
				}
			}
			if found && rename {
				idx := -1
				for i, opt := range value.MSelect {
					if oldName == opt.Content {
						idx = i
						break
					}
				}
				if 0 <= idx {
					value.MSelect = util.RemoveElem(value.MSelect, idx)
				}
			} else {
				for i, opt := range value.MSelect {
					if oldName == opt.Content {
						value.MSelect[i].Content = newName
						value.MSelect[i].Color = newColor
						break
					}
				}
			}
		}
		break
	}

	for _, view := range attrView.Views {
		for _, filter := range view.Filters {
			if filter.Column != key.ID {
				continue
			}

			if nil != filter.Value && (av.KeyTypeSelect == filter.Value.Type || av.KeyTypeMSelect == filter.Value.Type) {
				for i, opt := range filter.Value.MSelect {
					if oldName == opt.Content {
						filter.Value.MSelect[i].Content = newName
						filter.Value.MSelect[i].Color = newColor
						break
					}
				}
			}
		}
	}

	if err = e.Groups(attrView); err != nil {
		return
	}
	err = e.Stage(attrView)
	return
}

func SetAttributeViewColumnOptionDescWithEnvironment(operation *Operation, e *AttributeViewOptionEnvironment) (err error) {
	attrView, err := e.Read(operation.AvID)
	if err != nil {
		return
	}

	key, err := attrView.GetKey(operation.ID)
	if err != nil {
		return
	}

	data := operation.Data.(map[string]interface{})
	name := data["name"].(string)
	desc := data["desc"].(string)

	for i, opt := range key.Options {
		if name == opt.Name {
			key.Options[i].Desc = desc
			break
		}
	}

	err = e.Stage(attrView)
	return
}

func (e *AttributeViewDefaultEnvironment) Validate() error {
	if e == nil || e.Ctx == nil || e.Near == nil || e.TemplateKeys == nil || e.Reverse == nil || e.Now == nil || e.NewID == nil {
		return errors.New("transaction_av_default_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func (e *AttributeViewDefaultEnvironment) NearValue(item av.Item, key *av.Key, row string) *av.Value {
	if item == nil {
		return nil
	}
	v := item.GetValue(key.ID)
	if v == nil {
		return nil
	}
	v = v.Clone()
	if v == nil {
		return nil
	}
	v.ID = e.NewID()
	v.KeyID = key.ID
	v.BlockID = row
	v.CreatedAt = e.Now().UnixMilli()
	v.UpdatedAt = v.CreatedAt + 1000
	return v
}

func IsAttributeViewOrderAction(s string) bool {
	return s == "sortAttrViewRow" || s == "sortAttrViewCol" || s == "sortAttrViewKey" || s == "duplicateAttrViewKey"
}

func (e *AttributeViewOrderEnvironment) Validate(action string) error {
	if e == nil || e.Read == nil || e.Stage == nil || ((action == "sortAttrViewRow" || action == "sortAttrViewCol") && e.View == nil) || (action == "duplicateAttrViewKey" && e.CloneKey == nil) || (action == "sortAttrViewRow" && e.Groups == nil) {
		return errors.New("transaction_av_order_dependencies_unbound")
	}
	if action == "sortAttrViewRow" {
		return e.Defaults.Validate()
	}
	return nil
}

func ValidateAttributeViewOrderInput(op *Operation) error {
	if op == nil || !IsAttributeViewOrderAction(op.Action) {
		return errors.New("invalid_av_order_operation")
	}
	if op.Action == "duplicateAttrViewKey" && (op.NextID == "" || op.NextID == op.KeyID) {
		return errors.New("invalid_av_duplicate_key_id")
	}
	return nil
}

func RunAttributeViewOrder(ctx context.Context, op *Operation, e *AttributeViewOrderEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateAttributeViewOrderInput(op); err != nil {
		return err
	}
	if err := e.Validate(op.Action); err != nil {
		return err
	}
	local := *e
	local.Read = func(id string) (*av.AttributeView, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := e.Read(id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = ValidateAttributeViewFieldSource(a); err != nil {
			return nil, err
		}
		for _, v := range a.Views {
			for _, g := range v.Groups {
				if g == nil {
					return nil, ErrInvalidAVOrderSource
				}
			}
		}
		for _, kv := range a.KeyValues {
			for _, o := range kv.Key.Options {
				if o == nil {
					return nil, ErrInvalidAVOrderSource
				}
			}
			for _, v := range kv.Values {
				for _, o := range v.MSelect {
					if o == nil {
						return nil, ErrInvalidAVOrderSource
					}
				}
			}
		}
		a.RenderedViewables = map[string]av.Viewable{}
		return a, nil
	}
	wrap := func(fn func(*av.AttributeView) error) func(*av.AttributeView) error {
		return func(a *av.AttributeView) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(a); err != nil {
				return err
			}
			return ctx.Err()
		}
	}
	local.Stage = wrap(e.Stage)
	if e.Groups != nil {
		local.Groups = wrap(e.Groups)
	}
	local.View = func(a *av.AttributeView, id string) (*av.View, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, err := e.View(a, id)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if v == nil {
			return nil, ErrInvalidAVOrderSource
		}
		return v, nil
	}
	local.CloneKey = func(k *av.Key) (*av.Key, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k, err := e.CloneKey(k)
		if err != nil {
			return nil, err
		}
		return k, ctx.Err()
	}
	if e.Defaults != nil {
		d := *e.Defaults
		d.Ctx = ctx
		d.Near = func(a *av.AttributeView, v, g *av.View, p string) (av.Item, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			item, err := e.Defaults.Near(a, v, g, p)
			if err != nil {
				return nil, err
			}
			return item, ctx.Err()
		}
		d.TemplateKeys = func(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			keys, err := e.Defaults.TemplateKeys(a, k)
			if err != nil {
				return nil, err
			}
			return keys, ctx.Err()
		}
		d.Reverse = func(a *av.AttributeView, k *av.Key, v *av.Value) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := e.Defaults.Reverse(a, k, v); err != nil {
				return err
			}
			return ctx.Err()
		}
		local.Defaults = &d
	}
	var err error
	switch op.Action {
	case "sortAttrViewRow":
		err = SortAttributeViewRowWithEnvironment(op, &local)
	case "sortAttrViewCol":
		err = SortAttributeViewViewKeyWithEnvironment(op.AvID, op.BlockID, op.ID, op.PreviousID, &local)
	case "sortAttrViewKey":
		err = SortAttributeViewKeyWithEnvironment(op.AvID, op.ID, op.PreviousID, &local)
	case "duplicateAttrViewKey":
		err = DuplicateAttributeViewKeyWithEnvironment(op, &local)
	}
	if err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

func SortAttributeViewRowWithEnvironment(operation *Operation, env *AttributeViewOrderEnvironment) (err error) {
	if operation.ID == operation.PreviousID {

		return
	}

	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, operation.BlockID)
	if err != nil {
		return
	}

	var itemID string
	var idx, previousIndex int

	if nil != view.Group && "" != operation.GroupID {
		if groupView := view.GetGroupByID(operation.GroupID); nil != groupView {
			groupKey := view.GetGroupKey(attrView)
			if groupKey == nil {
				return ErrInvalidAVOrderSource
			}
			isAcrossGroup := operation.GroupID != operation.TargetGroupID
			if isAcrossGroup && (av.KeyTypeTemplate == groupKey.Type || av.KeyTypeCreated == groupKey.Type || av.KeyTypeUpdated == groupKey.Type) {

				return
			}

			for i, id := range groupView.GroupItemIDs {
				if id == operation.ID {
					itemID = id
					idx = i
					break
				}
			}
			if "" == itemID {
				itemID = operation.ID
				groupView.GroupItemIDs = append(groupView.GroupItemIDs, itemID)
				idx = len(groupView.GroupItemIDs) - 1
			}
			groupView.GroupItemIDs = append(groupView.GroupItemIDs[:idx], groupView.GroupItemIDs[idx+1:]...)

			if isAcrossGroup {
				if targetGroupView := view.GetGroupByID(operation.TargetGroupID); nil != targetGroupView && !gulu.Str.Contains(itemID, targetGroupView.GroupItemIDs) {
					if err = FillAttributeViewDefaultValues(attrView, view, targetGroupView, operation.PreviousID, itemID, false, env.Defaults); err != nil {
						return err
					}

					if val := attrView.GetValue(groupKey.ID, itemID); nil != val {
						if av.MSelectExistOption(val.MSelect, groupView.GetGroupValue()) {
							val.MSelect = av.MSelectRemoveOption(val.MSelect, groupView.GetGroupValue())
						}
					}

					for i, r := range targetGroupView.GroupItemIDs {
						if r == operation.PreviousID {
							previousIndex = i + 1
							break
						}
					}
					targetGroupView.GroupItemIDs = util.InsertElem(targetGroupView.GroupItemIDs, previousIndex, itemID)
				}

				if err = env.Groups(attrView); err != nil {
					return err
				}
			} else {
				for i, r := range groupView.GroupItemIDs {
					if r == operation.PreviousID {
						previousIndex = i + 1
						break
					}
				}
				groupView.GroupItemIDs = util.InsertElem(groupView.GroupItemIDs, previousIndex, itemID)
			}
		}
	} else {
		for i, id := range view.ItemIDs {
			if id == operation.ID {
				itemID = id
				idx = i
				break
			}
		}
		if "" == itemID {
			itemID = operation.ID
			view.ItemIDs = append(view.ItemIDs, itemID)
			idx = len(view.ItemIDs) - 1
		}

		view.ItemIDs = append(view.ItemIDs[:idx], view.ItemIDs[idx+1:]...)
		for i, r := range view.ItemIDs {
			if r == operation.PreviousID {
				previousIndex = i + 1
				break
			}
		}
		view.ItemIDs = util.InsertElem(view.ItemIDs, previousIndex, itemID)
	}

	err = env.Stage(attrView)
	return
}

func SortAttributeViewViewKeyWithEnvironment(avID, blockID, keyID, previousKeyID string, env *AttributeViewOrderEnvironment) (err error) {
	if keyID == previousKeyID {

		return
	}

	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	view, err := env.View(attrView, blockID)
	if err != nil {
		return
	}

	var curIndex, previousIndex int
	switch view.LayoutType {
	case av.LayoutTypeTable:
		var col *av.ViewTableColumn
		for i, column := range view.Table.Columns {
			if column.ID == keyID {
				col = column
				curIndex = i
				break
			}
		}
		if nil == col {
			return
		}

		view.Table.Columns = append(view.Table.Columns[:curIndex], view.Table.Columns[curIndex+1:]...)
		for i, column := range view.Table.Columns {
			if column.ID == previousKeyID {
				previousIndex = i + 1
				break
			}
		}
		view.Table.Columns = util.InsertElem(view.Table.Columns, previousIndex, col)
	case av.LayoutTypeGallery:
		var field *av.ViewGalleryCardField
		for i, cardField := range view.Gallery.CardFields {
			if cardField.ID == keyID {
				field = cardField
				curIndex = i
				break
			}
		}
		if nil == field {
			return
		}

		view.Gallery.CardFields = append(view.Gallery.CardFields[:curIndex], view.Gallery.CardFields[curIndex+1:]...)
		for i, cardField := range view.Gallery.CardFields {
			if cardField.ID == previousKeyID {
				previousIndex = i + 1
				break
			}
		}
		view.Gallery.CardFields = util.InsertElem(view.Gallery.CardFields, previousIndex, field)
	case av.LayoutTypeKanban:
		var field *av.ViewKanbanField
		for i, kanbanField := range view.Kanban.Fields {
			if kanbanField.ID == keyID {
				field = kanbanField
				curIndex = i
				break
			}
		}
		if nil == field {
			return
		}

		view.Kanban.Fields = append(view.Kanban.Fields[:curIndex], view.Kanban.Fields[curIndex+1:]...)
		for i, kanbanField := range view.Kanban.Fields {
			if kanbanField.ID == previousKeyID {
				previousIndex = i + 1
				break
			}
		}
		view.Kanban.Fields = util.InsertElem(view.Kanban.Fields, previousIndex, field)
	}

	err = env.Stage(attrView)
	return
}

func SortAttributeViewKeyWithEnvironment(avID, keyID, previousKeyID string, env *AttributeViewOrderEnvironment) (err error) {
	if keyID == previousKeyID {
		return
	}

	attrView, err := env.Read(avID)
	if err != nil {
		return
	}

	RefreshAttributeViewKeyIDs(attrView)

	var currentKeyID string
	var idx, previousIndex int
	for i, k := range attrView.KeyIDs {
		if k == keyID {
			currentKeyID = k
			idx = i
			break
		}
	}
	if "" == currentKeyID {
		return
	}

	attrView.KeyIDs = append(attrView.KeyIDs[:idx], attrView.KeyIDs[idx+1:]...)

	for i, k := range attrView.KeyIDs {
		if k == previousKeyID {
			previousIndex = i + 1
			break
		}
	}
	attrView.KeyIDs = util.InsertElem(attrView.KeyIDs, previousIndex, currentKeyID)

	err = env.Stage(attrView)
	return
}

func DuplicateAttributeViewKeyWithEnvironment(operation *Operation, env *AttributeViewOrderEnvironment) (err error) {
	attrView, err := env.Read(operation.AvID)
	if err != nil {
		return
	}

	key, _ := attrView.GetKey(operation.KeyID)
	if nil == key {
		return
	}

	if av.KeyTypeBlock == key.Type || av.KeyTypeRelation == key.Type {
		return
	}

	copyKey, err := env.CloneKey(key)
	if err != nil {
		return err
	}
	if copyKey == nil {
		return ErrInvalidAVOrderSource
	}
	copyKey.ID = operation.NextID
	copyKey.Name = util.GetDuplicateName(key.Name)

	attrView.KeyValues = append(attrView.KeyValues, &av.KeyValues{Key: copyKey})

	for _, view := range attrView.Views {
		switch view.LayoutType {
		case av.LayoutTypeTable:
			for i, column := range view.Table.Columns {
				if column.ID == key.ID {
					view.Table.Columns = append(view.Table.Columns[:i+1], append([]*av.ViewTableColumn{
						{
							BaseField: &av.BaseField{
								ID:     copyKey.ID,
								Wrap:   column.Wrap,
								Hidden: column.Hidden,
								Desc:   column.Desc,
							},
							Pin:   column.Pin,
							Width: column.Width,
						},
					}, view.Table.Columns[i+1:]...)...)
					break
				}
			}
		case av.LayoutTypeGallery:
			for i, field := range view.Gallery.CardFields {
				if field.ID == key.ID {
					view.Gallery.CardFields = append(view.Gallery.CardFields[:i+1], append([]*av.ViewGalleryCardField{
						{
							BaseField: &av.BaseField{
								ID:     copyKey.ID,
								Wrap:   field.Wrap,
								Hidden: field.Hidden,
								Desc:   field.Desc,
							},
						},
					}, view.Gallery.CardFields[i+1:]...)...)
					break
				}
			}
		case av.LayoutTypeKanban:
			for i, field := range view.Kanban.Fields {
				if field.ID == key.ID {
					view.Kanban.Fields = append(view.Kanban.Fields[:i+1], append([]*av.ViewKanbanField{
						{
							BaseField: &av.BaseField{
								ID:     copyKey.ID,
								Wrap:   field.Wrap,
								Hidden: field.Hidden,
								Desc:   field.Desc,
							},
						},
					}, view.Kanban.Fields[i+1:]...)...)
					break
				}
			}
		}
	}

	err = env.Stage(attrView)
	return
}

func RefreshAttributeViewKeyIDs(attrView *av.AttributeView) {

	existKeyIDs := map[string]bool{}
	for _, keyValues := range attrView.KeyValues {
		existKeyIDs[keyValues.Key.ID] = true
	}

	for k, _ := range existKeyIDs {
		if !gulu.Str.Contains(k, attrView.KeyIDs) {
			attrView.KeyIDs = append(attrView.KeyIDs, k)
		}
	}

	var tmp []string
	for _, k := range attrView.KeyIDs {
		if ok := existKeyIDs[k]; ok {
			tmp = append(tmp, k)
		}
	}
	attrView.KeyIDs = tmp

}

func SortGroupsBySelectOption(v *av.View, k *av.Key) { avrender.SortBySelectOption(v, k) }

func IsAttributeViewRowAction(s string) bool {
	return s == "insertAttrViewBlock" || s == "removeAttrViewBlock" || s == "replaceAttrViewBlock"
}

func ValidateAttributeViewRowInput(op *Operation) error {
	if op == nil || !IsAttributeViewRowAction(op.Action) || op.AvID == "" {
		return errors.New("invalid_av_row_operation")
	}
	if op.Action == "insertAttrViewBlock" {
		for _, s := range op.Srcs {
			if s == nil {
				return errors.New("invalid_av_row_source")
			}
			d, ok := s["isDetached"].(bool)
			if !ok {
				return errors.New("invalid_av_row_source")
			}
			for _, k := range []string{"itemID", "content"} {
				if v, present := s[k]; present && v != nil {
					if _, ok := v.(string); !ok {
						return errors.New("invalid_av_row_source")
					}
				}
			}
			if !d {
				if _, ok := s["id"].(string); !ok {
					return errors.New("invalid_av_row_source")
				}
			}
		}
	}
	if op.Action == "replaceAttrViewBlock" && (op.PreviousID == "" || (!op.IsDetached && !ast.IsNodeIDPattern(op.NextID))) {
		return errors.New("invalid_av_row_replacement")
	}
	return nil
}

func (e *AttributeViewRowEnvironment) Validate(action string) error {
	if e == nil {
		return errors.New("transaction_av_row_dependencies_unbound")
	}
	if err := e.Cell.Validate(); err != nil {
		return err
	}
	if (action == "insertAttrViewBlock" && (e.View == nil || e.Text == nil || e.Duplicate == nil)) || (action == "replaceAttrViewBlock" && e.Text == nil) || (action == "removeAttrViewBlock" && (e.OptionalNode == nil || e.Exists == nil || e.History == nil)) {
		return errors.New("transaction_av_row_dependencies_unbound")
	}
	if action == "insertAttrViewBlock" {
		return e.Defaults.Validate()
	}
	return nil
}

func RunAttributeViewRow(op *Operation, e *AttributeViewRowEnvironment) error {
	if err := ValidateAttributeViewRowInput(op); err != nil {
		return err
	}
	if err := e.Validate(op.Action); err != nil {
		return err
	}
	ctx := e.Cell.Ctx

	local := *e
	c := *e.Cell
	local.Cell = &c
	guard := func(f func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := f(); err != nil {
			return err
		}
		return ctx.Err()
	}
	c.Read = func(id string) (a *av.AttributeView, err error) {
		err = guard(func() error { a, err = e.Cell.Read(id); return err })
		if err != nil {
			return
		}
		if err = ValidateAttributeViewFieldSource(a); err != nil {
			return
		}
		if err = ValidateCellSource(a); err != nil {
			return
		}
		if a.ID != id || a.GetBlockKeyValues() == nil {
			return nil, ErrInvalidAVOrderSource
		}
		for _, v := range a.Views {
			for _, g := range v.Groups {
				if g == nil {
					return nil, ErrInvalidAVOrderSource
				}
			}
		}
		a.RenderedViewables = map[string]av.Viewable{}
		return
	}
	c.Stage = func(a *av.AttributeView) error { return guard(func() error { return e.Cell.Stage(a) }) }
	c.Groups = func(a *av.AttributeView) error { return guard(func() error { return e.Cell.Groups(a) }) }
	c.Related = func(id string) error { return guard(func() error { return e.Cell.Related(id) }) }
	c.Attrs = func(n *ast.Node, t *parse.Tree, a map[string]string) error {
		return guard(func() error { return e.Cell.Attrs(n, t, a) })
	}
	c.Node = func(id string) (n *ast.Node, t *parse.Tree, err error) {
		err = guard(func() error { n, t, err = e.Cell.Node(id); return err })
		return
	}
	c.Names = func(ids string) (s string, err error) {
		err = guard(func() error { s, err = e.Cell.Names(ids); return err })
		return
	}
	local.Text = func(n *ast.Node) (i, s string, err error) {
		err = guard(func() error { i, s, err = e.Text(n); return err })
		return
	}
	local.OptionalNode = func(id string) (n *ast.Node, t *parse.Tree, err error) {
		err = guard(func() error { n, t, err = e.OptionalNode(id); return err })
		return
	}
	local.Exists = func(id string) (yes bool, err error) {
		err = guard(func() error { yes, err = e.Exists(id); return err })
		return
	}
	local.Duplicate = func(a *av.AttributeView) error { return guard(func() error { return e.Duplicate(a) }) }
	local.History = func(a *av.AttributeView) error { return guard(func() error { return e.History(a) }) }
	local.View = func(a *av.AttributeView, id string) (v *av.View, err error) {
		err = guard(func() error { v, err = e.View(a, id); return err })
		if err == nil && v == nil {
			err = av.ErrViewNotFound
		}
		return
	}
	if e.Defaults != nil {
		d := *e.Defaults
		d.Ctx = ctx
		local.Defaults = &d
		d.Near = func(a *av.AttributeView, v, g *av.View, p string) (item av.Item, err error) {
			err = guard(func() error { item, err = e.Defaults.Near(a, v, g, p); return err })
			return
		}
		d.TemplateKeys = func(a *av.AttributeView, k *av.Key) (keys []*av.Key, err error) {
			err = guard(func() error { keys, err = e.Defaults.TemplateKeys(a, k); return err })
			return
		}
		d.Reverse = func(a *av.AttributeView, k *av.Key, v *av.Value) error {
			return guard(func() error { return e.Defaults.Reverse(a, k, v) })
		}
	}
	err := func() error {
		switch op.Action {
		case "insertAttrViewBlock":
			now := c.Now().UnixMilli()

			for i := len(op.Srcs) - 1; i >= 0; i-- {
				s := op.Srcs[i]
				d := s["isDetached"].(bool)
				id, _ := s["itemID"].(string)
				if id == "" {
					id = c.NewID()
				}
				bound := ""
				var tree *parse.Tree
				if !d {
					bound = s["id"].(string)
					if !ast.IsNodeIDPattern(bound) {
						continue
					}
					n, t, err := c.Node(bound)
					if err != nil {
						return err
					}
					if n == nil || t == nil {
						return ErrBlockNotFound
					}
					tree = t
				}
				content, _ := s["content"].(string)
				if err := AddAttributeViewBlockWithEnvironment(now, op.AvID, op.BlockID, op.ViewID, op.GroupID, op.PreviousID, id, bound, content, d, op.IgnoreDefaultFill, tree, &local); err != nil {
					return err
				}
			}
		case "removeAttrViewBlock":
			return RemoveAttributeViewBlockWithEnvironment(op.SrcIDs, op.AvID, &local)
		case "replaceAttrViewBlock":
			a, err := c.Read(op.AvID)
			if err != nil {
				return err
			}
			if err = ReplaceAttributeViewBlockWithEnvironment(a, op.PreviousID, op.NextID, op.IsDetached, &local); err != nil {
				return err
			}
			return c.Stage(a)
		}
		return ctx.Err()
	}()
	if err != nil {
		return err
	}
	if e.Cell.Complete != nil {
		return e.Cell.Complete()
	}
	return nil
}

func AddAttributeViewBlockWithEnvironment(now int64, avID, dbBlockID, viewID, groupID, previousItemID, addingItemID, addingBoundBlockID, addingBlockContent string, isDetached, ignoreDefaultFill bool, tree *parse.Tree, env *AttributeViewRowEnvironment) (err error) {
	var node *ast.Node
	if !isDetached {
		node = treenode.GetNodeInTree(tree, addingBoundBlockID)
		if nil == node {
			err = ErrBlockNotFound
			return
		}
	} else {
		if "" == addingItemID {
			addingItemID = env.Cell.NewID()
			env.
				Report("warn", "detached block id is empty, generate a new one [%s]", addingItemID)
		}
	}

	attrView, err := env.Cell.Read(avID)
	if err != nil {
		return
	}

	var blockIcon string
	if !isDetached {
		blockIcon, addingBlockContent, err = env.Text(node)
		if err != nil {
			return err
		}
		addingBlockContent = util.UnescapeHTML(addingBlockContent)
	}

	blockValues := attrView.GetBlockKeyValues()
	for _, blockValue := range blockValues.Values {
		if "" != addingBoundBlockID && blockValue.Block.ID == addingBoundBlockID {
			if !isDetached {

				if err = env.Cell.Binding(avID, node.ID, true); err != nil {
					return err
				}
				blockValue.IsDetached = isDetached
				blockValue.Block.Icon = blockIcon
				blockValue.Block.Content = addingBlockContent
				blockValue.UpdatedAt = now
				err = env.Cell.Stage(attrView)
			}

			if err == nil {
				err = env.Duplicate(attrView)
			}
			return
		}
	}

	blockValue := &av.Value{
		ID:         env.Cell.NewID(),
		KeyID:      blockValues.Key.ID,
		BlockID:    addingItemID,
		Type:       av.KeyTypeBlock,
		IsDetached: isDetached,
		CreatedAt:  now,
		UpdatedAt:  now,
		Block:      &av.ValueBlock{Icon: blockIcon, Content: addingBlockContent, Created: now, Updated: now}}
	if !isDetached {
		blockValue.Block.ID = addingBoundBlockID
	}

	blockValues.Values = append(blockValues.Values, blockValue)

	view, err := env.View(attrView, dbBlockID)
	if nil != err {
		env.
			Report("error", "get view by block ID [%s] failed: %s", dbBlockID, err)
		return
	}

	if "" != viewID {
		view = attrView.GetView(viewID)
		if nil == view {
			env.
				Report("error", "get view by view ID [%s] failed", viewID)
			return av.ErrViewNotFound
		}
	}

	groupView := view
	if "" != groupID {
		groupView = view.GetGroupByID(groupID)
		if groupView == nil {
			return av.ErrViewNotFound
		}
	}

	if !ignoreDefaultFill {
		if err = FillAttributeViewDefaultValues(attrView, view, groupView, previousItemID, addingItemID, true, env.Defaults); err != nil {
			return err
		}
	}

	for _, keyValues := range attrView.KeyValues {
		if av.KeyTypeDate == keyValues.Key.Type && nil != keyValues.Key.Date && keyValues.Key.Date.AutoFillNow {
			val := keyValues.GetValue(addingItemID)
			if nil == val {
				dateVal := &av.Value{
					ID: env.Cell.NewID(), KeyID: keyValues.Key.ID, BlockID: addingItemID, Type: av.KeyTypeDate, IsDetached: isDetached, CreatedAt: now, UpdatedAt: now + 1000,
					Date: &av.ValueDate{Content: now, IsNotEmpty: true, IsNotTime: !keyValues.Key.Date.FillSpecificTime},
				}
				keyValues.Values = append(keyValues.Values, dateVal)
			} else {
				if val.IsRenderAutoFill {
					if val.Date == nil {
						return ErrInvalidAVOrderSource
					}
					val.CreatedAt, val.UpdatedAt = now, now+1000
					val.Date.Content, val.Date.IsNotEmpty, val.Date.IsNotTime = now, true, !keyValues.Key.Date.FillSpecificTime
					val.IsRenderAutoFill = false
				}
			}
		}
	}

	if !isDetached {
		if err = env.Cell.Binding(avID, node.ID, true); err != nil {
			return err
		}
	}

	for _, v := range attrView.Views {
		if "" != previousItemID {
			changed := false
			for i, id := range v.ItemIDs {
				if id == previousItemID {
					v.ItemIDs = append(v.ItemIDs[:i+1], append([]string{addingItemID}, v.ItemIDs[i+1:]...)...)
					changed = true
					break
				}
			}
			if !changed {
				v.ItemIDs = append(v.ItemIDs, addingItemID)
			}
		} else {
			v.ItemIDs = append([]string{addingItemID}, v.ItemIDs...)
		}

		for _, g := range v.Groups {
			if "" != previousItemID {
				changed := false
				for i, id := range g.GroupItemIDs {
					if id == previousItemID {
						g.GroupItemIDs = append(g.GroupItemIDs[:i+1], append([]string{addingItemID}, g.GroupItemIDs[i+1:]...)...)
						changed = true
						break
					}
				}
				if !changed {
					g.GroupItemIDs = append(g.GroupItemIDs, addingItemID)
				}
			} else {
				g.GroupItemIDs = append([]string{addingItemID}, g.GroupItemIDs...)
			}
		}
	}

	if err = env.Cell.Groups(attrView); err != nil {
		return err
	}
	err = env.Cell.Stage(attrView)
	return
}

func ReplaceAttributeViewBlockWithEnvironment(attrView *av.AttributeView, oldBlockID, newNodeID string, isDetached bool, env *AttributeViewRowEnvironment) (err error) {
	avID := attrView.ID
	var tree *parse.Tree
	var node *ast.Node
	if !isDetached {
		node, tree, err = env.Cell.Node(newNodeID)
		if err != nil {
			return err
		}
		if node == nil || tree == nil {
			return ErrBlockNotFound
		}
	}

	now := env.Cell.Now().UnixMilli()

	for _, blockVal := range attrView.GetBlockKeyValues().Values {
		if !isDetached && blockVal.Block.ID == newNodeID && nil != node && nil != tree {
			if err = env.Cell.Binding(avID, node.ID, true); err != nil {
				return err
			}
			blockVal.IsDetached = false
			icon, content, textErr := env.Text(node)
			if textErr != nil {
				return textErr
			}
			content = util.UnescapeHTML(content)
			blockVal.Block.Icon, blockVal.Block.Content = icon, content
			blockVal.UpdatedAt = now
			if err = env.Cell.Groups(attrView); err != nil {
				return err
			}
			return
		}
	}

	for _, blockVal := range attrView.GetBlockKeyValues().Values {
		if blockVal.BlockID != oldBlockID {
			continue
		}

		if av.KeyTypeBlock == blockVal.Type {
			blockVal.IsDetached = isDetached
			if !isDetached {
				if "" != blockVal.Block.ID && blockVal.Block.ID != newNodeID {
					if err = env.Cell.Binding(avID, blockVal.Block.ID, false); err != nil {
						return err
					}
				}
				if err = env.Cell.Binding(avID, newNodeID, true); err != nil {
					return err
				}

				blockVal.Block.ID = newNodeID
				icon, content, textErr := env.Text(node)
				if textErr != nil {
					return textErr
				}
				content = util.UnescapeHTML(content)
				blockVal.Block.Icon, blockVal.Block.Content = icon, content

				if err = env.Cell.Related(avID); err != nil {
					return err
				}
			} else {
				blockVal.Block.ID = ""
			}
		}
	}

	if err = env.Cell.Groups(attrView); err != nil {
		return err
	}
	return
}

func RemoveAttributeViewBlockWithEnvironment(srcIDs []string, avID string, env *AttributeViewRowEnvironment) (err error) {
	attrView, err := env.Cell.Read(avID)
	if err != nil {
		return
	}

	for _, keyValues := range attrView.KeyValues {
		tmp := keyValues.Values[:0]
		for i, val := range keyValues.Values {
			if !gulu.Str.Contains(val.BlockID, srcIDs) {
				tmp = append(tmp, keyValues.Values[i])
			} else {

				if !val.IsDetached && nil != val.Block {
					node, tree, loadErr := env.OptionalNode(val.Block.ID)
					if loadErr != nil {
						return loadErr
					}
					if node != nil && tree != nil {
						if err = RemoveNodeAvIDWithEnvironment(node, avID, tree, env); err != nil {
							return err
						}
					}
				}
			}
		}
		keyValues.Values = tmp
	}

	for _, view := range attrView.Views {
		for _, blockID := range srcIDs {
			view.ItemIDs = gulu.Str.RemoveElem(view.ItemIDs, blockID)
		}
	}

	if err = env.Cell.Groups(attrView); err != nil {
		return err
	}

	err = env.Cell.Stage(attrView)
	if nil != err {
		return
	}

	if err = env.Cell.Related(avID); err != nil {
		return err
	}

	return env.History(attrView)
}

func RemoveNodeAvIDWithEnvironment(node *ast.Node, avID string, tree *parse.Tree, env *AttributeViewRowEnvironment) (err error) {
	attrs := parse.IAL2Map(node.KramdownIAL)
	if ast.NodeDocument == node.Type {
		delete(attrs, "custom-hidden")
		node.RemoveIALAttr("custom-hidden")
	}

	if avs := attrs[av.NodeAttrNameAvs]; "" != avs {
		avIDs := strings.Split(avs, ",")
		avIDs = gulu.Str.RemoveElem(avIDs, avID)
		var existAvIDs []string
		for _, attributeViewID := range avIDs {
			exists, existsErr := env.Exists(attributeViewID)
			if existsErr != nil {
				return existsErr
			}
			if exists {
				existAvIDs = append(existAvIDs, attributeViewID)
			}
		}
		avIDs = existAvIDs

		if 0 == len(avIDs) {
			attrs[av.NodeAttrNameAvs] = ""
		} else {
			attrs[av.NodeAttrNameAvs] = strings.Join(avIDs, ",")
			node.SetIALAttr(av.NodeAttrNameAvs, strings.Join(avIDs, ","))
			avNames, nameErr := env.Cell.Names(node.IALAttr(av.NodeAttrNameAvs))
			if nameErr != nil {
				return nameErr
			}
			attrs[av.NodeAttrViewNames] = avNames
		}
	}

	return env.Cell.Attrs(node, tree, attrs)
}

func ValidateAttributeViewSettings(op *Operation) error {
	switch op.Action {
	case "setAttrViewFilters", "setAttrViewSorts":
		if _, ok := op.Data.([]interface{}); !ok {
			return errors.New("invalid_av_view_rules")
		}
	case "setAttrViewPageSize":
		n, ok := op.Data.(float64)
		bound := math.Ldexp(1, strconv.IntSize-1)
		if !ok || math.IsNaN(n) || n < -bound || n >= bound {
			return errors.New("invalid_av_page_size")
		}
	default:
		return errors.New("invalid_av_view_operation")
	}
	return nil
}

// Both legacy wrappers and original transaction receivers use the same algorithm.
func SetAttributeViewSettings(ctx context.Context, op *Operation, e *AttributeViewSettingsEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if op == nil || e == nil || e.Read == nil || e.View == nil || e.Stage == nil {
		return errors.New("transaction_av_view_dependencies_unbound")
	}
	if err := ValidateAttributeViewSettings(op); err != nil {
		return err
	}
	a, err := e.Read(op.AvID)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if a == nil {
		return errors.New("invalid_av_view_source")
	}
	v, err := e.View(a, op.BlockID)
	if err != nil {
		return err
	}
	if v == nil {
		return errors.New("invalid_av_view")
	}
	if op.Action == "setAttrViewPageSize" {
		v.PageSize = int(op.Data.(float64))
	} else {
		data, err := gulu.JSON.MarshalJSON(op.Data.([]interface{}))
		if err != nil {
			return err
		}
		if op.Action == "setAttrViewFilters" {
			err = gulu.JSON.UnmarshalJSON(data, &v.Filters)
		} else {
			err = gulu.JSON.UnmarshalJSON(data, &v.Sorts)
		}
		if err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = e.Stage(a); err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return ctx.Err()
}

func (e *AttributeViewTextEnvironment) Validate() error {
	if e == nil || e.Ctx == nil || e.Read == nil || e.Clone == nil || e.Text == nil || e.Stage == nil || e.Reload == nil || e.Related == nil {
		return errors.New("av_text_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func (e *AttributeViewTextEnvironment) Guard(fn func() error) error {
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	if e.JobContext != nil {
		if err := e.JobContext.Err(); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	if e.JobContext != nil {
		if err := e.JobContext.Err(); err != nil {
			return err
		}
	}
	return e.Ctx.Err()
}

// Native writes/notifications are candidates; the source commit remains R04.
func UpdateAttributeViewBlockTextWithEnvironment(nodes map[string]*ast.Node, e *AttributeViewTextEnvironment) error {
	if err := e.Validate(); err != nil {
		return err
	}
	for _, n := range nodes {
		if n == nil || (!e.Legacy && n.ID == "") {
			return errors.New("invalid_av_text_node")
		}
	}
	var parents []*ast.Node
	for _, n := range nodes {
		for p := n.Parent; p != nil && p.Type != ast.NodeDocument; p = p.Parent {
			parents = append(parents, p)
		}
	}
	for _, p := range parents {
		nodes[p.ID] = p
	}
	for _, n := range nodes {
		if err := e.Ctx.Err(); err != nil {
			return err
		}
		ids := n.IALAttr(av.NodeAttrNameAvs)
		if ids == "" {
			continue
		}
		for _, id := range strings.Split(ids, ",") {
			var a *av.AttributeView
			var source *av.AttributeView
			if err := e.Guard(func() (err error) { source, err = e.Read(id); return }); err != nil {
				if e.Legacy && e.Ctx.Err() == nil {
					continue
				}
				return err
			}
			if source == nil || source.ID != id {
				return errors.New("invalid_av_text_source")
			}
			if err := ValidateUpdatedAttributeView(source); err != nil {
				return err
			}
			if source.GetBlockKeyValues() == nil {
				if e.Legacy {
					continue
				}
				return errors.New("av_text_primary_missing")
			}
			if err := e.Guard(func() (err error) { a, err = e.Clone(source); return }); err != nil {
				return err
			}
			if a == nil || a == source || a.ID != id {
				return errors.New("invalid_av_text_clone")
			}
			if err := ValidateUpdatedAttributeView(a); err != nil {
				return err
			}
			if a.GetBlockKeyValues() == nil {
				return errors.New("av_text_primary_missing")
			}
			changed := false
			for _, value := range a.GetBlockKeyValues().Values {
				if value.Block.ID != n.ID {
					continue
				}
				var icon, text string
				if err := e.Guard(func() (err error) { icon, text, err = e.Text(n, id); return }); err != nil {
					return err
				}
				if icon != value.Block.Icon {
					value.Block.Icon = icon
					changed = true
				}
				if text != value.Block.Content {
					value.Block.Content = util.UnescapeHTML(text)
					changed = true
				}
				break
			}
			if !changed {
				continue
			}
			if err := e.Guard(func() error { return e.Stage(a) }); err != nil {
				if !e.Legacy || e.Ctx.Err() != nil {
					return err
				}
			}
			if err := e.Guard(func() error { return e.Reload(id) }); err != nil {
				return err
			}
			if err := e.Guard(func() error { return e.Related(id) }); err != nil {
				return err
			}
		}
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return e.Ctx.Err()
}

func (e *AttributeViewUpdateEnvironment) Validate(update bool) error {
	if e == nil || e.Ctx == nil || e.Read == nil || e.Upsert == nil || e.Sources == nil || e.Groups == nil || e.Stage == nil || e.Reload == nil || (update && (e.Attrs == nil || e.Tree == nil)) {
		return errors.New("av_update_dependencies_unbound")
	}
	return e.Ctx.Err()
}

func (e *AttributeViewUpdateEnvironment) Guard(fn func() error) error {
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.Ctx.Err()
}

func UpdateAttributeViewWithEnvironment(node *ast.Node, tree *parse.Tree, e *AttributeViewUpdateEnvironment) error {
	if err := e.Validate(true); err != nil {
		return err
	}
	if node == nil || tree == nil || tree.Root == nil || node.ID == "" || tree.ID == "" {
		return errors.New("invalid_av_update_tree")
	}
	if !e.Legacy && (tree.Root.ID != tree.ID || tree.Box == "" || tree.Path == "" || treenode.GetNodeInTree(tree, node.ID) != node) {
		return errors.New("invalid_av_update_tree")
	}
	if err := RegisterUpdatedAttributeViews(node, e); err != nil {
		return err
	}
	if node.Type != ast.NodeAttributeView {
		return e.Ctx.Err()
	}
	var a *av.AttributeView
	if err := e.Guard(func() (err error) { a, err = e.Read(node.AttributeViewID); return }); err != nil {
		if e.Legacy && e.Ctx.Err() == nil {
			return nil
		}
		return err
	}
	if a == nil {
		if e.Legacy {
			return nil
		}
		return errors.New("invalid_av_update_source")
	}
	if a.ID != node.AttributeViewID {
		return errors.New("invalid_av_update_source")
	}
	if err := ValidateUpdatedAttributeView(a); err != nil {
		return err
	}

	view := a.GetView(a.ViewID)
	if view == nil {
		return e.Ctx.Err()
	}
	node.AttributeViewType = string(view.LayoutType)
	attrs := parse.IAL2Map(node.KramdownIAL)
	if attrs[av.NodeAttrView] == "" {
		attrs[av.NodeAttrView] = view.ID
		if err := e.Guard(func() error { return e.Attrs(node, tree, attrs) }); err != nil {
			return err
		}
	}

	return e.Guard(func() error { return e.Tree(tree) })
}

func RegisterUpdatedAttributeViews(node *ast.Node, e *AttributeViewUpdateEnvironment) error {
	if err := e.Validate(false); err != nil {
		return err
	}
	if node == nil {
		return errors.New("invalid_av_update_node")
	}
	var ids []string
	var walkErr error
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || n.Type != ast.NodeAttributeView {
			return ast.WalkContinue
		}
		if !e.Legacy && (n.ID == "" || n.AttributeViewID == "") {
			walkErr = errors.New("invalid_av_update_mirror")
			return ast.WalkStop
		}
		var changed bool
		walkErr = e.Guard(func() (err error) { changed, err = e.Upsert(n.AttributeViewID, n.ID); return })
		if walkErr != nil {
			return ast.WalkStop
		}
		if changed {
			ids = append(ids, n.AttributeViewID)
		}
		return ast.WalkContinue
	})
	if walkErr != nil {
		return walkErr
	}

	for current := node; current != nil; current = current.Parent {
		if err := e.Ctx.Err(); err != nil {
			return err
		}
		if current != node && current.Type == ast.NodeDocument {
			break
		}
		ast.Walk(current, func(n *ast.Node, entering bool) ast.WalkStatus {
			if entering {
				if members := n.IALAttr(av.NodeAttrNameAvs); members != "" {
					ids = append(ids, strings.Split(members, ",")...)
				}
			}
			return ast.WalkContinue
		})
	}
	ids = UniqueUpdatedAttributeViews(ids)
	if e.Legacy {
		if e.AsyncRefresh == nil {
			return errors.New("AV refresh owner unbound")
		}
		return e.AsyncRefresh(ids)
	}
	return RefreshUpdatedAttributeViews(ids, e)
}

func UniqueUpdatedAttributeViews(ids []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func RefreshUpdatedAttributeViews(ids []string, e *AttributeViewUpdateEnvironment) error {

	all := append([]string(nil), ids...)
	for _, id := range ids {
		var sources []string
		if err := e.Guard(func() (err error) { sources, err = e.Sources(id); return }); err != nil {
			return err
		}
		all = append(all, sources...)
	}
	for _, id := range UniqueUpdatedAttributeViews(all) {
		var a *av.AttributeView
		err := e.Guard(func() (err error) { a, err = e.Read(id); return })
		if err != nil && (!e.Legacy || e.StrictDependencies || e.Ctx.Err() != nil) {
			return err
		}
		if a != nil {
			if a.ID != id {
				return errors.New("invalid_av_update_source")
			}
			if err := ValidateUpdatedAttributeView(a); err != nil {
				return err
			}
			if err := e.Guard(func() error { return e.Groups(a) }); err != nil {
				if !e.Legacy || e.StrictDependencies {
					return err
				}
			}
			if err := e.Guard(func() error { return e.Stage(a) }); err != nil {
				if !e.Legacy || e.StrictDependencies {
					return err
				}
			}
		} else if !e.Legacy || e.StrictDependencies {
			return errors.New("invalid_av_update_source")
		}

		if err := e.Guard(func() error { return e.Reload(id) }); err != nil {
			return err
		}
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return e.Ctx.Err()
}

func ValidateUpdatedAttributeView(a *av.AttributeView) error {
	if err := ValidateCellSource(a); err != nil {
		return err
	}
	for _, view := range a.Views {
		if view == nil {
			return errors.New("invalid_av_update_view")
		}
	}
	return nil
}

func (e *AttributeViewBoundEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewManagementEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewOptionEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewRowEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewNameEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewUpdateEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewCellEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewSettingsEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewDefaultEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewFieldEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewOrderEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewBindingEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
func (e *AttributeViewTextEnvironment) Report(level, format string, args ...any) {
	if e != nil && e.Diagnostic != nil {
		e.Diagnostic(level, format, args...)
	}
}
