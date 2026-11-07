// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"slices"
	"sort"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func CheckBlockRefContext(ctx context.Context, ids []string) (bool, error) {
	ids = slices.Clone(ids)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (bool, error) {
		return docquery.CheckReferences(c, ids, func(c context.Context, ids []string) ([]indexdb.BlockTreeRecord, error) {
			if e := o.r.guard(c); e != nil {
				return nil, e
			}
			return o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
		}, indexdb.References{DB: o.r.main.DB})
	})
}

type referenceMetadata struct {
	defs     []*RefDefs
	original map[string]string
}

func GetBlockRefsContext(ctx context.Context, id string) ([]*RefDefs, map[string]string, error) {
	v, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (referenceMetadata, error) {
		loc, e := o.r.referenceLookup(c, id)
		if e != nil {
			return referenceMetadata{}, e
		}
		if loc == nil {
			return referenceMetadata{defs: []*RefDefs{}, original: map[string]string{}}, nil
		}
		defs, original, e := o.referenceDefs(c, id, loc.RootID)
		return referenceMetadata{defs, original}, e
	})
	return v.defs, v.original, e
}
func GetBlockRefIDsByFileAnnotationIDContext(ctx context.Context, id string) ([]string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]string, error) {
		return (indexdb.References{DB: o.r.main.DB}).ByAnnotation(c, id)
	})
}
func GetBlockDefIDsByRefTextContext(ctx context.Context, text string, exclude []string) ([]string, error) {
	exclude = slices.Clone(exclude)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]string, error) {
		if o.settings.Search == nil {
			return nil, errors.New("reference_search_settings_missing")
		}
		out, e := (indexdb.References{DB: o.r.main.DB}).DefinitionIDsByText(c, text, exclude, o.settings.Search.CaseSensitive)
		if e != nil {
			return nil, e
		}
		sort.Sort(sort.Reverse(sort.StringSlice(out)))
		if out == nil {
			out = []string{}
		}
		return out, c.Err()
	})
}
