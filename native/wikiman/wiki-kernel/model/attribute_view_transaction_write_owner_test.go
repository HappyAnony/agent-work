//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"testing"
	"wiki-kernel/av"
)

func TestAVTransactionWriteFactoriesUseCapturedOwner(t *testing.T) {
	tx, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	for _, read := range []func(string) (*av.AttributeView, error){tx.attributeViewEnvironment().read, tx.attributeViewSettingsEnvironment().read, tx.attributeViewAppendEnvironment().read} {
		if got, e := read(a.ID); e != nil || got == nil || got.ID != a.ID {
			t.Fatalf("bound read failed: %v", e)
		}
	}
	old := Conf
	Conf = &AppConf{}
	defer func() { Conf = old }()
	for i, read := range []func(string) (*av.AttributeView, error){tx.attributeViewEnvironment().read, tx.attributeViewSettingsEnvironment().read, tx.attributeViewAppendEnvironment().read} {
		if _, e := read(a.ID); e == nil {
			t.Fatalf("factory %d followed global replacement", i)
		}
	}
}

func TestAVTransactionSettingsUsePendingBlockView(t *testing.T) {
	tx, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	other, _, _ := av.NewTableViewWithBlockKey("other")
	other.ID = ast.NewNodeID()
	a.Views = append(a.Views, other)
	var blockID string
	for _, tree := range tx.trees {
		if tree.Root.FirstChild != nil {
			blockID = tree.Root.FirstChild.ID
			tree.Root.FirstChild.SetIALAttr(av.NodeAttrView, other.ID)
			break
		}
	}
	if blockID == "" {
		t.Fatal("no candidate block")
	}
	settings := tx.attributeViewSettingsEnvironment()
	if view, e := settings.view(a, blockID); e != nil || view == nil || view.ID != other.ID {
		t.Fatalf("pending view ignored: %+v %v", view, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx.environment.ctx = ctx
	if _, e := tx.attributeViewSettingsEnvironment().view(a, blockID); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel lost: %v", e)
	}
}
