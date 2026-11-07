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
	"bytes"
	"context"
	"encoding/json"
	"github.com/siyuan-note/logging"
	"strings"
	"time"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/sql"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/dgraph-io/ristretto"
)

// virtualBlockRefCache 用于保存块关联的虚拟引用关键字。
// 改进打开虚拟引用后加载文档的性能 https://github.com/siyuan-note/siyuan/issues/7378
var virtualBlockRefCache, _ = ristretto.NewCache(&ristretto.Config{
	NumCounters: 102400,
	MaxCost:     10240,
	BufferItems: 64,
})

func getBlockVirtualRefKeywords(root *ast.Node) (ret []string) {
	val, ok := virtualBlockRefCache.Get(root.ID)
	if !ok {
		buf := bytes.Buffer{}
		ast.Walk(root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || !n.IsBlock() {
				return ast.WalkContinue
			}

			content := sql.NodeStaticContent(n, nil, false, false, false)
			content = strings.ReplaceAll(content, editor.Zwsp, "")
			buf.WriteString(content)
			return ast.WalkContinue
		})
		content := buf.String()
		ret = putBlockVirtualRefKeywords(content, root)
		return
	}
	ret = val.([]string)
	return
}

func putBlockVirtualRefKeywords(content string, root *ast.Node) []string {
	keys := docview.MatchVirtualKeywords(content, getVirtualRefKeywords(root), Conf.Search.CaseSensitive)
	if len(keys) > 0 {
		virtualBlockRefCache.SetWithTTL(root.ID, keys, 1, 10*time.Minute)
	}
	return keys
}

func CacheVirtualBlockRefJob() {
	logOwnedTaskError("virtual-reference cache admission", CacheVirtualBlockRefJobContext(context.Background()))
}

func CacheVirtualBlockRefJobContext(ctx context.Context) error {
	if Conf == nil || Conf.Editor == nil || !Conf.Editor.VirtualBlockRef {
		return ctx.Err()
	}
	_, err := startVirtualBlockRefCache(ctx)
	return err
}

func ClearVirtualBlockRefCache() { virtualBlockRefCache.Clear() }

func ResetVirtualBlockRefCache() {
	logOwnedTaskError("virtual-reference cache rebuild", ResetVirtualBlockRefCacheContext(context.Background()))
}

func ResetVirtualBlockRefCacheContext(ctx context.Context) error {
	e, err := captureLegacyVirtualCache(ctx)
	if err != nil {
		return err
	}
	return e.rebuild(ctx)
}

// addNewKeywords 将新关键字添加到虚拟引用关键字列表中，如果不存在则追加，保留空白字符
func addNewKeywords(s string, more []string) string { return textprefs.AddKeywords(s, more) }

func AddVirtualBlockRefInclude(keys []string) { addVirtualSetting("virtualInclude", keys) }

func AddVirtualBlockRefExclude(keys []string) { addVirtualSetting("virtualExclude", keys) }

func processVirtualRef(n *ast.Node, unlinks *[]*ast.Node, keys []string, refs map[string]int, parser *lute.Lute) bool {
	ok, _ := docview.ProcessVirtual(context.Background(), n, unlinks, keys, refs, parser, docview.VirtualOptions{Enabled: Conf.Editor.VirtualBlockRef, CaseSensitive: Conf.Search.CaseSensitive}, func(_ context.Context, n *ast.Node) (string, error) { return getNodeRefText(n), nil })
	return ok
}
func parseKeywords(s string) []string { return docview.ParseKeywords(s) }

func getVirtualRefKeywords(root *ast.Node) []string {
	var keys []string
	if val, ok := virtualBlockRefCache.Get("virtual_ref"); ok {
		keys = val.([]string)
	}
	return docview.FilterVirtualKeywords(root, keys, docview.VirtualOptions{Enabled: Conf.Editor.VirtualBlockRef, Include: Conf.Editor.VirtualBlockRefInclude, Exclude: Conf.Editor.VirtualBlockRefExclude})
}

func prepareMarkKeywords(keys []string) []string { return docview.PrepareKeywords(keys) }

func addVirtualSetting(kind string, keys []string) {
	if len(keys) == 0 {
		return
	}
	raw, e := json.Marshal(map[string][]string{"keywords": keys})
	if e == nil {
		_, e = setLegacyConfiguration(context.Background(), kind, raw, false)
	}
	if e != nil {
		logging.LogErrorf("set virtual keywords failed: %s", e)
	}
}
