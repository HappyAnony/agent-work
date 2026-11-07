package conf

import "testing"

func TestFullPresentationOwnsMaskAndRoleProjection(t *testing.T) {
	data := AppData{User: &User{UserSiYuanProExpireTime: -1}, UserData: "secret", AccessAuthCode: "secret", Editor: NewEditor(), Sync: NewSync(), System: &System{DataDir: "private", HomeDir: "private"}}
	data.Sync.Enabled = true
	data.Sync.Stat = "running"
	out, e := data.Presentation(PresentationOptions{Publish: true, DisabledSyncText: "disabled"})
	if e != nil {
		t.Fatal(e)
	}
	if out.User != nil || out.UserData != "" || out.AccessAuthCode != "*******" || !out.ReadOnly || out.Sync.Stat != "running" {
		t.Fatal("full projection changed")
	}
	out.Editor.FontSize = 99
	out.System.DataDir = "outside"
	if data.Editor.FontSize == 99 || data.System.DataDir != "private" || data.ReadOnly {
		t.Fatal("borrowed snapshot")
	}
	data.User = nil
	out, e = data.Presentation(PresentationOptions{Restricted: true, DisabledSyncText: "disabled"})
	if e != nil || out.System.DataDir != "" || out.System.HomeDir != "" || out.AI == nil || out.Api == nil || out.Sync == nil {
		t.Fatal(out, e)
	}
	data.System = nil
	out, e = data.Presentation(PresentationOptions{Restricted: true})
	if e != nil || out.System == nil {
		t.Fatal("nil system", e)
	}
	if data.System != nil {
		t.Fatal("projection mutated original")
	}
}
