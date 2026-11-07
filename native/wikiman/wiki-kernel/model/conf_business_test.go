package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestAppConfBusinessUsesOwnerAndOriginalProjection(t *testing.T) {
	items := []conf.Notebook{{ID: "same-id", Name: "A", Icon: "icon", Sort: 9, FlashcardCount: 7}, {ID: "closed", Name: "closed", Closed: true}}
	langs := map[string]map[int]string{"en_US": {1: "fallback ${accountServer}"}, "zh_CN": {2: "中文"}}
	servers := map[int]string{0: "a.invalid", 1: "b.invalid"}
	a := NewAppConfWithBusiness(conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return items, nil }, langs, servers))
	a.Lang = "zh_CN"
	b := NewAppConfWithBusiness(conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
		return []conf.Notebook{{ID: "same-id", Name: "B"}}, nil
	}, nil, nil))
	old := Conf
	Conf = b
	defer func() { Conf = old }()
	langs["en_US"][1] = "changed"
	servers[0] = "changed"
	if a.Box("same-id").Name != "A" || a.GetBox("same-id").Icon != "" || a.GetBox("same-id").Sort != 0 {
		t.Fatal("wrong owner or GetBoxes projection")
	}
	if a.GetOpenedBoxes()[0].FlashcardCount != 7 || len(a.GetClosedBoxes()) != 1 || a.Box("closed") != nil {
		t.Fatal("lost original filtering/full fields")
	}
	a.GetOpenedBoxes()[0].Name = "mutated"
	if a.BoxNames([]string{"same-id", "closed"})["same-id"] != "A" || len(a.BoxNames([]string{"closed"})) != 0 {
		t.Fatal("mutable provider or closed box leaked")
	}
	if a.Language(1) != "fallback a.invalid" || a.language(1) != "fallback ${accountServer}" || a.Language(2) != "中文" {
		t.Fatal("language fallback/expansion changed")
	}
	a.CloudRegion = 1
	if a.Language(1) != "fallback b.invalid" {
		t.Fatal("changed device region ignored")
	}
	a.AccessAuthCode = "synthetic secret"
	masked, err := a.Masked()
	if err != nil || masked.AccessAuthCode != MaskedAccessAuthCode || a.AccessAuthCode != "synthetic secret" {
		t.Fatal(masked, err)
	}
	if _, err := masked.notebooks(context.Background()); !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal("JSON projection acquired authority", err)
	}
	if _, err := NewAppConf().notebooks(context.Background()); !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal(err)
	}
	failure := errors.New("synthetic read failure")
	c := NewAppConfWithBusiness(conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return nil, failure }, nil, nil))
	if _, err := c.notebooks(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
