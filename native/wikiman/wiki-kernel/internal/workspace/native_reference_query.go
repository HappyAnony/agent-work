// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"maps"
	"slices"
	"wiki-kernel/internal/indexdb"
)

type NativeReferenceQuery struct {
	Mode, ID, OtherID, Keyword string
	IDs                        []string
	Types                      map[string]bool
	Children, OnlyDoc          bool
}

func (l *Lease) NativeReferenceQuery(ctx context.Context, q NativeReferenceQuery) (out any, err error) {
	q.IDs = slices.Clone(q.IDs)
	q.Types = maps.Clone(q.Types)
	err = l.WithNativeReferences(ctx, func(r indexdb.References) (e error) {
		switch q.Mode {
		case "counts":
			out, e = r.DefinitionCounts(ctx, q.IDs)
		case "root-counts":
			out, e = r.RootCounts(ctx)
		case "root-children-counts":
			out, e = r.RootChildrenCounts(ctx, q.ID)
		case "duplicated-roots":
			out, e = r.DuplicatedDefinitionRoots(ctx)
		case "child-definitions":
			out, e = r.ChildDefinitions(ctx, q.ID)
		case "child-definition-ids":
			out, e = r.ChildDefinitionIDs(ctx, q.ID)
		case "ref-ids":
			out, e = r.RefIDs(ctx, q.ID, q.Children)
		case "pair":
			out, e = r.ByPair(ctx, q.ID, q.OtherID)
		case "block-children":
			out, e = r.BlockChildren(ctx, q.ID)
		case "definition":
			out, e = r.ByDefinition(ctx, q.ID, q.Children)
		case "definition-roots":
			out, e = r.DefinitionRoots(ctx, q.ID)
		case "referring-roots":
			out, e = r.ReferringRoots(ctx, q.IDs)
		case "recent":
			ignore, err := nativeIgnoreLines(ctx, l.workspace, "refsearchignore")
			if err != nil {
				return err
			}
			out, e = r.Recent(ctx, q.OnlyDoc, indexdb.SearchTypeFilter(*l.workspace.resources.configuration.Search, q.Types), ignore)
		case "annotation":
			out, e = r.ByAnnotation(ctx, q.ID)
		case "definitions-by-text":
			out, e = r.DefinitionIDsByText(ctx, q.Keyword, q.IDs, l.workspace.resources.configuration.Search.CaseSensitive)
		default:
			return errors.New("invalid_reference_query")
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
