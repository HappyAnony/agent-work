//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	av "wiki-kernel/internal/avmodel"
)

func TestNativeOriginalGroupIndependentPagingAndCalc(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261023090300-av00001"
	a := layoutFixture(id, pageA, av.CoverFromNone)
	v := a.Views[1]
	v.Group = &av.ViewGroup{Field: "select", Method: av.GroupMethodValue, Order: av.GroupOrderMan}
	v.Gallery.CardFields[1].Calc = &av.FieldCalc{Operator: av.CalcOperatorSum}
	x, _ := avrenderGroupForTest("X", "stable-x", []string{"20261023090002-row0002", blockID})
	y, _ := avrenderGroupForTest("Y", "stable-y", []string{"20261023090001-row0001"})
	x.GroupFolded = true
	x.GroupHidden = 2
	x.GroupSort = 0
	y.GroupSort = 1
	v.Groups = []*av.View{x, y}
	writeLayoutFixture(t, root, pageA, a)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	raw, e := l.NativeAttributeViewRender(ctx, id, "gallery", "", 1, 10, map[string]NativeAVPaging{"stable-x": {Page: 2, PageSize: 1}})
	if e != nil {
		t.Fatal(e)
	}
	var output struct {
		Groups []struct {
			ID          string
			GroupFolded bool
			GroupHidden int
			CardCount   int
			Fields      []*av.GalleryField
			Cards       []*av.GalleryCard
		}
	}
	if e = json.Unmarshal(raw, &output); e != nil {
		t.Fatal(e)
	}
	if len(output.Groups) != 2 {
		t.Fatal(string(raw))
	}
	gx, gy := output.Groups[0], output.Groups[1]
	if gx.ID != "stable-x" || !gx.GroupFolded || gx.GroupHidden != 2 || gx.CardCount != 2 || len(gx.Cards) != 1 || gx.Cards[0].ID != blockID || gx.Fields[1].Calc.Result.Number.Content != 4 || gy.Fields[1].Calc.Result.Number.Content != 2 {
		t.Fatal("group state/paging/calc aliases", string(raw))
	}
	if after := digest(t, root); after != before {
		t.Fatal("group rendering saved source")
	}
}

func avrenderGroupForTest(value, id string, ids []string) (*av.View, error) {
	return &av.View{ID: id, LayoutType: av.LayoutTypeGallery, Gallery: av.NewLayoutGallery(), GroupVal: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: value}}, GroupItemIDs: ids}, nil
}

func TestNativeOriginalExtensionlessCoverAndSourceFailures(t *testing.T) {
	for _, failure := range []string{"", "asset-missing", "asset-link", "tree-missing"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			const id = "20261023090400-av00001"
			a := layoutFixture(id, pageA, av.CoverFromAssetField)
			for _, v := range a.KeyValues[3].Values {
				v.MAsset[0].Content = "assets/image"
			}
			if failure == "tree-missing" {
				a.Views[1].Gallery.CoverFrom = av.CoverFromContentBlock
			}
			writeLayoutFixture(t, root, pageA, a)
			assets := filepath.Join(root, "pages", pageA, "data", "assets")
			if e := os.MkdirAll(assets, 0700); e != nil {
				t.Fatal(e)
			}
			if failure == "asset-link" {
				if e := os.Symlink("/outside/not-authorized", filepath.Join(assets, "image")); e != nil {
					t.Fatal(e)
				}
			} else if failure != "asset-missing" {
				if e := os.WriteFile(filepath.Join(assets, "image"), []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			m := open(t, root, Options{Configuration: originalProjectionConfiguration})
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			if failure == "tree-missing" {
				if e = l.RebuildNativeBlockTree(ctx); e != nil {
					t.Fatal(e)
				}
				if e = os.Remove(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")); e != nil {
					t.Fatal(e)
				}
			}
			raw, e := l.NativeAttributeViewRender(ctx, id, "gallery", "", 1, 10, nil)
			if failure == "" {
				if e != nil || !strings.Contains(string(raw), `"coverURL":"assets/image"`) {
					t.Fatal(string(raw), e)
				}
			} else if e == nil || raw != nil {
				t.Fatal("dependency failure became empty cover", string(raw), e)
			}
		})
	}
}
