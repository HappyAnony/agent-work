// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

type htmlPasteContext struct {
	preparation *markdownImportPreparation
	container   string
	windows     bool
}

func captureHTMLPaste(ctx context.Context) (*htmlPasteContext, error) {
	e, err := CaptureArchiveImport(ctx)
	if err != nil {
		return nil, err
	}
	var options nativeparser.EditorOptions
	var container string
	err = e.binding.scope.commit.r.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if data.System == nil {
			return errors.New("html_paste_system_settings_missing")
		}
		container = data.System.Container
		var err error
		options, err = data.ParserOptions(e.binding.scope.commit.r.owner.customEmojis.Snapshot())
		return err
	})
	if err != nil {
		return nil, err
	}
	m := newMarkdownImportPreparation(ctx, options, e.guard)
	m.archive = e
	m.language = e.language
	return &htmlPasteContext{preparation: m, container: container, windows: gulu.OS.IsWindows()}, m.guard()
}
func HTMLPasteBlockDOM(ctx context.Context, html string) (string, error) {
	p, err := captureHTMLPaste(ctx)
	if err != nil {
		return "", err
	}
	return p.convert(html)
}

// The clipboard explicitly supplies each absolute local source. That selected
// file/directory gets its own borrowed root; relative and network URLs stay literal.
func (p *htmlPasteContext) copyLocal(a *markdownImportAssets, dest string) (name string, err error) {
	if err = p.preparation.guard(); err != nil {
		return "", err
	}
	if dest == "" || strings.HasPrefix(dest, "http") {
		return "", nil
	}
	local := strings.TrimPrefix(dest, "file://")
	if p.windows {
		local = strings.TrimPrefix(local, "/")
	}
	decoded, decodeErr := url.PathUnescape(local)
	if decodeErr != nil {
		return "", nil
	} // Preserve unresolvable URL spelling.
	local = decoded
	if !filepath.IsAbs(local) {
		return "", nil
	}
	source, err := p.preparation.openLocalSource(local)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, source.close())
		if err != nil {
			name = ""
		}
	}()
	owned := *a
	owned.source = source
	base := filepath.Base(source.local)
	ext := filepath.Ext(base)
	// Clipboard copies have always allocated a fresh suffix, even for sources
	// whose name already ends in an ID. Markdown import's naming stays separate.
	fresh := strings.TrimSuffix(base, ext) + "-" + ast.NewNodeID() + ext
	return owned.copy(source.local, "assets", fresh)
}
func (p *htmlPasteContext) convert(html string) (output string, err error) {
	if p == nil || p.preparation == nil || p.preparation.archive == nil {
		return "", errors.New("html_paste_context_unbound")
	}
	m := p.preparation
	if err = m.guard(); err != nil {
		return "", err
	}
	err = m.archive.withImageConversionAssets(func(a *markdownImportAssets) error {
		engine := nativeparser.NewLute(m.options.Markdown)
		engine.SetHTMLTag2TextMark(true)
		engine.SetHTML2MarkdownAttrs([]string{"alias", "memo", "bookmark", "custom-*"})
		tree, withMath, err := a.htmlTree(html, engine)
		if err != nil {
			return err
		}
		var copyAsset func(string) (string, error)
		if p.container == util.ContainerStd {
			copyAsset = func(dest string) (string, error) { return p.copyLocal(a, dest) }
		}
		output, err = docview.FormatHTMLPaste(engine, tree, withMath, a.guard, copyAsset)
		return err
	})
	if err != nil {
		return "", err
	}
	return output, nil
}
