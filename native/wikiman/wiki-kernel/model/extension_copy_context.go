// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/gabriel-vasile/mimetype"
	"github.com/siyuan-note/httpclient"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

type ExtensionCopyResult struct {
	Markdown string
	WithMath bool
	Message  string
}

// All ports are private and captured per invocation. This remains the original
// compatibility owner, not a Native multi-workspace publication transaction.
type extensionCopyContext struct {
	preparation *markdownImportPreparation
	openUpload  func(*multipart.FileHeader) (io.ReadCloser, error)
	removeForm  func(*multipart.Form) error
	fetch       func(context.Context, string) (io.ReadCloser, error)
}

func captureExtensionCopy(ctx context.Context) (*extensionCopyContext, error) {
	m, err := captureMarkdownImportPreparation(ctx)
	if err != nil {
		return nil, err
	}
	return &extensionCopyContext{preparation: m,
		openUpload: func(h *multipart.FileHeader) (io.ReadCloser, error) { return h.Open() },
		removeForm: func(f *multipart.Form) error { return f.RemoveAll() },
		fetch: func(ctx context.Context, address string) (io.ReadCloser, error) {
			response, err := httpclient.NewCloudRequest30s().SetContext(ctx).DisableAutoReadResponse().Get(address)
			if err != nil {
				if response != nil && response.Response != nil {
					return response.Body, err
				}
				return nil, err
			}
			if response == nil || response.Response == nil || response.Body == nil {
				return nil, errors.New("extension_response_body_missing")
			}
			return response.Body, nil
		},
	}, nil
}
func CopyExtensionRequest(ctx context.Context, request *http.Request, maxMemory int64) (ExtensionCopyResult, error) {
	p, err := captureExtensionCopy(ctx)
	if err != nil {
		return ExtensionCopyResult{}, err
	}
	return p.copyRequest(request, maxMemory)
}

type extensionGuardedBody struct {
	io.ReadCloser
	guard func() error
}

func (r extensionGuardedBody) Read(b []byte) (int, error) {
	if err := r.guard(); err != nil {
		return 0, err
	}
	n, err := r.ReadCloser.Read(b)
	if check := r.guard(); check != nil {
		return 0, errors.Join(err, check)
	}
	return n, err
}
func (p *extensionCopyContext) copyRequest(request *http.Request, maxMemory int64) (result ExtensionCopyResult, err error) {
	if p == nil || p.preparation == nil || request == nil {
		return result, errors.New("extension_request_unbound")
	}
	m := p.preparation
	if err = m.guard(); err != nil {
		return result, err
	}
	// Cleanup happens after the target root closes, and before success is exposed.
	// net/http still owns the request body itself; only multipart scratch is ours.
	defer func() {
		if request.MultipartForm != nil {
			err = errors.Join(err, p.removeForm(request.MultipartForm))
		}
		if err == nil {
			err = m.guard()
		}
		if err != nil {
			result = ExtensionCopyResult{}
		}
	}()
	err = m.archive.withImageConversionAssets(func(a *markdownImportAssets) error {
		original := request.Body
		if original == nil {
			return errors.New("extension_body_missing")
		}
		request.Body = extensionGuardedBody{ReadCloser: original, guard: a.guard}
		parseErr := request.ParseMultipartForm(maxMemory)
		request.Body = original
		if parseErr != nil {
			return parseErr
		}
		if err := a.guard(); err != nil {
			return err
		}
		var err error
		result, err = p.copyForm(a, request.MultipartForm)
		return err
	})
	return
}
func extensionField(form *multipart.Form, key string) string {
	if form == nil || len(form.Value[key]) == 0 {
		return ""
	}
	return form.Value[key][0]
}
func (a *markdownImportAssets) extensionDirectory(notebook string) (string, error) {
	if err := a.guard(); err != nil {
		return "", err
	}
	folder := "assets"
	if notebook != "" {
		if !ast.IsNodeIDPattern(notebook) {
			return "", indexrepair.ErrSourcePath
		}
		candidate := path.Join(notebook, "assets")
		if err := a.target.Check(candidate, true); err != nil {
			return "", err
		}
		info, err := a.target.Data.Lstat(candidate)
		if err == nil && info.IsDir() {
			folder = candidate
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := a.target.Check(folder, true); err != nil {
		return "", err
	}
	if err := a.target.Data.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	return folder, a.guard()
}
func (p *extensionCopyContext) readAndClose(input io.ReadCloser) (data []byte, err error) {
	defer func() {
		err = errors.Join(err, input.Close())
		if err != nil {
			data = nil
		}
	}()
	var body bytes.Buffer
	err = archiveCopy(p.preparation.ctx, markdownAssetWriter{Writer: &body, guard: p.preparation.guard}, extensionGuardedBody{ReadCloser: input, guard: p.preparation.guard})
	if err != nil {
		return nil, err
	}
	return body.Bytes(), p.preparation.guard()
}
func (p *extensionCopyContext) upload(a *markdownImportAssets, folder string, header *multipart.FileHeader, name string) (string, error) {
	if err := a.guard(); err != nil {
		return "", err
	}
	input, err := p.openUpload(header)
	if err != nil {
		return "", err
	}
	// Preserve the original whole-file MIME/SVG detection policy. Reads and target
	// publication are checked; this does not claim bounded-memory transcoding.
	data, err := p.readAndClose(input)
	if err != nil {
		return "", err
	}
	name = util.FilterUploadFileName(name)
	ext := util.Ext(name)
	if !util.IsCommonExt(ext) || strings.Contains(ext, "!") {
		if typ := mimetype.Detect(data); typ != nil {
			ext = typ.Extension()
			name += ext
		}
	}
	if ext == "" && bytes.HasPrefix(data, []byte("<svg ")) && bytes.HasSuffix(data, []byte("</svg>")) {
		name += ".svg"
	}
	name = util.AssetName(name, ast.NewNodeID())
	if err = a.publish(folder, name, bytes.NewReader(data), a.guard, func() error { return nil }); err != nil {
		return "", err
	}
	return "assets/" + name, nil
}
func (p *extensionCopyContext) copyForm(a *markdownImportAssets, form *multipart.Form) (result ExtensionCopyResult, err error) {
	if form == nil || len(form.Value["dom"]) == 0 {
		return result, errors.New("extension_dom_missing")
	}
	dom := extensionField(form, "dom")
	folder, err := a.extensionDirectory(extensionField(form, "notebook"))
	if err != nil {
		return result, err
	}
	clippingSym := false
	symArticleHref := ""
	hasHref := len(form.Value["href"]) > 0
	isPartClip := extensionField(form, "clipType") == "part"
	if hasHref && !isPartClip {
		// 剪藏链滴帖子时直接使用 Markdown 接口的返回
		// https://ld246.com/article/raw/1724850322251
		symArticleHref = form.Value["href"][0]

		var baseURL, originalPrefix string
		if strings.HasPrefix(symArticleHref, "https://ld246.com/article/") {
			baseURL = "https://ld246.com/article/raw/"
			originalPrefix = "https://ld246.com/article/"
		} else if strings.HasPrefix(symArticleHref, "https://liuyun.io/article/") {
			baseURL = "https://liuyun.io/article/raw/"
			originalPrefix = "https://liuyun.io/article/"
		}

		if "" != baseURL {
			articleID := strings.TrimPrefix(symArticleHref, originalPrefix)
			if idx := strings.IndexAny(articleID, "/?#"); -1 != idx {
				articleID = articleID[:idx]
			}

			symArticleHref = baseURL + articleID
			clippingSym = true
		}
	}
	uploaded := map[string]string{}
	for originalName, file := range form.File {
		oName, err := url.PathUnescape(originalName)
		unescaped := oName

		if clippingSym && strings.Contains(oName, "img-loading.svg") {
			continue
		}

		if err != nil {
			if strings.Contains(originalName, "%u") {
				originalName = strings.ReplaceAll(originalName, "%u", "\\u")
				originalName, err = strconv.Unquote("\"" + originalName + "\"")
				if err != nil {
					continue
				}
				oName, err = url.PathUnescape(originalName)
				if err != nil {
					continue
				}
			} else {
				continue
			}
		}
		if strings.Contains(oName, "%") {
			unescaped, _ := url.PathUnescape(oName)
			if "" != unescaped {
				oName = unescaped
			}
		}

		u, parseErr := url.Parse(oName)
		if parseErr != nil || u == nil || "" == u.Path {
			continue
		}
		fName := path.Base(u.Path)

		if len(file) == 0 {
			continue
		}
		if err = a.guard(); err != nil {
			return result, err
		}
		uploaded[unescaped], err = p.upload(a, folder, file[0], fName)
		if err != nil {
			return result, err
		}
	}

	luteEngine := nativeparser.NewLute(p.preparation.options.Markdown)
	luteEngine.SetHTMLTag2TextMark(true)
	var md string
	var withMath bool
	if clippingSym {
		if err = a.guard(); err != nil {
			return result, err
		}
		body, fetchErr := p.fetch(p.preparation.ctx, symArticleHref)
		if fetchErr != nil {
			if body != nil {
				return result, errors.Join(fetchErr, body.Close())
			}
			if err = a.guard(); err != nil {
				return result, errors.Join(fetchErr, err)
			}
			// Original network-failure fallback remains; cancellation never becomes HTML.
			logging.LogWarnf("get [%s] failed: %s", symArticleHref, fetchErr)
		} else {
			if body == nil {
				return result, errors.New("extension_response_body_missing")
			}
			bodyData, readErr := p.readAndClose(body)
			if readErr != nil {
				return result, readErr
			}
			var walkErr error
			md = string(bodyData)
			luteEngine.SetIndentCodeBlock(true) // 链滴支持缩进代码块，因此需要开启
			tree := parse.Parse("", []byte(md), luteEngine.ParseOptions)
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if walkErr = a.guard(); walkErr != nil {
					return ast.WalkStop
				}
				if ast.NodeInlineMath == n.Type {
					withMath = true
					return ast.WalkStop
				} else if ast.NodeCodeBlock == n.Type {
					if !n.IsFencedCodeBlock {
						// 将缩进代码块转换为围栏代码块
						n.IsFencedCodeBlock = true
						n.CodeBlockFenceChar = '`'
						n.PrependChild(&ast.Node{Type: ast.NodeCodeBlockFenceInfoMarker})
						n.PrependChild(&ast.Node{Type: ast.NodeCodeBlockFenceOpenMarker, Tokens: []byte("```"), CodeBlockFenceLen: 3})
						n.LastChild.InsertAfter(&ast.Node{Type: ast.NodeCodeBlockFenceCloseMarker, Tokens: []byte("```"), CodeBlockFenceLen: 3})
						code := n.ChildByType(ast.NodeCodeBlockCode)
						if nil != code {
							code.Tokens = bytes.TrimPrefix(code.Tokens, []byte("    "))
							code.Tokens = bytes.ReplaceAll(code.Tokens, []byte("\n    "), []byte("\n"))
							code.Tokens = bytes.TrimPrefix(code.Tokens, []byte("\t"))
							code.Tokens = bytes.ReplaceAll(code.Tokens, []byte("\n\t"), []byte("\n"))
						}
					}
				}
				return ast.WalkContinue
			})

			if walkErr != nil {
				return result, walkErr
			}
			md, _ = lute.FormatNodeSync(tree.Root, luteEngine.ParseOptions, luteEngine.RenderOptions)
		}
	}
	var walkErr error
	var tree *parse.Tree
	if "" == md {
		// 通过正则将 <iframe>.*</iframe> 标签中间包含的换行去掉
		regx, _ := regexp.Compile(`(?i)<iframe[^>]*>([\s\S]*?)<\/iframe>`)
		dom = regx.ReplaceAllStringFunc(dom, func(s string) string {
			s = strings.ReplaceAll(s, "\n", "")
			s = strings.ReplaceAll(s, "\r", "")
			return s
		})

		var conversionErr error
		tree, withMath, conversionErr = a.htmlTree(dom, luteEngine)
		if conversionErr != nil {
			return result, conversionErr
		}
	} else {
		tree = parse.Parse("", []byte(md), luteEngine.ParseOptions)
	}

	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if walkErr = a.guard(); walkErr != nil {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeText == n.Type {
			// 剔除行首空白
			if ast.NodeParagraph == n.Parent.Type && n.Parent.FirstChild == n {
				n.Tokens = bytes.TrimLeft(n.Tokens, " \t\n")
			}
		} else if ast.NodeImage == n.Type {
			if dest := n.ChildByType(ast.NodeLinkDest); nil != dest {
				assetPath := uploaded[string(dest.Tokens)]
				if "" == assetPath {
					assetPath = uploaded[string(dest.Tokens)+"?imageView2/2/interlace/1/format/webp"]
				}
				if "" != assetPath {
					dest.Tokens = []byte(assetPath)
				}

				// 检测 alt 和 title 格式，如果不是文本的话转换为文本 https://github.com/siyuan-note/siyuan/issues/14233
				if linkText := n.ChildByType(ast.NodeLinkText); nil != linkText {
					if inlineTree := parse.Inline("", linkText.Tokens, luteEngine.ParseOptions); nil != inlineTree && nil != inlineTree.Root && nil != inlineTree.Root.FirstChild {
						if fc := inlineTree.Root.FirstChild.FirstChild; nil != fc {
							if ast.NodeText != fc.Type {
								linkText.Tokens = []byte(fc.Text())
							}
						}
					}
				}
				if title := n.ChildByType(ast.NodeLinkTitle); nil != title {
					if inlineTree := parse.Inline("", title.Tokens, luteEngine.ParseOptions); nil != inlineTree && nil != inlineTree.Root && nil != inlineTree.Root.FirstChild {
						if fc := inlineTree.Root.FirstChild.FirstChild; nil != fc {
							if ast.NodeText != fc.Type {
								title.Tokens = []byte(fc.Text())
							}
						}
					}
				}
			}
		}
		return ast.WalkContinue
	})
	if walkErr != nil {
		return result, walkErr
	}

	parse.TextMarks2Inlines(tree) // 先将 TextMark 转换为 Inlines https://github.com/siyuan-note/siyuan/issues/13056
	parse.NestedInlines2FlattedSpansHybrid(tree, false)

	md, _ = lute.FormatNodeSync(tree.Root, luteEngine.ParseOptions, luteEngine.RenderOptions)
	if err = a.guard(); err != nil {
		return result, err
	}
	return ExtensionCopyResult{Markdown: md, WithMath: withMath, Message: p.preparation.language(72)}, nil
}
