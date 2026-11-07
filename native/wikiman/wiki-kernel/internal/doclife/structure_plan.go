// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"wiki-kernel/internal/docview"
)

type StructureRequest struct {
	Action, ID, TargetID, TargetBox, TargetPath, PreviousPath string
	After                                                     bool
}
type StructurePlan struct {
	Source, Target *parse.Tree
	From           Location
	DeleteSource   bool
	PreviousPath   string
	Effects        []string
	SourceHashes   map[string][32]byte
}
type StructureServices struct {
	Lifecycle Services
	Body      TemplateBodyServices
	ByPath    func(context.Context, string, string) (Location, bool, error)
	Text      func(context.Context, *ast.Node) (string, error)
	ListText  func(context.Context, *ast.Node) (string, error)
	CanRemove func(context.Context, Location) error
}

func PrepareStructure(ctx context.Context, q StructureRequest, s StructureServices) (out StructurePlan, err error) {
	if ctx == nil || s.Lifecycle.Lookup == nil || s.Lifecycle.Load == nil || s.Lifecycle.Parser == nil || s.Lifecycle.Box == nil || s.Lifecycle.Exists == nil || s.Body.Now == nil || s.Body.NewID == nil {
		return out, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	from, ok, err := s.Lifecycle.Lookup(ctx, q.ID)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, ErrMissing
	}
	source, err := s.Lifecycle.Load(ctx, from.Box, from.Path)
	if err != nil {
		return out, err
	}
	out = StructurePlan{Source: source, From: from, PreviousPath: q.PreviousPath}
	switch q.Action {
	case "doc-to-heading":
		if source.Root.ID != q.ID || s.CanRemove == nil {
			return StructurePlan{}, ErrDependency
		}
		if err = s.CanRemove(ctx, from); err != nil {
			return StructurePlan{}, err
		}
		to, ok, e := s.Lifecycle.Lookup(ctx, q.TargetID)
		if e != nil {
			return StructurePlan{}, e
		}
		if !ok {
			return StructurePlan{}, ErrMissing
		}
		out.Target, err = s.Lifecycle.Load(ctx, to.Box, to.Path)
		if err != nil {
			return StructurePlan{}, err
		}
		if err = DocumentToHeading(ctx, source, out.Target, q.TargetID, q.After, s.Body); err != nil {
			return StructurePlan{}, err
		}
		out.DeleteSource = true
	case "heading-to-doc", "list-to-doc":
		nodeType, titleFor := ast.NodeHeading, s.Text
		if q.Action == "list-to-doc" {
			nodeType, titleFor = ast.NodeListItem, s.ListText
		}
		if titleFor == nil || s.ByPath == nil {
			return StructurePlan{}, ErrDependency
		}
		if !ast.IsNodeIDPattern(q.TargetBox) {
			return StructurePlan{}, ErrDependency
		}
		ok, e := s.Lifecycle.Box(ctx, q.TargetBox)
		if e != nil {
			return StructurePlan{}, e
		}
		if !ok {
			return StructurePlan{}, ErrMissing
		}
		node := docview.GetNodeInTree(source, q.ID)
		if node == nil || node.Type != nodeType {
			return StructurePlan{}, ErrMissing
		}
		title, e := titleFor(ctx, node)
		if e != nil {
			return StructurePlan{}, e
		}
		if strings.Contains(title, "/") {
			title = strings.ReplaceAll(title, "/", "_")
			out.Effects = append(out.Effects, "heading-title-slash-replaced")
		}
		folder, hpath := "/", path.Join("/", title)
		if q.PreviousPath != "" {
			_, ok, e = s.ByPath(ctx, q.TargetBox, q.PreviousPath)
			if e != nil {
				return StructurePlan{}, e
			}
			if !ok {
				return StructurePlan{}, ErrMissing
			}
			parent := path.Dir(q.PreviousPath)
			if parent != "/" {
				parent += ".docj"
				loc, ok, e := s.ByPath(ctx, q.TargetBox, parent)
				if e != nil {
					return StructurePlan{}, e
				}
				if !ok {
					return StructurePlan{}, ErrMissing
				}
				folder = path.Join(path.Dir(parent), loc.ID)
				hpath = path.Join(loc.HPath, title)
			}
		} else if q.TargetPath != "/" {
			loc, ok, e := s.ByPath(ctx, q.TargetBox, q.TargetPath)
			if e != nil {
				return StructurePlan{}, e
			}
			if !ok {
				return StructurePlan{}, ErrMissing
			}
			folder = path.Join(path.Dir(q.TargetPath), loc.ID)
			hpath = path.Join(loc.HPath, title)
		}
		dest := Location{Box: q.TargetBox, Path: path.Join(folder, q.ID+".docj"), HPath: hpath, ID: q.ID, Document: true}
		if exists, e := s.Lifecycle.Exists(ctx, dest.Box, dest.Path); e != nil {
			return StructurePlan{}, e
		} else if exists {
			return StructurePlan{}, errors.New("structural_document_target_exists")
		}
		parser, e := s.Lifecycle.Parser(ctx)
		if e != nil {
			return StructurePlan{}, e
		}
		if q.Action == "list-to-doc" {
			out.Target, err = ListItemToDocument(ctx, source, node, title, dest, parser, s.Body)
		} else {
			out.Target, err = HeadingToDocument(ctx, source, q.ID, title, dest, parser, s.Body)
		}
		if err != nil {
			return StructurePlan{}, err
		}
	default:
		return StructurePlan{}, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return StructurePlan{}, err
	}
	return out, nil
}
