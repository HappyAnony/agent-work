// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/editcore"
)

func transactionEditDiagnostic(level, format string, args ...any) {
	if level == "warn" {
		logging.LogWarnf(format, args...)
	} else {
		logging.LogErrorf(format, args...)
	}
}
func (e *attributeViewBindingEnvironment) r04Core() *editcore.AttributeViewBindingEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewBindingEnvironment{Diagnostic: transactionEditDiagnostic,
		StrictDependencies: e.strictDependencies,
		Ctx:                e.ctx,
		Legacy:             e.legacy,
		Node:               e.node,
		Names:              e.names,
		Attrs:              e.attrs,
	}
}
func r04FromAttributeViewBindingEnvironment(e *editcore.AttributeViewBindingEnvironment) *attributeViewBindingEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewBindingEnvironment{
		strictDependencies: e.StrictDependencies,
		ctx:                e.Ctx,
		legacy:             e.Legacy,
		node:               e.Node,
		names:              e.Names,
		attrs:              e.Attrs,
	}
}
func (e *attributeViewBoundEnvironment) r04Core() *editcore.AttributeViewBoundEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewBoundEnvironment{Diagnostic: transactionEditDiagnostic,
		OwnerGuard:         e.ownerGuard,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Ctx:                e.ctx,
		Legacy:             e.legacy,
		Read:               e.read,
		Stage:              e.stage,
		Groups:             e.groups,
		Exists:             e.exists,
		Load:               e.load,
		IsMirror:           e.isMirror,
		RemoveMirror:       e.removeMirror,
		Names:              e.names,
		Attrs:              e.attrs,
		Cache:              e.cache,
		Notify:             e.notify,
		Tree:               e.tree,
		Reload:             e.reload,
	}
}
func r04FromAttributeViewBoundEnvironment(e *editcore.AttributeViewBoundEnvironment) *attributeViewBoundEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewBoundEnvironment{
		ownerGuard:         e.OwnerGuard,
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		ctx:                e.Ctx,
		legacy:             e.Legacy,
		read:               e.Read,
		stage:              e.Stage,
		groups:             e.Groups,
		exists:             e.Exists,
		load:               e.Load,
		isMirror:           e.IsMirror,
		removeMirror:       e.RemoveMirror,
		names:              e.Names,
		attrs:              e.Attrs,
		cache:              e.Cache,
		notify:             e.Notify,
		tree:               e.Tree,
		reload:             e.Reload,
	}
}
func (e *attributeViewCellEnvironment) r04Core() *editcore.AttributeViewCellEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewCellEnvironment{Diagnostic: transactionEditDiagnostic,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Legacy:             e.legacy,
		Ctx:                e.ctx,
		Read:               e.read,
		Stage:              e.stage,
		Groups:             e.groups,
		Node:               e.node,
		BlockText:          e.blockText,
		Names:              e.names,
		Attrs:              e.attrs,
		Related:            e.related,
		Now:                e.now,
		NewID:              e.newID,
		Color:              e.color,
	}
}
func r04FromAttributeViewCellEnvironment(e *editcore.AttributeViewCellEnvironment) *attributeViewCellEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewCellEnvironment{
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		legacy:             e.Legacy,
		ctx:                e.Ctx,
		read:               e.Read,
		stage:              e.Stage,
		groups:             e.Groups,
		node:               e.Node,
		blockText:          e.BlockText,
		names:              e.Names,
		attrs:              e.Attrs,
		related:            e.Related,
		now:                e.Now,
		newID:              e.NewID,
		color:              e.Color,
	}
}
func (e *attributeViewDefaultEnvironment) r04Core() *editcore.AttributeViewDefaultEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewDefaultEnvironment{Diagnostic: transactionEditDiagnostic,
		Ctx:          e.ctx,
		Near:         e.near,
		TemplateKeys: e.templateKeys,
		Reverse:      e.reverse,
		Now:          e.now,
		NewID:        e.newID,
	}
}
func r04FromAttributeViewDefaultEnvironment(e *editcore.AttributeViewDefaultEnvironment) *attributeViewDefaultEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewDefaultEnvironment{
		ctx:          e.Ctx,
		near:         e.Near,
		templateKeys: e.TemplateKeys,
		reverse:      e.Reverse,
		now:          e.Now,
		newID:        e.NewID,
	}
}
func (e *attributeViewEnvironment) r04Core() *editcore.AttributeViewEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewEnvironment{Diagnostic: transactionEditDiagnostic,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Legacy:             e.legacy,
		Read:               e.read,
		Stage:              e.stage,
		Groups:             e.groups,
		RemoveRelation:     e.removeRelation,
		UpsertRelation:     e.upsertRelation,
		Reload:             e.reload,
		Now:                e.now,
		NewID:              e.newID,
	}
}
func r04FromAttributeViewEnvironment(e *editcore.AttributeViewEnvironment) *attributeViewEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewEnvironment{
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		legacy:             e.Legacy,
		read:               e.Read,
		stage:              e.Stage,
		groups:             e.Groups,
		removeRelation:     e.RemoveRelation,
		upsertRelation:     e.UpsertRelation,
		reload:             e.Reload,
		now:                e.Now,
		newID:              e.NewID,
	}
}
func (e *attributeViewFieldEnvironment) r04Core() *editcore.AttributeViewFieldEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewFieldEnvironment{Diagnostic: transactionEditDiagnostic,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Legacy:             e.legacy,
		Read:               e.read,
		Stage:              e.stage,
		Groups:             e.groups,
		Sources:            e.sources,
		RemoveRelation:     e.removeRelation,
		Reload:             e.reload,
	}
}
func r04FromAttributeViewFieldEnvironment(e *editcore.AttributeViewFieldEnvironment) *attributeViewFieldEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewFieldEnvironment{
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		legacy:             e.Legacy,
		read:               e.Read,
		stage:              e.Stage,
		groups:             e.Groups,
		sources:            e.Sources,
		removeRelation:     e.RemoveRelation,
		reload:             e.Reload,
	}
}
func (e *attributeViewManagementEnvironment) r04Core() *editcore.AttributeViewManagementEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewManagementEnvironment{Diagnostic: transactionEditDiagnostic,
		OwnerGuard:         e.ownerGuard,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Ctx:                e.ctx,
		Legacy:             e.legacy,
		Read:               e.read,
		Stage:              e.stage,
		View:               e.view,
		Groups:             e.groups,
		Count:              e.count,
		SortGroups:         e.sortGroups,
		MakeView:           e.makeView,
		Language:           e.language,
		CloneView:          e.cloneView,
		Mirrors:            e.mirrors,
		Node:               e.node,
		Attrs:              e.attrs,
		MirrorAttrs:        e.mirrorAttrs,
		FlushTrees:         e.flushTrees,
		Reload:             e.reload,
	}
}
func r04FromAttributeViewManagementEnvironment(e *editcore.AttributeViewManagementEnvironment) *attributeViewManagementEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewManagementEnvironment{
		ownerGuard:         e.OwnerGuard,
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		ctx:                e.Ctx,
		legacy:             e.Legacy,
		read:               e.Read,
		stage:              e.Stage,
		view:               e.View,
		groups:             e.Groups,
		count:              e.Count,
		sortGroups:         e.SortGroups,
		makeView:           e.MakeView,
		language:           e.Language,
		cloneView:          e.CloneView,
		mirrors:            e.Mirrors,
		node:               e.Node,
		attrs:              e.Attrs,
		mirrorAttrs:        e.MirrorAttrs,
		flushTrees:         e.FlushTrees,
		reload:             e.Reload,
	}
}
func (e *attributeViewNameEnvironment) r04Core() *editcore.AttributeViewNameEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewNameEnvironment{Diagnostic: transactionEditDiagnostic,
		OwnerGuard:         e.ownerGuard,
		StrictDependencies: e.strictDependencies,
		Complete:           e.complete,
		Names:              e.names,
		Ctx:                e.ctx,
		Legacy:             e.legacy,
		Read:               e.read,
		Stage:              e.stage,
		Exists:             e.exists,
		Node:               e.node,
		Name:               e.name,
		Untitled:           e.untitled,
		Tree:               e.tree,
		Cache:              e.cache,
		Notify:             e.notify,
	}
}
func r04FromAttributeViewNameEnvironment(e *editcore.AttributeViewNameEnvironment) *attributeViewNameEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewNameEnvironment{
		ownerGuard:         e.OwnerGuard,
		strictDependencies: e.StrictDependencies,
		complete:           e.Complete,
		names:              e.Names,
		ctx:                e.Ctx,
		legacy:             e.Legacy,
		read:               e.Read,
		stage:              e.Stage,
		exists:             e.Exists,
		node:               e.Node,
		name:               e.Name,
		untitled:           e.Untitled,
		tree:               e.Tree,
		cache:              e.Cache,
		notify:             e.Notify,
	}
}
func (e *attributeViewOptionEnvironment) r04Core() *editcore.AttributeViewOptionEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewOptionEnvironment{Diagnostic: transactionEditDiagnostic,
		Complete: e.complete,
		Read:     e.read,
		Stage:    e.stage,
		Groups:   e.groups,
	}
}
func r04FromAttributeViewOptionEnvironment(e *editcore.AttributeViewOptionEnvironment) *attributeViewOptionEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewOptionEnvironment{
		complete: e.Complete,
		read:     e.Read,
		stage:    e.Stage,
		groups:   e.Groups,
	}
}
func (e *attributeViewOrderEnvironment) r04Core() *editcore.AttributeViewOrderEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewOrderEnvironment{Diagnostic: transactionEditDiagnostic,
		Complete: e.complete,
		Read:     e.read,
		Stage:    e.stage,
		View:     e.view,
		Groups:   e.groups,
		CloneKey: e.cloneKey,
		Defaults: e.defaults.r04Core(),
	}
}
func r04FromAttributeViewOrderEnvironment(e *editcore.AttributeViewOrderEnvironment) *attributeViewOrderEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewOrderEnvironment{
		complete: e.Complete,
		read:     e.Read,
		stage:    e.Stage,
		view:     e.View,
		groups:   e.Groups,
		cloneKey: e.CloneKey,
		defaults: r04FromAttributeViewDefaultEnvironment(e.Defaults),
	}
}
func (e *attributeViewRowEnvironment) r04Core() *editcore.AttributeViewRowEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewRowEnvironment{Diagnostic: transactionEditDiagnostic,
		Cell:         e.cell.r04Core(),
		Defaults:     e.defaults.r04Core(),
		View:         e.view,
		Text:         e.text,
		OptionalNode: e.optionalNode,
		Exists:       e.exists,
		Duplicate:    e.duplicate,
		History:      e.history,
	}
}
func r04FromAttributeViewRowEnvironment(e *editcore.AttributeViewRowEnvironment) *attributeViewRowEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewRowEnvironment{
		cell:         r04FromAttributeViewCellEnvironment(e.Cell),
		defaults:     r04FromAttributeViewDefaultEnvironment(e.Defaults),
		view:         e.View,
		text:         e.Text,
		optionalNode: e.OptionalNode,
		exists:       e.Exists,
		duplicate:    e.Duplicate,
		history:      e.History,
	}
}
func (e *attributeViewSettingsEnvironment) r04Core() *editcore.AttributeViewSettingsEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewSettingsEnvironment{Diagnostic: transactionEditDiagnostic,
		Complete: e.complete,
		Read:     e.read,
		View:     e.view,
		Stage:    e.stage,
	}
}
func r04FromAttributeViewSettingsEnvironment(e *editcore.AttributeViewSettingsEnvironment) *attributeViewSettingsEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewSettingsEnvironment{
		complete: e.Complete,
		read:     e.Read,
		view:     e.View,
		stage:    e.Stage,
	}
}
func (e *attributeViewTextEnvironment) r04Core() *editcore.AttributeViewTextEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewTextEnvironment{Diagnostic: transactionEditDiagnostic,
		Ctx:        e.ctx,
		JobContext: e.jobContext,
		Legacy:     e.legacy,
		Read:       e.read,
		Clone:      e.clone,
		Text:       e.text,
		Stage:      e.stage,
		Reload:     e.reload,
		Related:    e.related,
		Complete:   e.complete,
	}
}
func r04FromAttributeViewTextEnvironment(e *editcore.AttributeViewTextEnvironment) *attributeViewTextEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewTextEnvironment{
		ctx:        e.Ctx,
		jobContext: e.JobContext,
		legacy:     e.Legacy,
		read:       e.Read,
		clone:      e.Clone,
		text:       e.Text,
		stage:      e.Stage,
		reload:     e.Reload,
		related:    e.Related,
		complete:   e.Complete,
	}
}
func (e *attributeViewUpdateEnvironment) r04Core() *editcore.AttributeViewUpdateEnvironment {
	if e == nil {
		return nil
	}
	return &editcore.AttributeViewUpdateEnvironment{Diagnostic: transactionEditDiagnostic,
		StrictDependencies: e.strictDependencies,
		AsyncRefresh:       e.asyncRefresh,
		Complete:           e.complete,
		Ctx:                e.ctx,
		Legacy:             e.legacy,
		Read:               e.read,
		Upsert:             e.upsert,
		Sources:            e.sources,
		Groups:             e.groups,
		Stage:              e.stage,
		Reload:             e.reload,
		Attrs:              e.attrs,
		Tree:               e.tree,
	}
}
func r04FromAttributeViewUpdateEnvironment(e *editcore.AttributeViewUpdateEnvironment) *attributeViewUpdateEnvironment {
	if e == nil {
		return nil
	}
	return &attributeViewUpdateEnvironment{
		strictDependencies: e.StrictDependencies,
		asyncRefresh:       e.AsyncRefresh,
		complete:           e.Complete,
		ctx:                e.Ctx,
		legacy:             e.Legacy,
		read:               e.Read,
		upsert:             e.Upsert,
		sources:            e.Sources,
		groups:             e.Groups,
		stage:              e.Stage,
		reload:             e.Reload,
		attrs:              e.Attrs,
		tree:               e.Tree,
	}
}
