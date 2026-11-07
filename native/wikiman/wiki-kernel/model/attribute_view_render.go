// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/sql"
)

func RenderAttributeView(blockID, avID, viewID, query string, page, pageSize int, groupPaging map[string]interface{}) (viewable av.Viewable, attrView *av.AttributeView, err error) {
	return RenderAttributeViewContext(context.Background(), blockID, avID, viewID, query, page, pageSize, groupPaging)
}

func RenderAttributeViewContext(ctx context.Context, blockID, avID, viewID, query string, page, pageSize int, groupPaging map[string]interface{}) (viewable av.Viewable, attrView *av.AttributeView, err error) {
	type result struct {
		view      av.Viewable
		attribute *av.AttributeView
	}
	out, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (result, error) {
		if e := b.waitForStorages(); e != nil {
			return result{}, e
		}
		a, e := b.initialize(avID)
		if e != nil {
			return result{}, e
		}
		v, e := renderAttributeView(a, blockID, viewID, query, page, pageSize, groupPaging, b)
		return result{v, a}, e
	})
	return out.view, out.attribute, e
}

const (
	groupValueDefault                                        = "_@default@_"    // 默认分组值（值为空的默认分组）
	groupValueNotInRange                                     = "_@notInRange@_" // 不再范围内的分组值（只有数字类型的分组才可能是该值）
	groupValueLast30Days, groupValueLast7Days                = "_@last30Days@_", "_@last7Days@_"
	groupValueYesterday, groupValueToday, groupValueTomorrow = "_@yesterday@_", "_@today@_", "_@tomorrow@_"
	groupValueNext7Days, groupValueNext30Days                = "_@next7Days@_", "_@next30Days@_"
)

func renderAttributeView(attrView *av.AttributeView, nodeID, viewID, query string, page, pageSize int, groupPaging map[string]interface{}, bindings ...*legacyAVBinding) (viewable av.Viewable, err error) {
	var b *legacyAVBinding
	if len(bindings) > 0 {
		b = bindings[0]
	}
	var view *av.View
	if b != nil {
		view, err = b.selectView(attrView, viewID, nodeID)
	} else {
		view, err = getRenderAttributeViewView(attrView, viewID, nodeID)
	}
	if err != nil {
		return
	}
	if b != nil {
		err = b.prepareView(attrView, view)
		if err != nil {
			return
		}
	} else {
		checkAttrView(attrView, view)
		upgradeAttributeViewSpec(attrView)
	}

	if b != nil {
		if err = b.adopt(attrView); err != nil {
			return
		}
		viewable, err = b.scope.render(attrView, view, query)
		if err != nil {
			return
		}
		err = b.process(viewable, view, attrView, page, pageSize)
	} else {
		viewable = sql.RenderView(attrView, view, query)
		err = renderViewableInstance(viewable, view, attrView, page, pageSize)
	}
	if nil != err {
		return
	}

	// 渲染分组视图
	err = renderAttributeViewGroups(viewable, attrView, view, query, page, pageSize, groupPaging, b)
	return
}

func renderAttributeViewGroups(viewable av.Viewable, attrView *av.AttributeView, view *av.View, query string, page, pageSize int, groupPaging map[string]interface{}, bindings ...*legacyAVBinding) (err error) {
	var b *legacyAVBinding
	if len(bindings) > 0 {
		b = bindings[0]
	}
	generate := func() error {
		if b != nil {
			return b.group(attrView, view)
		}
		genAttrViewGroups(view, attrView)
		return nil
	}
	save := func() error {
		if b != nil {
			return b.save(attrView)
		}
		return av.SaveAttributeView(attrView)
	}
	groupKey := view.GetGroupKey(attrView)
	if nil == groupKey {
		if view.LayoutType == av.LayoutTypeKanban {
			preferredGroupKey := getKanbanPreferredGroupKey(attrView)
			group := &av.ViewGroup{Field: preferredGroupKey.ID}
			if b != nil {
				e := managementWithBinding(nil, b, b.scope.ctx)
				e.groups = b.groups
				e.count = b.count
				err = setAttributeViewGroupWithEnvironment(attrView, view, group, e)
			} else {
				setAttributeViewGroup(attrView, view, group)
			}
			if err != nil {
				return
			}
			if err = save(); err != nil {
				return
			}
			groupKey = view.GetGroupKey(attrView)
		} else {
			return
		}
	}

	// 当前日期可能会变，所以如果是按日期分组则需要重新生成分组
	if isGroupByDate(view) {
		createdDate := time.UnixMilli(view.GroupCreated).Format("2006-01-02")
		if time.Now().Format("2006-01-02") != createdDate {
			if err = generate(); err != nil {
				return
			}
			if err = save(); err != nil {
				return
			}
		}
	}

	// 如果是按模板分组则需要重新生成分组
	if isGroupByTemplate(attrView, view) {
		if err = generate(); err != nil {
			return
		}
		if err = save(); err != nil {
			return
		}
	}

	// 渲染分组视图
	if nil == view.Groups {
		if err = generate(); err != nil {
			return
		}
		if err = save(); err != nil {
			return
		}
	}

	if err = avrender.LabelGroups(view, groupKey, func(id int) (string, error) {
		if b != nil {
			if e := b.guard(); e != nil {
				return "", e
			}
			return b.scope.commit.avLanguage(id), nil
		}
		return Conf.language(id), nil
	}); err != nil {
		return err
	}

	if b != nil {
		avrender.SortGroups(attrView, view, b.scope.now)
	} else {
		sortGroupViews(attrView, view)
	}

	var groups []av.Viewable
	for _, groupView := range view.Groups {
		var groupViewable av.Viewable
		if b != nil {
			groupViewable, err = b.groupRender(attrView, view, groupView, query)
			if err != nil {
				return
			}
		} else {
			groupViewable = sql.RenderGroupView(attrView, view, groupView, query)
		}

		groupPage, groupPageSize := page, pageSize
		if nil != groupPaging {
			if paging := groupPaging[groupView.ID]; nil != paging {
				pagingMap := paging.(map[string]interface{})
				if nil != pagingMap["page"] {
					groupPage = int(pagingMap["page"].(float64))
				}
				if nil != pagingMap["pageSize"] {
					groupPageSize = int(pagingMap["pageSize"].(float64))
				}
			}
		}

		if b != nil {
			err = b.process(groupViewable, view, attrView, groupPage, groupPageSize)
		} else {
			err = renderViewableInstance(groupViewable, view, attrView, groupPage, groupPageSize)
		}
		if nil != err {
			return
		}

		hideEmptyGroupViews(view, groupViewable)
		groups = append(groups, groupViewable)

		// 将分组视图的分组字段清空，减少冗余（字段信息可以在总的视图 view 对象上获取到）
		switch groupView.LayoutType {
		case av.LayoutTypeTable:
			groupView.Table.Columns = nil
		case av.LayoutTypeGallery:
			groupView.Gallery.CardFields = nil
		case av.LayoutTypeKanban:
			groupView.Kanban.Fields = nil
		}
	}
	viewable.SetGroups(groups)

	// 将总的视图上的项目清空，减少冗余
	viewable.(av.Collection).SetItems(nil)
	return
}

func hideEmptyGroupViews(v *av.View, b av.Viewable) { avrender.HideEmptyGroups(v, b) }

func sortGroupViews(a *av.AttributeView, v *av.View) { avrender.SortGroups(a, v, time.Now()) }

func sortGroupsBySelectOption(v *av.View, k *av.Key) { editcore.SortGroupsBySelectOption(v, k) }

func isGroupByDate(v *av.View) bool { return avrender.IsGroupByDate(v) }

func isGroupByTemplate(attrView *av.AttributeView, view *av.View) bool {
	if !view.IsGroupView() {
		return false
	}

	groupKey := view.GetGroupKey(attrView)
	if nil == groupKey {
		return false
	}
	return av.KeyTypeTemplate == groupKey.Type
}

func renderViewableInstance(viewable av.Viewable, view *av.View, attrView *av.AttributeView, page, pageSize int) (err error) {
	if nil == viewable {
		err = av.ErrViewNotFound
		logging.LogErrorf("render attribute view [%s] failed", attrView.ID)
		return
	}

	cachedAttrViews := map[string]*av.AttributeView{}
	rollupFurtherCollections := sql.GetFurtherCollections(attrView, cachedAttrViews)
	av.Filter(viewable, attrView, rollupFurtherCollections, cachedAttrViews)
	av.Sort(viewable, attrView)
	av.Calc(viewable, attrView)

	return paginateAttributeView(viewable, view, page, pageSize)
}

func paginateAttributeView(viewable av.Viewable, view *av.View, page, pageSize int) (err error) {
	// 分页
	switch viewable.GetType() {
	case av.LayoutTypeTable:
		table := viewable.(*av.Table)
		table.RowCount = len(table.Rows)
		table.PageSize = view.PageSize
		if 1 > pageSize {
			pageSize = table.PageSize
		}
		start := (page - 1) * pageSize
		end := start + pageSize
		if len(table.Rows) < end {
			end = len(table.Rows)
		}
		table.Rows = table.Rows[start:end]
	case av.LayoutTypeGallery:
		gallery := viewable.(*av.Gallery)
		gallery.CardCount = len(gallery.Cards)
		gallery.PageSize = view.PageSize
		if 1 > pageSize {
			pageSize = gallery.PageSize
		}
		start := (page - 1) * pageSize
		end := start + pageSize
		if len(gallery.Cards) < end {
			end = len(gallery.Cards)
		}
		gallery.Cards = gallery.Cards[start:end]
	case av.LayoutTypeKanban:
		kanban := viewable.(*av.Kanban)
		kanban.CardCount = len(kanban.Cards)
		kanban.PageSize = view.PageSize
		if 1 > pageSize {
			pageSize = kanban.PageSize
		}
		start := (page - 1) * pageSize
		end := start + pageSize
		if len(kanban.Cards) < end {
			end = len(kanban.Cards)
		}
		kanban.Cards = kanban.Cards[start:end]
	}
	return
}

func getRenderAttributeViewView(attrView *av.AttributeView, viewID, nodeID string) (*av.View, error) {
	return selectBlockAttributeView(attrView, viewID, nodeID, legacyAttributeViewBlockKeysEnvironment())
}

func RenderRepoSnapshotAttributeView(index, id string) (av.Viewable, *av.AttributeView, error) {
	return RenderRepoSnapshotAttributeViewContext(context.Background(), index, id)
}
func RenderHistoryAttributeView(block, id, view, query string, page, size int, paging map[string]interface{}, created string) (av.Viewable, *av.AttributeView, error) {
	return RenderHistoryAttributeViewContext(context.Background(), block, id, view, query, page, size, paging, created)
}
