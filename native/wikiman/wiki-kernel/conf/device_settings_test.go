package conf

import (
	"context"
	"errors"
	"testing"
)

func TestDeviceSettingPlansAreDetachedAndClassified(t *testing.T) {
	before := AppData{System: NewSystem(), Api: NewAPI(), Appearance: NewAppearance(), AccessAuthCode: "secret"}
	for _, tc := range []struct{ kind, raw string }{
		{"uiLayout", `{"layout":{"layout":{"children":[]}}}`},
		{"apiToken", `{"token":"new-token"}`},
		{"accessAuthCode", `{"accessAuthCode":"*******"}`},
		{"lockScreenMode", `{"lockScreenMode":1}`},
		{"appearanceMode", `{"mode":1}`},
		{"networkServe", `{"networkServe":true}`},
		{"autoLaunch", `{"autoLaunch":2}`},
		{"downloadInstallPkg", `{"downloadInstallPkg":false}`},
		{"networkProxy", `{"scheme":"http","host":"localhost","port":"8080"}`},
		{"defenderExcluded", `{}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			p, err := before.PrepareSettingWithServices(context.Background(), tc.kind, []byte(tc.raw), SettingServices{ThemeScriptExists: func(context.Context, string) (bool, error) { return true, nil }})
			if err != nil {
				t.Fatal(err)
			}
			patches, err := p.PreferencePatches(before)
			if err != nil {
				t.Fatal(err)
			}
			for scope := range patches {
				if scope != DevicePreference {
					t.Fatal(scope)
				}
			}
			if before.System.NetworkServe || before.System.MicrosoftDefenderExcluded || before.Api.Token == "new-token" {
				t.Fatal("changed input")
			}
			if tc.kind == "accessAuthCode" && p.Data.AccessAuthCode != "secret" {
				t.Fatal("mask replaced credential")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := before.PrepareSettingWithServices(ctx, "apiToken", []byte(`{"token":"bad"}`), SettingServices{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := before.PrepareSetting("apiToken", []byte(`{}`)); err == nil {
		t.Fatal("missing field admitted")
	}
}
