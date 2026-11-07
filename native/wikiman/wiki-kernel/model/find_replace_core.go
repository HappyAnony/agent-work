// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"regexp"
	"strings"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Original FindReplace traversal; source/history/query/notification ownership is
// supplied explicitly. Native writes are candidates, not R04 source receipts.
func executeFindReplace(q findReplaceRequest, e *findReplaceEnvironment) (err error) {
	keyword, replacement, replaceTypes, method := q.Keyword, q.Replacement, q.ReplaceTypes, q.Method
	r, _ := regexp.Compile(keyword)
	escapedKey := util.EscapeHTML(keyword)
	escapedKey = strings.ReplaceAll(escapedKey, "&#34;", "&quot;")
	escapedKey = strings.ReplaceAll(escapedKey, "&#39;", "'")
	escapedR, _ := regexp.Compile(escapedKey)
	ids := uniqueUpdatedAttributeViews(q.IDs)
	var renameRoots []*ast.Node
	renameRootTitles := map[string]string{}
	cachedTrees := map[string]*parse.Tree{}
	var historyID string
	if err = e.guard(func() (err error) { historyID, err = e.beginHistory(); return }); err != nil {
		return
	}
	if historyID == "" {
		return errors.New("invalid_replace_history")
	}
	if len(ids) == 0 {
		if err = e.guard(func() (err error) { ids, err = e.search(copyFindReplaceRequest(q)); return }); err != nil {
			return
		}
	}
	for _, id := range ids {
		var bt *treenode.BlockTree
		if err = e.guard(func() (err error) { bt, err = e.lookup(id); return }); err != nil {
			return
		}
		if bt == nil {
			continue
		}
		if !e.legacy && (bt.ID != id || bt.RootID == "") {
			return errors.New("invalid_replace_blocktree")
		}
		if cachedTrees[bt.RootID] != nil {
			continue
		}
		var tree *parse.Tree
		if err = e.guard(func() (err error) { tree, err = e.load(id); return }); err != nil {
			if !e.legacy {
				return
			}
			err = nil
		}
		if tree == nil {
			continue
		}
		if tree.Root == nil || (!e.legacy && (tree.ID != bt.RootID || tree.Root.ID != tree.ID || tree.Box == "" || tree.Path == "")) {
			return errors.New("invalid_replace_tree")
		}
		if err = e.guard(func() error { return e.history(historyID, tree) }); err != nil {
			return
		}
		cachedTrees[bt.RootID] = tree
	}
	if err = e.guard(func() error { return e.indexHistory(historyID) }); err != nil {
		return
	}
	var luteEngineValueError error
	luteEngine, luteEngineValueError := e.newParser()
	if luteEngineValueError != nil {
		return luteEngineValueError
	}
	if err = e.ctx.Err(); err != nil {
		return
	}
	if luteEngine == nil {
		return errors.New("replace_parser_unbound")
	}
	var reloadTreeIDs []string
	updateNodes := map[string]*ast.Node{}
	for i, id := range ids {
		var bt *treenode.BlockTree
		if err = e.guard(func() (err error) { bt, err = e.lookup(id); return }); err != nil {
			return
		}
		if bt == nil {
			continue
		}
		if !e.legacy && (bt.ID != id || bt.RootID == "") {
			return errors.New("invalid_replace_blocktree")
		}
		tree := cachedTrees[bt.RootID]
		if tree == nil {
			continue
		}
		node := treenode.GetNodeInTree(tree, id)
		if node == nil {
			continue
		}
		reloadTreeIDs = append(reloadTreeIDs, tree.ID)
		if ast.NodeDocument == node.Type {
			if !replaceTypes["docTitle"] {
				continue
			}

			title := node.IALAttr("title")
			tags := node.IALAttr("tags")
			if 0 == method {
				if strings.Contains(title, keyword) {
					docTitleReplacement := strings.ReplaceAll(replacement, "/", "／")
					renameRootTitles[node.ID] = strings.ReplaceAll(title, keyword, docTitleReplacement)
					renameRoots = append(renameRoots, node)
				}

				if strings.Contains(tags, keyword) {
					replacement = strings.TrimPrefix(replacement, "#")
					replacement = strings.TrimSuffix(replacement, "#")
					tags = strings.ReplaceAll(tags, keyword, replacement)
					tags = strings.ReplaceAll(tags, editor.Zwsp, "")
					node.SetIALAttr("tags", tags)
					if err = e.guard(e.tags); err != nil {
						return err
					}
				}
			} else if 3 == method {
				if nil != r && r.MatchString(title) {
					docTitleReplacement := strings.ReplaceAll(replacement, "/", "／")
					renameRootTitles[node.ID] = r.ReplaceAllString(title, docTitleReplacement)
					renameRoots = append(renameRoots, node)
				}

				if nil != r && r.MatchString(tags) {
					replacement = strings.TrimPrefix(replacement, "#")
					replacement = strings.TrimSuffix(replacement, "#")
					tags = r.ReplaceAllString(tags, replacement)
					tags = strings.ReplaceAll(tags, editor.Zwsp, "")
					node.SetIALAttr("tags", tags)
					if err = e.guard(e.tags); err != nil {
						return err
					}
				}
			}
		} else {
			var unlinks []*ast.Node
			ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
				if err = e.ctx.Err(); err != nil {
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}

				switch n.Type {
				case ast.NodeText:
					if !replaceTypes["text"] {
						return ast.WalkContinue
					}

					if replaceTextNode(n, method, keyword, replacement, r, luteEngine, e.caseSensitive) {
						if nil != n.Parent && ast.NodeBackslash == n.Parent.Type {
							unlinks = append(unlinks, n.Parent)

							prev, next := n.Parent.Previous, n.Parent.Next
							for ; prev != nil && ((ast.NodeText == prev.Type && prev.Tokens == nil) || ast.NodeBackslash == prev.Type); prev = prev.Previous {
								// Tokens 为空的节点或者转义节点之前已经处理，需要跳过
							}
							if nil != prev && ast.NodeText == prev.Type && nil != next && ast.NodeText == next.Type {
								prev.Tokens = append(prev.Tokens, next.Tokens...)
								next.Tokens = nil // 将 Tokens 设置为空，表示该节点已经被处理过
								unlinks = append(unlinks, next)
							}
						} else {
							unlinks = append(unlinks, n)
						}
					}
				case ast.NodeLinkDest:
					if !replaceTypes["imgSrc"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, strings.TrimSpace(replacement), r)
					if 1 > len(n.Tokens) {
						unlinks = append(unlinks, n.Parent)
						mergeSamePreNext(n)
					}
				case ast.NodeLinkText:
					if !replaceTypes["imgText"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, replacement, r)
				case ast.NodeLinkTitle:
					if !replaceTypes["imgTitle"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, replacement, r)
				case ast.NodeCodeBlockCode:
					if !replaceTypes["codeBlock"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, replacement, r)
				case ast.NodeMathBlockContent:
					if !replaceTypes["mathBlock"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, replacement, r)
				case ast.NodeHTMLBlock:
					if !replaceTypes["htmlBlock"] {
						return ast.WalkContinue
					}

					replaceNodeTokens(n, method, keyword, replacement, r)
				case ast.NodeTextMark:
					if n.IsTextMarkType("code") {
						if !replaceTypes["code"] {
							return ast.WalkContinue
						}

						if 0 == method {
							if strings.Contains(n.TextMarkTextContent, escapedKey) {
								n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, escapedKey, util.EscapeHTML(replacement))
							}
						} else if 3 == method {
							if nil != escapedR && escapedR.MatchString(n.TextMarkTextContent) {
								n.TextMarkTextContent = escapedR.ReplaceAllString(n.TextMarkTextContent, util.EscapeHTML(replacement))
							}
						}

						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("a") {
						if replaceTypes["aText"] {
							if 0 == method {
								content := util.UnescapeHTML(n.TextMarkTextContent)
								if strings.Contains(content, escapedKey) {
									n.TextMarkTextContent = strings.ReplaceAll(content, escapedKey, replacement)
								} else if strings.Contains(content, keyword) {
									n.TextMarkTextContent = strings.ReplaceAll(content, keyword, replacement)
								}
							} else if 3 == method {
								if nil != r && r.MatchString(n.TextMarkTextContent) {
									n.TextMarkTextContent = r.ReplaceAllString(n.TextMarkTextContent, replacement)
								}
							}
							if "" == n.TextMarkTextContent {
								unlinks = append(unlinks, n)
								mergeSamePreNext(n)
							}
						}

						if replaceTypes["aTitle"] {
							if 0 == method {
								title := util.UnescapeHTML(n.TextMarkATitle)
								if strings.Contains(title, escapedKey) {
									n.TextMarkATitle = strings.ReplaceAll(title, escapedKey, replacement)
								} else if strings.Contains(n.TextMarkATitle, keyword) {
									n.TextMarkATitle = strings.ReplaceAll(title, keyword, replacement)
								}
							} else if 3 == method {
								if nil != r && r.MatchString(n.TextMarkATitle) {
									n.TextMarkATitle = r.ReplaceAllString(n.TextMarkATitle, replacement)
								}
							}
						}

						if replaceTypes["aHref"] {
							if 0 == method {
								href := util.UnescapeHTML(n.TextMarkAHref)
								if strings.Contains(href, escapedKey) {
									n.TextMarkAHref = strings.ReplaceAll(href, escapedKey, util.EscapeHTML(replacement))
								} else if strings.Contains(href, keyword) {
									n.TextMarkAHref = strings.ReplaceAll(href, keyword, strings.TrimSpace(replacement))
								}
							} else if 3 == method {
								if nil != r && r.MatchString(n.TextMarkAHref) {
									n.TextMarkAHref = r.ReplaceAllString(n.TextMarkAHref, strings.TrimSpace(replacement))
								}
							}

							if "" == n.TextMarkAHref {
								if "" == n.TextMarkTextContent {
									unlinks = append(unlinks, n)
									mergeSamePreNext(n)
								} else {
									n.Type = ast.NodeText
									n.Tokens = []byte(n.TextMarkTextContent)
								}
							}
						}
					} else if n.IsTextMarkType("em") {
						if !replaceTypes["em"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "em", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("strong") {
						if !replaceTypes["strong"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "strong", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("kbd") {
						if !replaceTypes["kbd"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "kbd", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("mark") {
						if !replaceTypes["mark"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "mark", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("s") {
						if !replaceTypes["s"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "s", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("sub") {
						if !replaceTypes["sub"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "sub", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("sup") {
						if !replaceTypes["sup"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "sup", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("tag") {
						if !replaceTypes["tag"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "tag", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}

						if err = e.guard(e.tags); err != nil {
							return ast.WalkStop
						}
					} else if n.IsTextMarkType("u") {
						if !replaceTypes["u"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "u", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("inline-math") {
						if !replaceTypes["inlineMath"] {
							return ast.WalkContinue
						}

						if 0 == method {
							if strings.Contains(n.TextMarkInlineMathContent, keyword) {
								n.TextMarkInlineMathContent = strings.ReplaceAll(n.TextMarkInlineMathContent, keyword, replacement)
							}
						} else if 3 == method {
							if nil != r && r.MatchString(n.TextMarkInlineMathContent) {
								n.TextMarkInlineMathContent = r.ReplaceAllString(n.TextMarkInlineMathContent, replacement)
							}
						}

						if "" == n.TextMarkInlineMathContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("inline-memo") {
						if !replaceTypes["inlineMemo"] {
							return ast.WalkContinue
						}

						if 0 == method {
							if strings.Contains(n.TextMarkInlineMemoContent, keyword) {
								n.TextMarkInlineMemoContent = strings.ReplaceAll(n.TextMarkInlineMemoContent, keyword, replacement)
								n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, keyword, replacement)
							}
						} else if 3 == method {
							if nil != r && r.MatchString(n.TextMarkInlineMemoContent) {
								n.TextMarkInlineMemoContent = r.ReplaceAllString(n.TextMarkInlineMemoContent, replacement)
								n.TextMarkTextContent = r.ReplaceAllString(n.TextMarkTextContent, replacement)
							}
						}

						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("text") {
						// Search and replace fails in some cases https://github.com/siyuan-note/siyuan/issues/10016
						if !replaceTypes["text"] {
							return ast.WalkContinue
						}

						if err = e.guard(func() error {
							return replaceNodeTextMarkTextContent(n, method, keyword, escapedKey, replacement, r, "text", luteEngine, func(id string) error { return e.guard(func() error { return e.refCount(id) }) })
						}); err != nil {
							return ast.WalkStop
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
							mergeSamePreNext(n)
						}
					} else if n.IsTextMarkType("block-ref") {
						if !replaceTypes["blockRef"] {
							return ast.WalkContinue
						}

						if 0 == method {
							if strings.Contains(n.TextMarkTextContent, keyword) {
								n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, keyword, replacement)
								n.TextMarkBlockRefSubtype = "s"
							}
						} else if 3 == method {
							if nil != r && r.MatchString(n.TextMarkTextContent) {
								n.TextMarkTextContent = r.ReplaceAllString(n.TextMarkTextContent, replacement)
								n.TextMarkBlockRefSubtype = "s"
							}
						}

						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					} else if n.IsTextMarkType("file-annotation-ref") {
						if !replaceTypes["fileAnnotationRef"] {
							return ast.WalkContinue
						}

						if 0 == method {
							if strings.Contains(n.TextMarkTextContent, keyword) {
								n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, keyword, replacement)
							}
						} else if 3 == method {
							if nil != r && r.MatchString(n.TextMarkTextContent) {
								n.TextMarkTextContent = r.ReplaceAllString(n.TextMarkTextContent, replacement)
							}
						}
						if "" == n.TextMarkTextContent {
							unlinks = append(unlinks, n)
						}
					}
				}
				return ast.WalkContinue
			})

			if err != nil {
				return err
			}
			for _, unlink := range unlinks {
				unlink.Unlink()
			}
		}

		if err = e.guard(func() error { return e.stage(tree) }); err != nil {
			return
		}
		updateNodes[id] = node
		if err = e.guard(func() error { return e.progress("replace", i+1, len(ids)) }); err != nil {
			return
		}
	}
	for i, n := range renameRoots {
		tree := cachedTrees[n.ID]
		if tree == nil {
			return errors.New("replace_rename_tree_missing")
		}
		if err = e.guard(func() error { return e.rename(tree, renameRootTitles[n.ID]) }); err != nil {
			return
		}
		if err = e.guard(func() error { return e.progress("rename", i+1, len(renameRoots)) }); err != nil {
			return
		}
	}
	if err = e.guard(e.flush); err != nil {
		return
	}
	for _, id := range uniqueUpdatedAttributeViews(reloadTreeIDs) {
		if err = e.guard(func() error { return e.reload(id) }); err != nil {
			return
		}
	}
	// Reload may have staged a newer tree containing propagated dynamic refs.
	// Project AV text from that owner state, not the earlier replacement AST.
	if updateNodes, err = latestFindReplaceNodes(updateNodes, e); err != nil {
		return
	}
	if err = updateAttributeViewBlockTextWithEnvironment(updateNodes, e.av); err != nil {
		if !e.legacy || e.ctx.Err() != nil {
			return
		}
		logging.LogErrorf("refresh AV text failed: %s", err)
		err = nil // The original void legacy helper logged and continued.
	}
	if err = e.guard(e.flush); err != nil {
		return
	}
	return e.guard(func() error { return e.clear(true) })
}

func latestFindReplaceNodes(nodes map[string]*ast.Node, e *findReplaceEnvironment) (map[string]*ast.Node, error) {
	trees := map[string]*parse.Tree{}
	ret := map[string]*ast.Node{}
	for id := range nodes {
		var bt *treenode.BlockTree
		if err := e.guard(func() (err error) { bt, err = e.lookup(id); return }); err != nil {
			return nil, err
		}
		if bt == nil {
			continue
		}
		if !e.legacy && (bt.ID != id || bt.RootID == "") {
			return nil, errors.New("invalid_replace_blocktree")
		}
		tree := trees[bt.RootID]
		if tree == nil {
			if err := e.guard(func() (err error) { tree, err = e.load(id); return }); err != nil {
				if !e.legacy || e.ctx.Err() != nil {
					return nil, err
				}
				logging.LogWarnf("reload replacement projection failed: %s", err)
				continue
			}
			if tree == nil || tree.Root == nil || (!e.legacy && (tree.ID != bt.RootID || tree.Root.ID != tree.ID || tree.Box == "" || tree.Path == "")) {
				return nil, errors.New("invalid_replace_tree")
			}
			trees[bt.RootID] = tree
		}
		if n := treenode.GetNodeInTree(tree, id); n != nil {
			ret[id] = n
		}
	}
	return ret, nil
}
