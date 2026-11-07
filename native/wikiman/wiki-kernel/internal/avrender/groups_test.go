package avrender

import (
	"bytes"
	"errors"
	"github.com/88250/lute/ast"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/nativeparser"
)

func TestOriginalGroupMethodsAndStatePreservation(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	for _, method := range []av.GroupMethod{av.GroupMethodValue, av.GroupMethodRangeNum, av.GroupMethodDateDay, av.GroupMethodDateWeek, av.GroupMethodDateMonth, av.GroupMethodDateYear, av.GroupMethodDateRelative} {
		typ := av.KeyTypeDate
		if method == av.GroupMethodRangeNum {
			typ = av.KeyTypeNumber
		}
		if method == av.GroupMethodValue {
			typ = av.KeyTypeSelect
		}
		key := &av.Key{ID: "group", Type: typ, Options: []*av.SelectOption{{Name: "X"}, {Name: "Y"}, {Name: "Empty"}}}
		a := &av.AttributeView{KeyValues: []*av.KeyValues{{Key: key}}}
		table := &av.Table{BaseInstance: &av.BaseInstance{}, Columns: []*av.TableColumn{{BaseInstanceField: &av.BaseInstanceField{ID: key.ID}}}}
		for i := 0; i < 3; i++ {
			value := &av.Value{ID: "v", KeyID: key.ID, Type: typ}
			switch typ {
			case av.KeyTypeSelect:
				opt := "X"
				if i == 1 {
					opt = "Y"
				}
				value.MSelect = []*av.ValueSelect{{Content: opt}}
			case av.KeyTypeNumber:
				value.Number = &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}
			case av.KeyTypeDate:
				value.Date = &av.ValueDate{Content: now.AddDate(0, 0, i-1).UnixMilli(), IsNotEmpty: true}
			}
			table.Rows = append(table.Rows, &av.TableRow{ID: string(rune('a' + i)), Cells: []*av.TableCell{{BaseValue: &av.BaseValue{Value: value}}}})
		}
		view := &av.View{LayoutType: av.LayoutTypeGallery, Group: &av.ViewGroup{Field: key.ID, Method: method, Range: &av.GroupRange{NumStart: 0, NumEnd: 10, NumStep: 2}, Order: av.GroupOrderAsc}}
		env := GroupEnvironment{Now: func() time.Time { return now }, NewView: NewGroupView, Render: func(*av.AttributeView, *av.View, string) (av.Viewable, error) { return table, nil }}
		if e := GenerateGroups(view, a, env); e != nil {
			t.Fatal(method, e)
		}
		items := 0
		for _, g := range view.Groups {
			items += len(g.GroupItemIDs)
		}
		if items != 3 {
			t.Fatal("lost/duplicated group membership", method, view.Groups)
		}
		SortGroups(a, view, now)
		if method == av.GroupMethodValue {
			if len(view.Groups) != 4 {
				t.Fatal(view.Groups)
			}
			x := view.GetGroupByGroupValue("X")
			x.ID = "stable-id"
			x.GroupFolded = true
			x.GroupHidden = 2
			x.GroupSort = 0
			x.GroupItemIDs = []string{"c", "a"}
			if e := GenerateGroups(view, a, env); e != nil {
				t.Fatal(e)
			}
			x = view.GetGroupByGroupValue("X")
			if x.ID != "stable-id" || !x.GroupFolded || x.GroupHidden != 2 || x.GroupItemIDs[0] != "c" {
				t.Fatal("state not inherited", x)
			}
		}
	}
}

func TestOriginalRangeRejectsNonProgressAndUnbound(t *testing.T) {
	view := &av.View{Group: &av.ViewGroup{Method: av.GroupMethodRangeNum, Range: &av.GroupRange{NumStart: 1e20, NumEnd: 1e20, NumStep: 1}}}
	env := GroupEnvironment{Now: time.Now, NewView: NewGroupView, Render: func(*av.AttributeView, *av.View, string) (av.Viewable, error) {
		t.Fatal("invalid range reached rendering")
		return nil, nil
	}}
	if e := GenerateGroups(view, &av.AttributeView{}, env); !errors.Is(e, ErrShape) {
		t.Fatal(e)
	}
	if e := GenerateGroups(view, &av.AttributeView{}, GroupEnvironment{}); !errors.Is(e, ErrEnvironmentUnbound) {
		t.Fatal(e)
	}
}

func TestOriginalCoverRestoresTokensIDsIALAndEmptyDocument(t *testing.T) {
	parser := nativeparser.NewIndex(nativeparser.Defaults())
	node := &ast.Node{ID: "20261023090000-frm0001", Type: ast.NodeIFrame, Tokens: []byte(`<iframe src="https://example.invalid/embed"></iframe>`), KramdownIAL: [][]string{{"id", "20261023090000-frm0001"}, {av.NodeAttrNameAvs, "database"}}}
	before := bytes.Clone(node.Tokens)
	id := node.ID
	for i := 0; i < 2; i++ {
		out := renderCoverContentBlock(node, CoverServices{Parser: parser})
		if out == "" {
			t.Fatal("empty original cover")
		}
		if !bytes.Equal(before, node.Tokens) || node.ID != id || node.IALAttr(av.NodeAttrNameAvs) != "database" {
			t.Fatal("cover mutated input AST", node)
		}
	}
	if out := renderCoverContentBlock(&ast.Node{Type: ast.NodeDocument}, CoverServices{Parser: parser}); out != "" {
		t.Fatal(out)
	}
}
