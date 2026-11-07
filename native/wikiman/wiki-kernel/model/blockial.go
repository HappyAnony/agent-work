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
	"errors"
	"fmt"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/lex"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/markup"
	"wiki-kernel/util"
)

func SetBlockReminder(id, timed string) error {
	return SetBlockReminderContext(context.Background(), id, timed)
}

func BatchSetBlockAttrs(blockAttrs []map[string]interface{}) error {
	return BatchSetBlockAttrsContext(context.Background(), blockAttrs)
}
func BatchSetBlockAttrsContext(ctx context.Context, blockAttrs []map[string]interface{}) error {
	copy := make([]map[string]interface{}, len(blockAttrs))
	for i, item := range blockAttrs {
		attrs, ok := item["attrs"].(map[string]string)
		if !ok {
			return errors.New("invalid_block_attribute_input")
		}
		copy[i] = map[string]interface{}{"id": item["id"], "attrs": copyBlockAttributes(attrs)}
	}
	return runOwnedBlockAttributes(ctx, func(e *blockAttributeEnvironment) error { return batchSetBlockAttributesWithEnvironment(copy, e) })
}
func SetBlockAttrs(id string, nameValues map[string]string) error {
	return SetBlockAttrsContext(context.Background(), id, nameValues)
}
func SetBlockAttrsContext(ctx context.Context, id string, nameValues map[string]string) error {
	attrs := copyBlockAttributes(nameValues)
	return runOwnedBlockAttributes(ctx, func(e *blockAttributeEnvironment) error {
		load := e.load
		e.load = func(id string) (*parse.Tree, error) {
			t, err := load(id)
			if err == nil && t != nil && IsUserGuide(t.Box) && len(attrs) == 1 && attrs["scroll"] != "" {
				attrs["scroll"] = ""
			}
			return t, err
		}
		return setBlockAttributesWithEnvironment(id, attrs, e)
	})
}
func setNodeAttrs(node *ast.Node, tree *parse.Tree, nameValues map[string]string) error {
	return setNodeAttributesWithEnvironment(node, tree, nameValues, legacyBlockAttributeEnvironment(), true)
}
func setNodeAttrsWithTx(tx *Transaction, node *ast.Node, tree *parse.Tree, nameValues map[string]string) error {
	e, err := tx.blockAttributeEnvironment(false)
	if err != nil {
		return err
	}
	e.stage = tx.writeTree
	return setNodeAttributesWithEnvironment(node, tree, nameValues, e, false)
}

func setNodeAttrs0(node *ast.Node, nameValues map[string]string) (map[string]string, error) {
	return setNodeAttrs0WithServices(node, nameValues, func(id string) error { return errors.New(fmt.Sprintf(Conf.Language(25), id)) }, func() error { ReloadTag(); return nil })
}
func setNodeAttrs0WithServices(node *ast.Node, nameValues map[string]string, invalid func(string) error, tags func() error) (oldAttrs map[string]string, err error) {
	oldAttrs = parse.IAL2Map(node.KramdownIAL)

	for name := range nameValues {
		for i := 0; i < len(name); i++ {
			if !lex.IsASCIILetterNumHyphen(name[i]) {
				err = invalid(node.ID)
				return
			}
		}
	}

	if tag, ok := nameValues["tags"]; ok {
		nameValues["tags"] = markup.NormalizeTagList(tag)
	}

	normalizeKeysToLower(nameValues)

	for name, value := range nameValues {
		value = util.RemoveInvalidRetainCtrl(value)
		value = strings.TrimSpace(value)
		value = strings.TrimSuffix(value, ",")
		if "" == value {
			node.RemoveIALAttr(name)
		} else {
			node.SetIALAttr(name, value)
		}
	}

	if oldAttrs["tags"] != nameValues["tags"] {
		err = tags()
	}
	return
}

// normalizeKeysToLower 将 nameValues 的键统一为小写 https://github.com/siyuan-note/siyuan/issues/16447
func normalizeKeysToLower(nameValues map[string]string) {
	newMap := make(map[string]string, len(nameValues))
	for name, value := range nameValues {
		lower := strings.ToLower(name)
		newMap[lower] = value
	}

	for k := range nameValues {
		delete(nameValues, k)
	}

	for k, v := range newMap {
		nameValues[k] = v
	}
}

func pushBroadcastAttrTransactions(oldAttrs map[string]string, node *ast.Node) {
	evt := util.NewCmdResult("transactions", 0, util.PushModeBroadcast)
	evt.Data = blockAttrTransactions(oldAttrs, node)
	util.PushEvent(evt)
}

// Scoped handlers stage the same payload on the owner's transaction instead
// of publishing through the legacy global broadcaster.
func blockAttrTransactions(oldAttrs map[string]string, node *ast.Node) []*Transaction {
	newAttrs := parse.IAL2Map(node.KramdownIAL)
	data := map[string]interface{}{"old": oldAttrs, "new": newAttrs}
	if "" != node.AttributeViewType {
		data["data-av-type"] = node.AttributeViewType
	}
	doOp := &Operation{Action: "updateAttrs", Data: data, ID: node.ID}
	return []*Transaction{{
		DoOperations:   []*Operation{doOp},
		UndoOperations: []*Operation{},
	}}
}

func ResetBlockAttrs(id string, nameValues map[string]string) (err error) {
	return ResetBlockAttrsContext(context.Background(), id, nameValues)
}

func ResetBlockAttrsContext(ctx context.Context, id string, nameValues map[string]string) error {
	attrs := copyBlockAttributes(nameValues)
	return runOwnedBlockAttributes(ctx, func(e *blockAttributeEnvironment) error { return resetBlockAttributesWithEnvironment(id, attrs, e) })
}
