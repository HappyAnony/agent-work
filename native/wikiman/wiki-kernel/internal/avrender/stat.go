// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"bytes"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
)

type BlockStatResult struct{ RuneCount, WordCount, LinkCount, ImageCount, RefCount, BlockCount int }

func ContentStat(content string, p *lute.Lute) *BlockStatResult {
	tree := p.BlockDOM2Tree(content)
	r, w, l, i, f := tree.Root.Stat()
	return &BlockStatResult{RuneCount: r, WordCount: w, LinkCount: l, ImageCount: i, RefCount: f}
}
func StatTree(tree *parse.Tree, luteEngine *lute.Lute, read func(string) (*av.AttributeView, error)) (ret *BlockStatResult, err error) {
	blockCount := 0
	var databaseBlockNodes []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || tree.Root == n {
			return ast.WalkContinue
		}

		if n.IsBlock() {
			blockCount++
		}

		if ast.NodeAttributeView != n.Type {
			return ast.WalkContinue
		}

		databaseBlockNodes = append(databaseBlockNodes, n)
		return ast.WalkContinue
	})

	var dbRuneCnt, dbWordCnt, dbLinkCnt, dbImgCnt, dbRefCnt int
	for _, n := range databaseBlockNodes {
		if "" == n.AttributeViewID {
			continue
		}

		attrView, readErr := read(n.AttributeViewID)
		if readErr != nil {
			return nil, readErr
		}
		if nil == attrView {
			continue
		}

		content := bytes.Buffer{}
		for _, kValues := range attrView.KeyValues {
			for _, v := range kValues.Values {
				switch kValues.Key.Type {
				case av.KeyTypeURL:
					if v.IsBlank() {
						continue
					}

					dbLinkCnt++
					content.WriteString(v.URL.Content)
				case av.KeyTypeMAsset:
					if v.IsBlank() {
						continue
					}

					for _, asset := range v.MAsset {
						if av.AssetTypeImage == asset.Type {
							dbImgCnt++
						}
					}
				case av.KeyTypeBlock:
					if v.IsBlank() {
						continue
					}

					if !v.IsDetached {
						dbRefCnt++
					}
					content.WriteString(v.Block.Content)
				case av.KeyTypeText:
					if v.IsBlank() {
						continue
					}
					content.WriteString(v.Text.Content)
				case av.KeyTypeNumber:
					if v.IsBlank() {
						continue
					}
					v.Number.FormatNumber()
					content.WriteString(v.Number.FormattedContent)
				case av.KeyTypeEmail:
					if v.IsBlank() {
						continue
					}
					content.WriteString(v.Email.Content)
				case av.KeyTypePhone:
					if v.IsBlank() {
						continue
					}
					content.WriteString(v.Phone.Content)
				}
			}
		}

		dbStat := ContentStat(content.String(), luteEngine)
		dbRuneCnt += dbStat.RuneCount
		dbWordCnt += dbStat.WordCount
	}

	runeCnt, wordCnt, linkCnt, imgCnt, refCnt := tree.Root.Stat()
	runeCnt += dbRuneCnt
	wordCnt += dbWordCnt
	linkCnt += dbLinkCnt
	imgCnt += dbImgCnt
	refCnt += dbRefCnt
	return &BlockStatResult{
		RuneCount:  runeCnt,
		WordCount:  wordCnt,
		LinkCount:  linkCnt,
		ImageCount: imgCnt,
		RefCount:   refCnt,
		BlockCount: blockCount,
	}, nil
}
