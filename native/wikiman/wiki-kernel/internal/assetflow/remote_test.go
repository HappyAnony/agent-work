package assetflow

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"strings"
	"testing"
	av "wiki-kernel/internal/avmodel"
)

func TestRemoteConversionASTAndAVOriginalSelection(t *testing.T) {
	root := &ast.Node{Type: ast.NodeDocument}
	image := &ast.Node{Type: ast.NodeImage}
	dest := &ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("//example.invalid/image.png")}
	image.AppendChild(dest)
	root.AppendChild(image)
	link := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkAHref: "https://example.invalid/link.pdf"}
	root.AppendChild(link)
	node := &ast.Node{Type: ast.NodeAttributeView, AttributeViewID: "20261104100000-view001"}
	root.AppendChild(node)
	imageAsset := &av.ValueAsset{Type: av.AssetTypeImage, Content: "https://example.invalid/av.png"}
	otherAsset := &av.ValueAsset{Content: "https://example.invalid/other.pdf"}
	view := &av.AttributeView{ID: node.AttributeViewID, KeyValues: []*av.KeyValues{{Key: &av.Key{Type: av.KeyTypeMAsset}, Values: []*av.Value{{MAsset: []*av.ValueAsset{imageAsset, otherAsset}}}}}}
	writes, saves, fetches := 0, 0, []string{}
	ports := RemotePorts{NewID: func() string { return "20261104100000-asset01" }, CommonExt: func(string) bool { return true }, ReadLocal: func(context.Context, string) ([]byte, bool, error) { return nil, false, nil }, ReadAV: func(string) (*av.AttributeView, error) { return view, nil }, SaveAV: func(*av.AttributeView) error { saves++; return nil }, Write: func(context.Context, string, []byte) error { writes++; return nil }, Fetch: func(_ context.Context, address, referer string) (RemoteResponse, error) {
		fetches = append(fetches, address)
		if referer != "selected referer" {
			t.Fatal(referer)
		}
		return RemoteResponse{Data: []byte("image"), ContentType: "image/png", Status: 200}, nil
	}}
	result, e := ConvertRemote(context.Background(), root, true, "selected referer", ports)
	if e != nil || result.Files != 2 || writes != 2 || saves != 1 || fetches[0] != "https://example.invalid/image.png" {
		t.Fatal(result, writes, saves, fetches, e)
	}
	if !strings.HasPrefix(string(dest.Tokens), "assets/network-asset-") || !strings.HasPrefix(imageAsset.Content, "assets/network-asset-") || otherAsset.Content != "https://example.invalid/other.pdf" || link.TextMarkAHref != "https://example.invalid/link.pdf" {
		t.Fatal("selection/replacement changed")
	}
	view.KeyValues = append(view.KeyValues, nil)
	writes = 0
	if _, e = ConvertRemote(context.Background(), root, false, "selected referer", ports); e == nil || writes != 0 {
		t.Fatal("malformed selected AV published partial input", writes, e)
	}
}

func TestRemoteConversionForbiddenDownloadSkipAndSourceFailure(t *testing.T) {
	root := &ast.Node{Type: ast.NodeDocument}
	node := &ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("https://example.invalid/image.png")}
	root.AppendChild(node)
	ports := RemotePorts{NewID: func() string { return "20261104100000-asset01" }, CommonExt: func(string) bool { return true }, ReadLocal: func(context.Context, string) ([]byte, bool, error) { return nil, false, nil }, ReadAV: func(string) (*av.AttributeView, error) { return nil, nil }, SaveAV: func(*av.AttributeView) error { return nil }, Write: func(context.Context, string, []byte) error { return errors.New("source failure") }, Fetch: func(context.Context, string, string) (RemoteResponse, error) { return RemoteResponse{Status: 403}, nil }}
	result, e := ConvertRemote(context.Background(), root, false, "", ports)
	if e != nil || result.Forbidden != 1 || result.Files != 0 {
		t.Fatal(result, e)
	}
	ports.Fetch = func(context.Context, string, string) (RemoteResponse, error) {
		return RemoteResponse{Status: 200, Data: []byte("image")}, nil
	}
	if result, e = ConvertRemote(context.Background(), root, false, "", ports); e == nil || result.Files != 0 || string(node.Tokens) != "https://example.invalid/image.png" {
		t.Fatal(result, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = ConvertRemote(ctx, root, false, "", ports); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
