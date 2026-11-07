// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"html"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

// BlockLocation preserves the original projected parent/heading chain.
func BlockLocation(ctx context.Context, id string, tree *parse.Tree, parser *lute.Lute, content indexdb.ContentServices) (map[string]string, error) {
	if ctx == nil || tree == nil || tree.Root == nil || parser == nil {
		return nil, errors.New("block_location_unbound")
	}
	project := func(id string) (*blockview.Block, error) {
		v, e := Blocks(ctx, BlocksRequest{Mode: "block", ID: id}, BlocksServices{Load: func(c context.Context, _ string) (*parse.Tree, error) { return tree, c.Err() }, Parser: parser, Content: content, Missing: errors.New("block_location_missing")})
		if e != nil {
			return nil, e
		}
		if v.Block == nil {
			return nil, errors.New("block_location_missing")
		}
		return blockview.FromRow(v.Block, "", 0, false), nil
	}
	block, e := project(id)
	if e != nil {
		return nil, e
	}
	b := block
	rootChildID := ""
	for range 128 {
		if b.ParentID == "" {
			rootChildID = b.ID
			break
		}
		b, e = project(b.ParentID)
		if e != nil {
			return nil, e
		}
	}
	if rootChildID == "" {
		return nil, errors.New("block_location_parent_depth")
	}
	// Read root metadata from the parsed source. Re-tokenizing unrelated JSON
	// attributes (for example scroll) can consume later quoted IAL fields.
	return map[string]string{"box": block.Box, "path": block.Path, "rootID": block.RootID, "rootTitle": html.UnescapeString(tree.Root.IALAttr("title")), "rootChildID": rootChildID, "rootIcon": tree.Root.IALAttr("icon")}, ctx.Err()
}
