// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/internal/workspace"
)

func uiMarkdownImport(endpoint string) bool {
	return endpoint == "/api/import/importZipMd" || endpoint == "/api/import/importStdMd"
}

func runUIMarkdownImport(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest) error {
	var input struct {
		Notebook, ToPath, Name string
		Data                   []byte
		Files                  map[string][]byte
	}
	if err := Decode(bytes.NewReader(ui.Data), &input); err != nil || ui.OperationID == "" || !nativeID.MatchString(input.Notebook) ||
		(input.ToPath != "/" && (!strings.HasPrefix(input.ToPath, "/") || !documentRelativePath(strings.TrimPrefix(input.ToPath, "/")))) {
		return ErrArgument
	}
	if ui.Endpoint == "/api/import/importZipMd" {
		if len(input.Data) == 0 || len(input.Data) > 8<<20 || len(input.Files) != 0 {
			return ErrArgument
		}
	} else if len(input.Data) != 0 || len(input.Files) == 0 || len(input.Files) > 1000 {
		return ErrArgument
	}
	if len(input.Name) > 255 || strings.ContainsAny(input.Name, "/\\\x00") {
		return ErrArgument
	}
	r := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID, Action: ui.Endpoint,
		BoxID: input.Notebook, AST: ui.Data, OperationID: ui.OperationID}
	_, err := planContentLifecycle(ctx, m, r, func(p *lifecycleEdit) error {
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
		boxes, err := l.ConfigurationNotebooks(p.ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(boxes, func(box conf.Notebook) bool { return box.ID == input.Notebook && !box.Closed }) {
			return fs.ErrNotExist
		}
		folder := input.Notebook
		if input.ToPath != "/" {
			id := strings.TrimSuffix(path.Base(input.ToPath), ".docj")
			name, _, err := p.doc(input.Notebook, id)
			if err != nil {
				return err
			}
			if name != input.Notebook+input.ToPath {
				return sourcetx.ErrConflict
			}
			folder = strings.TrimSuffix(name, ".docj")
		}
		files := input.Files
		if len(input.Data) != 0 {
			files, err = archivepack.ReadMarkdownArchive(p.ctx, input.Data)
			if err != nil {
				return errors.Join(ErrArgument, err)
			}
		}
		emojis, err := l.CustomEmojis(p.ctx)
		if err != nil {
			return err
		}
		opts, err := cfg.ParserOptions(emojis)
		if err != nil {
			return err
		}
		return p.stageMarkdownImport(folder, input.Name, files, opts, len(input.Data) != 0)
	})
	return err
}

func markdownExtension(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".md" || ext == ".markdown"
}

func (p *lifecycleEdit) stageMarkdownImport(folder, label string, files map[string][]byte, opts nativeparser.EditorOptions, zipped bool) error {
	roots, documents := map[string]string{}, map[string]string{}
	total := 0
	for name, data := range files {
		total += len(data)
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") || len(name) > 1024 || strings.Count(name, "/") > 32 ||
			total > 48<<20 || !zipped && total > 8<<20 {
			return ErrArgument
		}
		if !markdownExtension(name) {
			continue
		}
		if !utf8.Valid(data) || len(data) > 8<<20 {
			return ErrArgument
		}
		stem := strings.TrimSuffix(name, path.Ext(name))
		if documents[stem] != "" {
			return ErrArgument
		}
		documents[stem] = name
		for rel := stem; rel != "."; rel = path.Dir(rel) {
			if roots[rel] == "" {
				roots[rel] = ast.NewNodeID()
			}
		}
	}
	if len(documents) == 0 || len(roots) > 1000 {
		return ErrArgument
	}
	if zipped {
		roots["."] = ast.NewNodeID()
		folder = path.Join(folder, roots["."])
	}
	std := nativeparser.NewStdLute(opts.Markdown)
	std.SetYamlFrontMatter(true)
	editor := nativeparser.NewEditor(opts)
	service := doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now}
	trees := map[string]*parse.Tree{}
	for _, stem := range slices.Sorted(maps.Keys(roots)) {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		tree := parse.Parse("", files[documents[stem]], std.ParseOptions)
		_, title, updated, err := doclife.NormalizeCreationTemplate(p.ctx, tree, service)
		if err != nil {
			return err
		}
		if title == "" {
			title = path.Base(stem)
			if stem == "." {
				title = strings.TrimSuffix(label, path.Ext(label))
			}
		}
		if decoded, e := url.PathUnescape(title); e == nil {
			title = decoded
		}
		title = textprefs.FilterFileName(title)
		if title == "" || title == "." || title == ".." {
			title = "Markdown import"
		}
		id := roots[stem]
		var name string
		if stem == "." {
			name = folder + ".docj"
		} else {
			mounted, err := archiveMountPath(stem, roots)
			if err != nil {
				return err
			}
			name = path.Join(folder, mounted+".docj")
		}
		tree.ID, tree.Root.ID, tree.Root.Spec = id, id, "1"
		tree.Box, tree.Path = p.request.BoxID, strings.TrimPrefix(name, p.request.BoxID)
		tree.Root.SetIALAttr("id", id)
		tree.Root.SetIALAttr("title", title)
		tree.Root.SetIALAttr("type", "doc")
		if updated != "" {
			tree.Root.SetIALAttr("updated", updated)
		}
		parse.TextMarks2Inlines(tree)
		parse.NestedInlines2FlattedSpansHybrid(tree, false)
		trees[stem] = tree
	}
	assets := map[string]string{}
	candidates := map[string][]byte{}
	for stem, tree := range trees {
		var problem error
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || n.Type != ast.NodeLinkDest && !n.IsTextMarkType("a") {
				return ast.WalkContinue
			}
			dest := n.TextMarkAHref
			if n.Type == ast.NodeLinkDest {
				dest = n.TokensStr()
			}
			u, err := url.Parse(dest)
			if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
				return ast.WalkContinue
			}
			local := path.Join(path.Dir(documents[stem]), u.Path)
			targetStem := strings.TrimSuffix(local, path.Ext(local))
			if id := roots[targetStem]; documents[targetStem] == local && id != "" {
				if u.Fragment != "" {
					target := trees[targetStem]
					ast.Walk(target.Root, func(heading *ast.Node, enter bool) ast.WalkStatus {
						if enter && heading.Type == ast.NodeHeading && heading.Text() == u.Fragment {
							id = heading.ID
							return ast.WalkStop
						}
						return ast.WalkContinue
					})
				}
				dest = "siyuan://blocks/" + id
			} else if data, ok := files[local]; ok && !markdownExtension(local) {
				if assets[local] == "" {
					name := textprefs.FilterUploadFileName(path.Base(local))
					if name == "" || len(name) > 200 {
						problem = ErrArgument
						return ast.WalkStop
					}
					assets[local] = "assets/" + ast.NewNodeID() + "-" + name
					p.write(assets[local], data)
				}
				dest = assets[local]
			} else {
				return ast.WalkContinue
			}
			if n.Type == ast.NodeLinkDest {
				n.Tokens = []byte(dest)
			} else {
				n.TextMarkAHref = dest
			}
			return ast.WalkContinue
		})
		if problem != nil {
			return problem
		}
		if _, err := docview.PrepareWriteTree(p.ctx, tree, func() *ast.Node { return docview.NewParagraph("") }); err != nil {
			return err
		}
		raw, err := docview.EncodeWriteTree(tree, editor, true)
		if err != nil {
			return err
		}
		name := tree.Box + tree.Path
		if p.files[name].Hash != "" {
			return sourcetx.ErrConflict
		}
		candidates[name] = raw
	}
	if err := p.validateDocumentIDs(candidates, nil); err != nil {
		return err
	}
	for name, raw := range candidates {
		p.write(name, raw)
	}
	return nil
}
