// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	"context"
	"errors"
	"sort"
)

var ErrOperation = errors.New("invalid_original_edit_operation")

// EditingActions is the original transaction switch, excluding the internal
// parse.Tree-only create action whose lifecycle admission remains R05.
func EditingActions() []string {
	ret := []string{"update", "insert", "delete", "move", "moveOutlineHeading", "append", "appendInsert", "prependInsert", "foldHeading", "unfoldHeading", "setAttrs", "doUpdateUpdated", "addFlashcards", "removeFlashcards", "setAttrViewName", "setAttrViewFilters", "setAttrViewSorts", "setAttrViewPageSize", "updateAttrViewColTemplate", "updateAttrViewColRelation", "updateAttrViewColRollup", "updateAttrViewCell"}
	for _, s := range AttributeViewManagementActions {
		ret = append(ret, s)
	}
	for s := range AttributeViewFieldSettingOperations {
		ret = append(ret, s)
	}
	ret = append(ret, "insertAttrViewTable", "insertAttrViewBlock", "removeAttrViewBlock", "replaceAttrViewBlock", "sortAttrViewRow", "sortAttrViewCol", "sortAttrViewKey", "duplicateAttrViewKey", "addAttrViewCol", "updateAttrViewCol", "removeAttrViewCol", "updateAttrViewColOptions", "removeAttrViewColOption", "updateAttrViewColOption", "setAttrViewColOptionDesc")
	sort.Strings(ret)
	return ret
}

func ValidateOperations(ctx context.Context, operations []*Operation) error {
	if ctx == nil {
		return ErrOperation
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(operations) == 0 || len(operations) > 10000 {
		return ErrOperation
	}
	actions := map[string]bool{}
	for _, name := range EditingActions() {
		actions[name] = true
	}
	for _, op := range operations {
		if op == nil || !actions[op.Action] {
			return ErrOperation
		}
		switch op.Action {
		case "update", "insert", "appendInsert", "prependInsert", "setAttrs", "doUpdateUpdated", "updateAttrViewColTemplate":
			if _, ok := op.Data.(string); !ok {
				return ErrOperation
			}
		}
	}
	return nil
}
