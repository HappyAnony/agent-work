package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestOriginalAppConfPresentationUsesReceiver(t *testing.T) {
	a := NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {53: "owner disabled"}}, nil))
	a.Lang = "en_US"
	a.Sync = conf.NewSync()
	a.Editor = conf.NewEditor()
	a.AccessAuthCode = "private"
	a.UserData = "private"
	a.System = &conf.System{DataDir: "owner data"}
	other := NewAppConfWithBusiness(conf.NewBusiness(nil, nil, nil))
	old := Conf
	Conf = other
	defer func() { Conf = old }()
	shown, e := a.Presentation(false, false)
	if e != nil || shown.Sync.Stat != "owner disabled" || shown.AccessAuthCode != "*******" || shown.UserData != "" {
		t.Fatal(shown, e)
	}
	shown.Editor.FontSize = 99
	if a.Editor.FontSize == 99 {
		t.Fatal("presentation borrowed settings")
	}
	shown, e = a.Presentation(true, true)
	if e != nil || !shown.ReadOnly || shown.System.DataDir != "" || a.System.DataDir != "owner data" {
		t.Fatal(e)
	}
	if _, e = shown.notebooks(context.Background()); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("presentation gained authority", e)
	}
	if _, e = NewAppConf().Presentation(false, false); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal(e)
	}
}
