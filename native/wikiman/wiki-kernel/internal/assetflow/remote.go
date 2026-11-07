// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/gabriel-vasile/mimetype"
	"mime"
	"net/url"
	"path"
	"strings"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/textprefs"
)

type RemoteResponse struct {
	Data        []byte
	ContentType string
	Status      int
}
type RemotePorts struct {
	ReadLocal func(context.Context, string) ([]byte, bool, error)
	Fetch     func(context.Context, string, string) (RemoteResponse, error)
	Write     func(context.Context, string, []byte) error
	ReadAV    func(string) (*av.AttributeView, error)
	SaveAV    func(*av.AttributeView) error
	CommonExt func(string) bool
	NewID     func() string
	Progress  func(string)
}
type RemoteResult struct{ Files, Forbidden, Failed int }

func RemoteDests(node *ast.Node, onlyImg bool, read func(string) (*av.AttributeView, error)) (out []string, err error) {
	if node == nil || read == nil {
		return nil, errors.New("asset_remote_unbound")
	}
	appendDest := func(s string) {
		if !IsAssetLinkDest([]byte(s)) {
			out = append(out, s)
		}
	}
	switch {
	case node.Type == ast.NodeLinkDest:
		if !onlyImg || node.ParentIs(ast.NodeImage) {
			appendDest(string(node.Tokens))
		}
	case node.IsTextMarkType("a"):
		if !onlyImg {
			appendDest(node.TextMarkAHref)
		}
	case node.Type == ast.NodeAudio || node.Type == ast.NodeVideo:
		if !onlyImg {
			appendDest(NodeSrc(node))
		}
	case node.Type == ast.NodeAttributeView:
		a, e := read(node.AttributeViewID)
		if e != nil {
			return nil, e
		}
		if a == nil {
			return out, nil
		}
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil {
				return nil, errors.New("asset_remote_invalid_av")
			}
			if kv.Key.Type != av.KeyTypeMAsset {
				continue
			}
			for _, v := range kv.Values {
				if v == nil {
					return nil, errors.New("asset_remote_invalid_av")
				}
				for _, item := range v.MAsset {
					if item == nil {
						return nil, errors.New("asset_remote_invalid_av")
					}
					if onlyImg && item.Type != av.AssetTypeImage {
						continue
					}
					if !IsAssetLinkDest([]byte(item.Content)) {
						out = append(out, strings.TrimSpace(item.Content))
					}
				}
			}
		}
	}
	return out, nil
}

func SetRemoteDest(node *ast.Node, old, dest string, read func(string) (*av.AttributeView, error), save func(*av.AttributeView) error) error {
	if node == nil || read == nil || save == nil {
		return errors.New("asset_remote_unbound")
	}
	switch {
	case node.Type == ast.NodeLinkDest:
		if bytes.HasPrefix(node.Tokens, []byte("//")) {
			node.Tokens = append([]byte("https:"), node.Tokens...)
		}
		node.Tokens = bytes.ReplaceAll(node.Tokens, []byte(old), []byte(dest))
	case node.IsTextMarkType("a"):
		if strings.HasPrefix(node.TextMarkAHref, "//") {
			node.TextMarkAHref = "https:" + node.TextMarkAHref
		}
		node.TextMarkAHref = strings.ReplaceAll(node.TextMarkAHref, old, dest)
	case node.Type == ast.NodeAudio || node.Type == ast.NodeVideo:
		node.Tokens = bytes.ReplaceAll(node.Tokens, []byte(old), []byte(dest))
	case node.Type == ast.NodeAttributeView:
		a, e := read(node.AttributeViewID)
		if e != nil {
			return e
		}
		if a == nil {
			return nil
		}
		changed := false
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil {
				return errors.New("asset_remote_invalid_av")
			}
			if kv.Key.Type != av.KeyTypeMAsset {
				continue
			}
			for _, v := range kv.Values {
				if v == nil {
					return errors.New("asset_remote_invalid_av")
				}
				for _, item := range v.MAsset {
					if item == nil {
						return errors.New("asset_remote_invalid_av")
					}
					if item.Content == old && old != dest {
						item.Content = dest
						changed = true
					}
				}
			}
		}
		if changed {
			return save(a)
		}
	}
	return nil
}

// ConvertRemote preserves selected-link order and the original per-download
// skip policy. Source/write/AV errors and cancellation abort explicitly; already
// written assets remain a successful prefix, never a multi-file transaction.
func ConvertRemote(ctx context.Context, root *ast.Node, onlyImg bool, referer string, p RemotePorts) (out RemoteResult, err error) {
	if ctx == nil || root == nil || p.ReadLocal == nil || p.Fetch == nil || p.Write == nil || p.ReadAV == nil || p.SaveAV == nil || p.CommonExt == nil || p.NewID == nil {
		return out, errors.New("asset_remote_unbound")
	}
	type target struct {
		node  *ast.Node
		dests []string
	}
	targets := []target{}
	ast.Walk(root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		var dests []string
		dests, err = RemoteDests(n, onlyImg, p.ReadAV)
		if err != nil {
			return ast.WalkStop
		}
		if len(dests) > 0 {
			targets = append(targets, target{n, dests})
		}
		return ast.WalkContinue
	})
	if err != nil {
		return out, err
	}
	for _, target := range targets {
		for _, dest := range target.dests {
			if e := ctx.Err(); e != nil {
				return out, e
			}
			var raw []byte
			var name string
			lower := strings.ToLower(dest)
			if strings.HasPrefix(lower, "file://") {
				selected := dest[7:]
				if decoded, e := url.PathUnescape(selected); e == nil {
					selected = decoded
				}
				if strings.Contains(selected, ":") {
					selected = strings.TrimPrefix(selected, "/")
				}
				selected = strings.SplitN(selected, "?", 2)[0]
				data, found, e := p.ReadLocal(ctx, selected)
				if e != nil {
					return out, e
				}
				if !found {
					continue
				}
				raw = data
				name = Name("network-asset-"+textprefs.FilterUploadFileName(path.Base(strings.ReplaceAll(selected, "\\", "/"))), p.NewID())
			} else if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(dest, "//") {
				if strings.HasPrefix(dest, "//") {
					dest = "https:" + dest
				}
				u := dest
				if strings.Contains(u, "qpic.cn") {
					u = strings.Replace(u, "http://", "https://", 1)
				}
				if p.Progress != nil {
					p.Progress(u)
				}
				response, e := p.Fetch(ctx, u, referer)
				if e != nil {
					if ctx.Err() != nil {
						return out, ctx.Err()
					}
					out.Failed++
					continue
				}
				if response.Status == 401 || response.Status == 403 {
					out.Forbidden++
				}
				if strings.Contains(strings.ToLower(response.ContentType), "text/html") || response.Status != 200 || len(response.Data) > 96<<20 {
					out.Failed++
					continue
				}
				raw = response.Data
				name = path.Base(strings.SplitN(u, "?", 2)[0])
				name = strings.SplitN(name, "#", 2)[0]
				if decoded, e := url.PathUnescape(name); e == nil {
					name = decoded
				}
				name = textprefs.FilterUploadFileName(name)
				ext := path.Ext(name)
				if ext == "." {
					ext = ""
				}
				if !p.CommonExt(ext) {
					if detected := mimetype.Detect(raw); detected != nil {
						ext = detected.Extension()
						name += ext
					}
				}
				if ext == "" && bytes.HasPrefix(raw, []byte("<svg ")) && bytes.HasSuffix(raw, []byte("</svg>")) {
					ext = ".svg"
					name += ext
				}
				if ext == "" {
					extensions, _ := mime.ExtensionsByType(response.ContentType)
					if len(extensions) > 0 {
						name += extensions[0]
					}
				}
				name = "network-asset-" + Name(name, p.NewID())
			} else {
				continue
			}
			if e := p.Write(ctx, name, raw); e != nil {
				return out, e
			}
			if e := SetRemoteDest(target.node, dest, "assets/"+name, p.ReadAV, p.SaveAV); e != nil {
				return out, e
			}
			out.Files++
		}
	}
	return out, ctx.Err()
}
