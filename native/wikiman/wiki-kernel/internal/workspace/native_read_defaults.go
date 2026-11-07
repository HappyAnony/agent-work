// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io/fs"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/notebooks"
)

func defaultReadConfiguration() conf.AppData {
	return conf.AppData{Lang: "en_US", Langs: conf.PortableLanguages(), Appearance: conf.NewAppearance(), Editor: conf.NewEditor(), Search: conf.NewSearch(), Export: conf.NewExport(), FileTree: conf.NewFileTree(), Tag: conf.NewTag(), Graph: conf.NewGraph(), Flashcard: conf.NewFlashcard()}
}

func bindNativeReadServices(w *WorkspaceContext) {
	r := w.resources
	if r.configurationBusiness == nil {
		r.configurationBusiness = conf.NewReadBusiness(func(ctx context.Context) ([]conf.Notebook, error) {
			if w.data == nil {
				return []conf.Notebook{}, ctx.Err()
			}
			order := 0
			if r.configuration.FileTree != nil {
				order = r.configuration.FileTree.Sort
			}
			plan, err := notebooks.Scan(ctx, w.data, "auto", order)
			if err != nil {
				return nil, err
			}
			for _, repair := range plan.Repairs {
				if !repair.Missing {
					return nil, errors.New("invalid_notebook_metadata")
				}
			}
			return plan.Notebooks, err // Repairs are deliberately never applied on reads.
		})
	}
	if r.nativeContent.AssetText == nil {
		r.nativeContent.AssetText = func(ctx context.Context, p string) (string, error) {
			raw, e := readNativeFile(ctx, w, "assets/ocr-texts.json", 16<<20)
			if errors.Is(e, fs.ErrNotExist) {
				return "", ctx.Err()
			}
			if e != nil {
				return "", e
			}
			var texts map[string]string
			if e = json.Unmarshal(raw, &texts); e != nil {
				return "", e
			}
			return texts[p], ctx.Err()
		}
	}
	if r.nativeContent.AttributeView == nil {
		r.nativeContent.AttributeView = func(ctx context.Context, id string, full bool) (string, error) {
			if !full {
				name, _, err := r.readNativeAV(ctx, w, id)
				return name, err
			}
			raw, _, err := readNativeAVSource(ctx, w, id)
			if err != nil {
				return "", err
			}
			a, err := decodeNativeAV(raw, id, time.Now())
			if err != nil {
				return "", err
			}
			if a == nil || a.ID != id {
				return "", ErrDocument
			}
			return avrender.SearchText(ctx, a)
		}
	}
	if r.nativeOCR == nil {
		r.nativeOCR = func(ctx context.Context, _ *ast.Node) error { return ctx.Err() }
	}
}
