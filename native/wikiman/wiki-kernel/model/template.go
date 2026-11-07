// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"io/fs"
	"os"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/markup"
)

func RenderGoTemplate(source string) (string, error) {
	return RenderGoTemplateContext(context.Background(), source)
}

func RemoveTemplate(p string) (err error) {
	err = RemoveTemplateContext(context.Background(), p)
	if err != nil {
		logging.LogErrorf("remove template failed: %s", err)
	}
	return
}

func SearchTemplate(keyword string) (ret []*Block) {
	ret, err := SearchTemplateContext(context.Background(), keyword)
	if err != nil {
		logging.LogErrorf("search templates failed: %s", err)
		return []*Block{}
	}
	return ret
}

func searchTemplateCatalog(ctx context.Context, templates, keyword string, caseSensitive bool, readDir func(string) ([]os.DirEntry, error), walk func(string, fs.WalkDirFunc) error) (ret []*Block, err error) {
	return catalogue.SearchTemplates(ctx, templates, keyword, caseSensitive, readDir, walk)
}

func DocSaveAsTemplate(id, name string, overwrite bool) (int, error) {
	return DocSaveAsTemplateContext(context.Background(), id, name, overwrite)
}

func RenderDynamicIconContentTemplate(source, id string) string {
	out, err := RenderDynamicIconContentTemplateContext(context.Background(), source, id)
	if err != nil {
		logging.LogErrorf("render icon template: %s", err)
	}
	return out
}

func RenderTemplate(p, id string, preview bool) (*parse.Tree, string, error) {
	return RenderTemplateContext(context.Background(), p, id, preview)
}

func addBlockIALNodes(tree *parse.Tree, removeUpdated bool) {
	if err := markup.AddBlockIALNodes(context.Background(), tree, removeUpdated); err != nil {
		logging.LogErrorf("template block IAL: %s", err)
	}
}
