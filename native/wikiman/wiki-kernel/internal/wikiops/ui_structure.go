// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
)

var uiStructureActions = map[string]string{
	"/api/filetree/doc2Heading": "doc-to-heading",
	"/api/filetree/heading2Doc": "heading-to-doc",
	"/api/filetree/li2Doc":      "list-to-doc",
}

func runUIStructure(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, action string) (any, error) {
	q := doclife.StructureRequest{Action: action}
	if action == "doc-to-heading" {
		var input struct {
			SrcID, TargetID string
			After           bool
		}
		if err := Decode(bytes.NewReader(ui.Data), &input); err != nil || !nativeID.MatchString(input.TargetID) {
			return nil, ErrArgument
		}
		q.ID, q.TargetID, q.After = input.SrcID, input.TargetID, input.After
	} else {
		var input struct {
			SrcHeadingID, SrcListItemID, TargetNotebook, TargetPath, PreviousPath string
			Callback                                                              json.RawMessage
		}
		if err := Decode(bytes.NewReader(ui.Data), &input); err != nil || !nativeID.MatchString(input.TargetNotebook) {
			return nil, ErrArgument
		}
		q.ID = input.SrcHeadingID
		if action == "list-to-doc" {
			q.ID = input.SrcListItemID
		}
		for _, name := range []string{input.TargetPath, input.PreviousPath} {
			if name != "" && name != "/" && (!strings.HasPrefix(name, "/") || !documentRelativePath(strings.TrimPrefix(name, "/"))) {
				return nil, ErrArgument
			}
		}
		q.TargetBox, q.TargetPath, q.PreviousPath = input.TargetNotebook, input.TargetPath, input.PreviousPath
		if q.TargetPath == "" {
			q.TargetPath = "/"
		}
	}
	if !nativeID.MatchString(q.ID) {
		return nil, ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID, Action: action,
		OperationID: "ui-structure-" + ast.NewNodeID()}
	var reply map[string]string
	_, err := planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
		l, err := p.lease()
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(p.ctx)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		plan, err := l.PlanNativeStructure(p.ctx, q)
		if err != nil {
			return err
		}
		if err = p.checkPlanInputs(plan.SourceHashes); err != nil {
			return err
		}
		source, target := path.Join(plan.Source.Box, plan.Source.Path), path.Join(plan.Target.Box, plan.Target.Path)
		before, err := p.bytes(source)
		if err != nil {
			return err
		}
		preimages := [][]byte{before}
		if plan.DeleteSource {
			if p.isEntry(plan.Source.Box, plan.Source.ID) || len(p.subtree(source)) != 1 {
				return ErrArgument
			}
			raw, err := p.bytes(target)
			if err != nil {
				return err
			}
			preimages = append(preimages, raw)
		}
		engine := nativeparser.NewEditor(nativeparser.EditorOptions{})
		stage := func(t *parse.Tree, name string) error {
			data, err := docview.EncodeWriteTree(t, engine, true)
			if err != nil {
				return err
			}
			data, err = docast.SerializeDocument(preimages, data)
			if err != nil {
				return err
			}
			p.write(name, data)
			return nil
		}
		if err = stage(plan.Target, target); err != nil {
			return err
		}
		if plan.DeleteSource {
			p.request.BoxID, p.request.DocID = plan.Source.Box, plan.Source.ID
			if err = p.saveVersion(before); err != nil {
				return err
			}
			p.change(p.prefix+source, p.files[source].Hash, nil, true)
			_, err = p.removeRanks(l, plan.Source.Box, []string{plan.Source.ID})
			reply = map[string]string{"srcTreeBox": plan.Source.Box, "srcTreePath": plan.Source.Path}
			return err
		}
		if err = stage(plan.Source, source); err != nil {
			return err
		}
		reply = map[string]string{"srcRootBlockID": plan.Source.ID, "path": plan.Target.Path, "box": plan.Target.Box}
		return p.sortCreation(l, plan.Target.Box, path.Dir(plan.Target.Path), plan.Target.ID, plan.PreviousPath)
	})
	return reply, err
}
