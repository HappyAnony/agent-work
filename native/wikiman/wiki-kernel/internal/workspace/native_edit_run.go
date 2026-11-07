// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/flashcard"
)

func (p *nativeEdit) run(op *editcore.Operation) error {
	if e := p.ctx.Err(); e != nil {
		return e
	}
	var failure *editcore.Failure
	switch op.Action {
	case "update":
		failure = p.tx.DoUpdate(op)
	case "insert":
		failure = p.tx.DoInsert(op)
	case "insertAttrViewTable":
		return p.insertAttributeViewTable(op)
	case "delete":
		failure = p.tx.DoDelete(op)
	case "move":
		failure = p.tx.DoMove(op)
	case "moveOutlineHeading":
		failure = p.tx.DoMoveOutlineHeading(op)
	case "append":
		failure = p.tx.DoAppend(op)
	case "appendInsert":
		failure = p.tx.DoAppendInsert(op)
	case "prependInsert":
		failure = p.tx.DoPrependInsert(op)
	case "foldHeading":
		failure = p.tx.DoFoldHeading(op)
	case "unfoldHeading":
		failure = p.tx.DoUnfoldHeading(op)
	case "setAttrs":
		failure = p.tx.DoSetAttrs(op)
	case "doUpdateUpdated":
		failure = p.tx.DoUpdateUpdated(op)
	case "addFlashcards", "removeFlashcards":
		return p.flashcard(op)
	default:
		return p.runAV(op)
	}
	return p.failure(op, failure)
}

func (p *nativeEdit) insertAttributeViewTable(op *editcore.Operation) error {
	if !documentID.MatchString(op.ID) || !documentID.MatchString(op.AvID) || !documentID.MatchString(op.PreviousID) ||
		op.ParentID != "" || op.NextID != "" || op.Data != nil {
		return editcore.ErrOperation
	}
	if _, err := p.readAV(op.AvID); err == nil {
		return editcore.ErrOperation
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	labels := map[string]string{"table": p.language("table"), "key": p.language("key"), "select": p.language("select")}
	a, err := av.DefaultAttributeView(op.AvID, labels, ast.NewNodeID)
	if err != nil {
		return err
	}
	a.ReadAttributeView = p.readAV
	p.avs[a.ID], p.avDirty[a.ID] = a, true
	insert := &editcore.Operation{Action: "insert", ID: op.ID, PreviousID: op.PreviousID,
		Data: `<div data-type="NodeAttributeView" data-node-id="` + op.ID + `" data-av-id="` + op.AvID + `" data-av-type="table" class="av"></div>`}
	return p.failure(insert, p.tx.DoInsert(insert))
}

func (p *nativeEdit) runAV(op *editcore.Operation) error {
	reload := func(id string) error { return p.notice("attributeView", id, nil) }
	settings := &editcore.AttributeViewSettingsEnvironment{Read: p.readAV, Stage: p.saveAV, View: p.view}
	configuration := &editcore.AttributeViewEnvironment{Read: p.readAV, Stage: p.saveAV, Groups: p.groups, RemoveRelation: func(src, dest string) error { return p.relation(src, dest, false) }, UpsertRelation: func(src, dest string) error { return p.relation(src, dest, true) }, Reload: reload, Now: func() time.Time { return p.now }, NewID: ast.NewNodeID, StrictDependencies: true}
	if editcore.IsAttributeViewFieldSettingAction(op.Action) {
		return editcore.RunAttributeViewFieldSetting(p.ctx, op, settings)
	}
	if editcore.IsAttributeViewFieldAction(op.Action) {
		return editcore.RunAttributeViewField(p.ctx, op, &editcore.AttributeViewFieldEnvironment{Read: p.readAV, Stage: p.saveAV, Groups: p.groups, Sources: p.sources, RemoveRelation: configuration.RemoveRelation, Reload: reload, StrictDependencies: true})
	}
	if editcore.IsAttributeViewOrderAction(op.Action) {
		return editcore.RunAttributeViewOrder(p.ctx, op, &editcore.AttributeViewOrderEnvironment{Read: p.readAV, Stage: p.saveAV, View: p.view, Groups: p.groups, CloneKey: editcore.CloneAttributeViewOrderKey, Defaults: p.defaults})
	}
	if editcore.IsAttributeViewRowAction(op.Action) {
		return editcore.RunAttributeViewRow(op, &editcore.AttributeViewRowEnvironment{Cell: p.cell, Defaults: p.defaults, View: p.view, Text: p.text, OptionalNode: p.node, Exists: func(id string) (bool, error) {
			_, e := p.readAV(id)
			if errors.Is(e, fs.ErrNotExist) {
				return false, nil
			}
			return e == nil, e
		}, Duplicate: func(a *av.AttributeView) error { return p.notice("duplicateAttributeViewRow", a.ID, a.Name) }, History: func(a *av.AttributeView) error { return p.notice("attributeViewHistory", a.ID, nil) }})
	}
	if editcore.IsAttributeViewManagementAction(op.Action) {
		return editcore.RunAttributeViewManagement(op, &editcore.AttributeViewManagementEnvironment{Ctx: p.ctx, StrictDependencies: true, Read: p.readAV, Stage: p.saveAV, View: p.view, Groups: p.groups, Count: p.count, SortGroups: func(a *av.AttributeView, v *av.View) error {
			if key := v.GetGroupKey(a); key != nil {
				avrender.SortBySelectOption(v, key)
			}
			return p.ctx.Err()
		}, MakeView: func(t av.LayoutType) *av.View {
			return editcore.NewAttributeViewManagementView(t, p.language, ast.NewNodeID)
		}, Language: p.language, CloneView: editcore.CloneAttributeViewManagementView, Mirrors: p.mirrors, Node: p.node, Attrs: p.attrs, MirrorAttrs: p.attrs, FlushTrees: p.flushTrees, Reload: reload})
	}
	switch op.Action {
	case "setAttrViewFilters", "setAttrViewSorts", "setAttrViewPageSize":
		return editcore.SetAttributeViewSettings(p.ctx, op, settings)
	case "updateAttrViewColOptions", "removeAttrViewColOption", "updateAttrViewColOption", "setAttrViewColOptionDesc":
		return editcore.RunAttributeViewOption(p.ctx, op, &editcore.AttributeViewOptionEnvironment{Read: p.readAV, Stage: p.saveAV, Groups: p.groups})
	case "updateAttrViewColTemplate":
		return editcore.UpdateAttributeViewColTemplateWithEnvironment(op, configuration)
	case "updateAttrViewColRelation":
		return editcore.UpdateAttributeViewColRelationWithEnvironment(op, configuration)
	case "updateAttrViewColRollup":
		return editcore.UpdateAttributeViewColRollupWithEnvironment(op, configuration)
	case "updateAttrViewCell":
		_, e := editcore.UpdateAttributeViewCellWithEnvironment(op.AvID, op.KeyID, op.RowID, op.Data, p.cell)
		return e
	case "setAttrViewName":
		return editcore.RunAttributeViewName(op, &editcore.AttributeViewNameEnvironment{Ctx: p.ctx, StrictDependencies: true, Read: p.readAV, Stage: p.saveAV, Names: p.names, Exists: p.exists, Node: p.node, Name: func(id string) (string, error) {
			a, e := p.readAV(id)
			if e != nil {
				return "", e
			}
			return a.Name, nil
		}, Untitled: func() string { return p.r.configurationBusiness.Language(p.r.configuration.Lang, 105, false, 0) }, Tree: func(*parse.Tree) error { return p.ctx.Err() }, Cache: func(string, map[string]string) error { return p.ctx.Err() }, Notify: func(old map[string]string, n *ast.Node) error {
			return p.notice("attributes", n.ID, parse.IAL2Map(n.KramdownIAL))
		}})
	}
	return editcore.ErrOperation
}

func (p *nativeEdit) language(key string) string {
	return p.r.configurationBusiness.AttributeViewLabel(p.r.configuration.Lang, key)
}

func (p *nativeEdit) flashcard(op *editcore.Operation) error {
	if p.r.configuration.Flashcard == nil {
		return flashcard.ErrDependency
	}
	store := flashcard.Open(flashcard.Source{List: func(c context.Context) ([]string, error) {
		manifest, e := confirmedWorkspaceDirectoryManifest(c, p.w, "storage/riff", workspaceDirectoryIO{})
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return nil, e
		}
		seen := map[string]bool{}
		for _, entry := range manifest.entries {
			if !entry.info.IsDir() && strings.HasSuffix(entry.name, ".deck") {
				seen[path.Base(entry.name)] = true
			}
		}
		for name := range p.files {
			if strings.HasPrefix(name, "storage/riff/") && strings.HasSuffix(name, ".deck") {
				seen[path.Base(name)] = true
			}
		}
		out := []string{}
		for name := range seen {
			out = append(out, name)
		}
		slices.Sort(out)
		return out, c.Err()
	}, Read: func(_ context.Context, name string) ([]byte, error) { return p.readFile("storage/riff/" + name) }}, *p.r.configuration.Flashcard, func() time.Time { return p.now })
	defer store.Close()
	action := "add"
	if op.Action == "removeFlashcards" {
		action = "remove"
	}
	plan, e := store.PlanMutation(p.ctx, flashcard.MutationRequest{Action: action, DeckID: op.DeckID, BlockIDs: slices.Clone(op.BlockIDs)}, flashcard.MutationServices{NewID: ast.NewNodeID, Load: func(_ context.Context, id string) (*parse.Tree, error) { _, t, e := p.node(id); return t, e }, Select: func(c context.Context, scope, id string) ([]string, error) {
		return p.r.flashcardBlockIDs(c, p.w, scope, id)
	}})
	if e != nil {
		return e
	}
	for _, t := range plan.Trees {
		if e = p.stage(t); e != nil {
			return e
		}
	}
	for _, change := range plan.Changes {
		if change.Remove {
			return flashcard.ErrInvalid
		}
		if e = p.putFile("storage/riff/"+change.Path, change.After); e != nil {
			return e
		}
	}
	for _, effect := range plan.Attributes {
		n, t, e := p.node(effect.ID)
		if e != nil {
			return e
		}
		if e = p.attrs(n, t, effect.New); e != nil {
			return e
		}
		if e = p.notice("attributes", effect.ID, effect.New); e != nil {
			return e
		}
	}
	return p.ctx.Err()
}
