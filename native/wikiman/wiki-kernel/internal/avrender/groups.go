// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/textsort"
)

func GenerateGroups(view *av.View, attrView *av.AttributeView, env GroupEnvironment) error {
	if env.Render == nil || env.Now == nil || env.NewView == nil {
		return ErrEnvironmentUnbound
	}
	if view.Group != nil && view.Group.Method == av.GroupMethodRangeNum && (view.Group.Range == nil || view.Group.Range.NumStep <= 0 || math.IsNaN(view.Group.Range.NumStep) || math.IsInf(view.Group.Range.NumStep, 0)) {
		return ErrShape
	}
	if view.Group != nil && view.Group.Method == av.GroupMethodRangeNum {
		r := view.Group.Range
		if math.IsNaN(r.NumStart) || math.IsNaN(r.NumEnd) || math.IsInf(r.NumStart, 0) || math.IsInf(r.NumEnd, 0) || r.NumStart+r.NumStep <= r.NumStart || r.NumEnd+r.NumStep <= r.NumEnd {
			return ErrShape
		}
	}
	if !view.IsGroupView() {

		return nil
	}

	groupStates := GroupStates(view)

	group := view.Group
	view.Groups = nil
	viewable, err := env.Render(attrView, view, "")
	if err != nil {
		return err
	}
	if viewable == nil {
		return ErrShape
	}
	var items []av.Item
	for _, item := range viewable.(av.Collection).GetItems() {
		items = append(items, item)
	}

	groupKey := view.GetGroupKey(attrView)
	if nil == groupKey {

		return nil
	}

	var rangeStart, rangeEnd float64
	switch group.Method {
	case av.GroupMethodValue:
		if av.GroupOrderMan != group.Order {
			sort.SliceStable(items, func(i, j int) bool {
				return items[i].GetValue(group.Field).String(false) < items[j].GetValue(group.Field).String(false)
			})
		}
	case av.GroupMethodRangeNum:
		if nil == group.Range {

			return nil
		}

		rangeStart, rangeEnd = group.Range.NumStart, group.Range.NumStart+group.Range.NumStep
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].GetValue(group.Field).Number.Content < items[j].GetValue(group.Field).Number.Content
		})
	case av.GroupMethodDateDay, av.GroupMethodDateWeek, av.GroupMethodDateMonth, av.GroupMethodDateYear, av.GroupMethodDateRelative:
		if av.KeyTypeCreated == groupKey.Type {
			sort.SliceStable(items, func(i, j int) bool {
				return items[i].GetValue(group.Field).Created.Content < items[j].GetValue(group.Field).Created.Content
			})
		} else if av.KeyTypeUpdated == groupKey.Type {
			sort.SliceStable(items, func(i, j int) bool {
				return items[i].GetValue(group.Field).Updated.Content < items[j].GetValue(group.Field).Updated.Content
			})
		} else if av.KeyTypeDate == groupKey.Type {
			sort.SliceStable(items, func(i, j int) bool {
				return items[i].GetValue(group.Field).Date.Content < items[j].GetValue(group.Field).Date.Content
			})
		}
	}

	todayStart := env.Now()
	todayStart = time.Date(todayStart.Year(), todayStart.Month(), todayStart.Day(), 0, 0, 0, 0, todayStart.Location())

	var relationDestAv *av.AttributeView
	if av.KeyTypeRelation == groupKey.Type && nil != groupKey.Relation {
		if attrView.ID == groupKey.Relation.AvID {
			relationDestAv = attrView
		} else {
			if env.Read == nil {
				return ErrEnvironmentUnbound
			}
			var err error
			relationDestAv, err = env.Read(groupKey.Relation.AvID)
			if err != nil {
				return err
			}
		}
	}

	groupItemsMap := map[string][]av.Item{}
	for _, item := range items {
		value := item.GetValue(group.Field)
		if value.IsBlank() {
			groupItemsMap[groupValueDefault] = append(groupItemsMap[groupValueDefault], item)
			continue
		}

		var groupVal string
		switch group.Method {
		case av.GroupMethodValue:
			if av.KeyTypeSelect == groupKey.Type || av.KeyTypeMSelect == groupKey.Type {
				for _, s := range value.MSelect {
					groupItemsMap[s.Content] = append(groupItemsMap[s.Content], item)
				}
				continue
			} else if av.KeyTypeRelation == groupKey.Type {
				if nil == relationDestAv {
					continue
				}

				for _, bID := range value.Relation.BlockIDs {
					groupItemsMap[bID] = append(groupItemsMap[bID], item)
				}
				continue
			}

			groupVal = value.String(false)
		case av.GroupMethodRangeNum:
			if group.Range.NumStart > value.Number.Content || group.Range.NumEnd < value.Number.Content {
				groupVal = groupValueNotInRange
				break
			}

			for rangeEnd <= group.Range.NumEnd && rangeEnd <= value.Number.Content {
				rangeStart += group.Range.NumStep
				rangeEnd += group.Range.NumStep
			}

			if rangeStart <= value.Number.Content && rangeEnd > value.Number.Content {
				groupVal = fmt.Sprintf("%s - %s", strconv.FormatFloat(rangeStart, 'f', -1, 64), strconv.FormatFloat(rangeEnd, 'f', -1, 64))
			}
		case av.GroupMethodDateDay, av.GroupMethodDateWeek, av.GroupMethodDateMonth, av.GroupMethodDateYear, av.GroupMethodDateRelative:
			var contentTime time.Time
			switch value.Type {
			case av.KeyTypeDate:
				contentTime = time.UnixMilli(value.Date.Content)
			case av.KeyTypeCreated:
				contentTime = time.UnixMilli(value.Created.Content)
			case av.KeyTypeUpdated:
				contentTime = time.UnixMilli(value.Updated.Content)
			}
			switch group.Method {
			case av.GroupMethodDateDay:
				groupVal = contentTime.Format("2006-01-02")
			case av.GroupMethodDateWeek:
				year, week := contentTime.ISOWeek()
				groupVal = fmt.Sprintf("%d-W%02d", year, week)
			case av.GroupMethodDateMonth:
				groupVal = contentTime.Format("2006-01")
			case av.GroupMethodDateYear:
				groupVal = contentTime.Format("2006")
			case av.GroupMethodDateRelative:
				// 过去 30 天之前的按月分组
				// 过去 30 天、过去 7 天、昨天、今天、明天、未来 7 天、未来 30 天
				// 未来 30 天之后的按月分组
				if contentTime.Before(todayStart.AddDate(0, 0, -30)) {
					groupVal = contentTime.Format("2006-01") // 开头的数字用于排序
				} else if contentTime.Before(todayStart.AddDate(0, 0, -7)) {
					groupVal = groupValueLast30Days
				} else if contentTime.Before(todayStart.AddDate(0, 0, -1)) {
					groupVal = groupValueLast7Days
				} else if contentTime.Before(todayStart) {
					groupVal = groupValueYesterday
				} else if (contentTime.After(todayStart) || contentTime.Equal(todayStart)) && contentTime.Before(todayStart.AddDate(0, 0, 1)) {
					groupVal = groupValueToday
				} else if contentTime.After(todayStart.AddDate(0, 0, 30)) {
					groupVal = contentTime.Format("2006-01")
				} else if contentTime.After(todayStart.AddDate(0, 0, 7)) {
					groupVal = groupValueNext30Days
				} else if contentTime.Equal(todayStart.AddDate(0, 0, 2)) || contentTime.After(todayStart.AddDate(0, 0, 2)) {
					groupVal = groupValueNext7Days
				} else {
					groupVal = groupValueTomorrow
				}
			}
		}

		groupItemsMap[groupVal] = append(groupItemsMap[groupVal], item)
	}

	if av.KeyTypeSelect == groupKey.Type || av.KeyTypeMSelect == groupKey.Type {
		for _, o := range groupKey.Options {
			if _, ok := groupItemsMap[o.Name]; !ok {
				groupItemsMap[o.Name] = []av.Item{}
			}
		}
	}

	if av.KeyTypeCheckbox != groupKey.Type {
		if 1 > len(groupItemsMap[groupValueDefault]) {
			// 始终保留默认分组 https://github.com/siyuan-note/siyuan/issues/15587
			groupItemsMap[groupValueDefault] = []av.Item{}
		}
	} else {
		// 对于复选框分组，空白分组表示未选中状态，始终保留 https://github.com/siyuan-note/siyuan/issues/15650
		if nil == groupItemsMap[""] {
			groupItemsMap[""] = []av.Item{}
		}
		if nil == groupItemsMap[av.CheckboxCheckedStr] {
			groupItemsMap[av.CheckboxCheckedStr] = []av.Item{}
		}
	}

	if env.MaxGroups > 0 && len(groupItemsMap) > env.MaxGroups {
		return ErrLimit
	}
	for groupValue, groupItems := range groupItemsMap {
		v, err := env.NewView(view.LayoutType)
		if err != nil {
			return err
		}
		if v == nil {
			return ErrShape
		}

		v.GroupItemIDs = []string{}
		for _, item := range groupItems {
			v.GroupItemIDs = append(v.GroupItemIDs, item.GetID())
		}

		v.Name = ""       // 分组视图的名称在渲染时才填充
		v.GroupHidden = 1 // 默认隐藏空白分组
		v.GroupKey = groupKey
		v.GroupVal = &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: groupValue}}
		if av.KeyTypeSelect == groupKey.Type || av.KeyTypeMSelect == groupKey.Type {
			if opt := groupKey.GetOption(groupValue); nil != opt {
				v.GroupVal.Text = nil
				v.GroupVal.Type = av.KeyTypeSelect
				v.GroupVal.MSelect = []*av.ValueSelect{{Content: opt.Name, Color: opt.Color}}
			}
		} else if av.KeyTypeRelation == groupKey.Type {
			if relationDestAv != nil && groupValueDefault != groupValue {
				v.GroupVal.Text = nil
				v.GroupVal.Type = av.KeyTypeRelation
				v.GroupVal.Relation = &av.ValueRelation{BlockIDs: []string{groupValue}}

				if destBlock := relationDestAv.GetBlockValue(groupValue); nil != destBlock {
					v.GroupVal.Relation.Contents = []*av.Value{destBlock}
				}
			}
		} else if av.KeyTypeCheckbox == groupKey.Type {
			v.GroupVal.Text = nil
			v.GroupVal.Type = av.KeyTypeCheckbox
			v.GroupVal.Checkbox = &av.ValueCheckbox{}
			if "" != groupValue {
				v.GroupVal.Checkbox.Checked = true
			}
		}
		v.GroupSort = -1
		view.Groups = append(view.Groups, v)
	}

	view.GroupCreated = env.Now().UnixMilli()
	RestoreGroupStates(view, groupStates)
	return nil
}

type GroupState struct {
	ID      string
	Folded  bool
	Hidden  int
	Sort    int
	ItemIDs []string
}

func GroupStates(view *av.View) (groupStates map[string]*GroupState) {
	groupStates = map[string]*GroupState{}
	if !view.IsGroupView() {
		return
	}

	for _, groupView := range view.Groups {
		if av.LayoutTypeKanban == groupView.LayoutType {
			// 看板视图的分组不能折叠
			groupView.GroupFolded = false
		}

		groupStates[groupView.GetGroupValue()] = &GroupState{
			ID:      groupView.ID,
			Folded:  groupView.GroupFolded,
			Hidden:  groupView.GroupHidden,
			Sort:    groupView.GroupSort,
			ItemIDs: groupView.GroupItemIDs,
		}
	}
	return
}

func RestoreGroupStates(view *av.View, groupStates map[string]*GroupState) {
	for _, groupView := range view.Groups {
		if state, ok := groupStates[groupView.GetGroupValue()]; ok {
			groupView.ID = state.ID
			groupView.GroupFolded = state.Folded
			groupView.GroupHidden = state.Hidden
			groupView.GroupSort = state.Sort

			itemIDsSort := map[string]int{}
			for i, itemID := range state.ItemIDs {
				itemIDsSort[itemID] = i
			}

			sort.SliceStable(groupView.GroupItemIDs, func(i, j int) bool {
				return itemIDsSort[groupView.GroupItemIDs[i]] < itemIDsSort[groupView.GroupItemIDs[j]]
			})
		}
	}

	defaultGroup := view.GetGroupByGroupValue(groupValueDefault)
	if nil != defaultGroup {
		if -1 == defaultGroup.GroupSort {
			view.RemoveGroupByID(defaultGroup.ID)
		} else {
			defaultGroup = nil
		}
	}

	for i, groupView := range view.Groups {
		if i != groupView.GroupSort && -1 == groupView.GroupSort {
			groupView.GroupSort = i
		}
	}

	if nil != defaultGroup {
		view.Groups = append(view.Groups, defaultGroup)
		defaultGroup.GroupSort = len(view.Groups) - 1
	}
}

func HideEmptyGroups(view *av.View, viewable av.Viewable) {
	if !view.IsGroupView() {
		return
	}

	groupHidden := viewable.GetGroupHidden()
	if !view.Group.HideEmpty {
		if 2 != groupHidden {
			viewable.SetGroupHidden(0)
		}
		return
	}

	itemCount := viewable.(av.Collection).CountItems()
	if 1 == groupHidden && 0 < itemCount {
		viewable.SetGroupHidden(0)
	}
}

func SortGroups(attrView *av.AttributeView, view *av.View, now time.Time) {
	if av.GroupOrderMan == view.Group.Order {
		sort.Slice(view.Groups, func(i, j int) bool { return view.Groups[i].GroupSort < view.Groups[j].GroupSort })
		return
	}

	if av.GroupMethodDateRelative == view.Group.Method {
		var relativeDateGroups []*av.View
		var last30Days, last7Days, yesterday, today, tomorrow, next7Days, next30Days, defaultGroup *av.View
		for _, groupView := range view.Groups {
			_, err := time.Parse("2006-01", groupView.GetGroupValue())
			if nil == err { // 如果能解析出来说明是 30 天之前或 30 天之后的分组形式
				relativeDateGroups = append(relativeDateGroups, groupView)
			} else { // 否则是相对日期分组形式
				switch groupView.GetGroupValue() {
				case groupValueLast30Days:
					last30Days = groupView
				case groupValueLast7Days:
					last7Days = groupView
				case groupValueYesterday:
					yesterday = groupView
				case groupValueToday:
					today = groupView
				case groupValueTomorrow:
					tomorrow = groupView
				case groupValueNext7Days:
					next7Days = groupView
				case groupValueNext30Days:
					next30Days = groupView
				case groupValueDefault:
					defaultGroup = groupView
				}
			}
		}

		sort.SliceStable(relativeDateGroups, func(i, j int) bool {
			return relativeDateGroups[i].GetGroupValue() < relativeDateGroups[j].GetGroupValue()
		})

		var lastNext30Days []*av.View
		if nil != next30Days {
			lastNext30Days = append(lastNext30Days, next30Days)
		}
		if nil != next7Days {
			lastNext30Days = append(lastNext30Days, next7Days)
		}
		if nil != tomorrow {
			lastNext30Days = append(lastNext30Days, tomorrow)
		}
		if nil != today {
			lastNext30Days = append(lastNext30Days, today)
		}
		if nil != yesterday {
			lastNext30Days = append(lastNext30Days, yesterday)
		}

		if nil != last7Days {
			lastNext30Days = append(lastNext30Days, last7Days)
		}
		if nil != last30Days {
			lastNext30Days = append(lastNext30Days, last30Days)
		}

		startIdx := -1
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		thisMonth := todayStart.Format("2006-01")
		for i, monthGroup := range relativeDateGroups {
			if monthGroup.GetGroupValue() < thisMonth {
				startIdx = i + 1
			}
		}
		if -1 == startIdx {
			startIdx = 0
		}
		for _, g := range lastNext30Days {
			relativeDateGroups = slices.Insert(relativeDateGroups, startIdx, g)
		}

		if av.GroupOrderDesc == view.Group.Order {
			slices.Reverse(relativeDateGroups)
		}

		if nil != defaultGroup {
			relativeDateGroups = append(relativeDateGroups, defaultGroup)
		}

		view.Groups = relativeDateGroups
		return
	}

	if av.GroupOrderAsc == view.Group.Order || av.GroupOrderDesc == view.Group.Order {
		defaultGroup := view.GetGroupByGroupValue(groupValueDefault)
		if nil != defaultGroup {
			view.RemoveGroupByID(defaultGroup.ID)
		}

		sort.SliceStable(view.Groups, func(i, j int) bool {
			iVal, jVal := view.Groups[i].GetGroupValue(), view.Groups[j].GetGroupValue()
			if av.GroupOrderAsc == view.Group.Order {
				return textsort.NaturalCompare(iVal, jVal)
			}
			return textsort.NaturalCompare(jVal, iVal)
		})

		if nil != defaultGroup {
			view.Groups = append(view.Groups, defaultGroup)
		}
		return
	}

	if av.GroupOrderSelectOption == view.Group.Order {
		groupKey := view.GetGroupKey(attrView)
		if nil == groupKey {
			return
		}

		if av.KeyTypeSelect != groupKey.Type && av.KeyTypeMSelect != groupKey.Type {
			return
		}

		SortBySelectOption(view, groupKey)
		return
	}
}

func SortBySelectOption(view *av.View, groupKey *av.Key) {
	optionSort := map[string]int{}
	for i, op := range groupKey.Options {
		optionSort[op.Name] = i
	}

	defaultGroup := view.GetGroupByGroupValue(groupValueDefault)
	if nil != defaultGroup {
		view.RemoveGroupByID(defaultGroup.ID)
	}

	sort.Slice(view.Groups, func(i, j int) bool {
		vSort := optionSort[view.Groups[i].GetGroupValue()]
		oSort := optionSort[view.Groups[j].GetGroupValue()]
		return vSort < oSort
	})

	if nil != defaultGroup {
		view.Groups = append(view.Groups, defaultGroup)
	}
}

func IsGroupByDate(view *av.View) bool {
	if !view.IsGroupView() {
		return false
	}
	return av.GroupMethodDateDay == view.Group.Method || av.GroupMethodDateWeek == view.Group.Method || av.GroupMethodDateMonth == view.Group.Method || av.GroupMethodDateYear == view.Group.Method || av.GroupMethodDateRelative == view.Group.Method
}

const (
	groupValueDefault                                        = "_@default@_"    // 默认分组值（值为空的默认分组）
	groupValueNotInRange                                     = "_@notInRange@_" // 不再范围内的分组值（只有数字类型的分组才可能是该值）
	groupValueLast30Days, groupValueLast7Days                = "_@last30Days@_", "_@last7Days@_"
	groupValueYesterday, groupValueToday, groupValueTomorrow = "_@yesterday@_", "_@today@_", "_@tomorrow@_"
	groupValueNext7Days, groupValueNext30Days                = "_@next7Days@_", "_@next30Days@_"
)
