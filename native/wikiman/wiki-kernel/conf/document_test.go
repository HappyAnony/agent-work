package conf

import (
	"encoding/json"
	"testing"
)

func TestClassifiedPreferencesRoundTripAndSecrets(t *testing.T) {
	base := AppData{Editor: NewEditor(), Export: NewExport(), Graph: NewGraph(), Repo: NewRepo(), Sync: NewSync(), System: &System{DataDir: "runtime-only"}, User: &User{}}
	base.Repo.Key = []byte("synthetic")
	base.Repo.SyncIndexTiming = 9007199254740993
	base.Sync.S3 = &S3{SecretKey: "synthetic"}
	base.Editor.FontSize = 23
	result, err := base.Clone()
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []Scope{DevicePreference, StorePreference, WorkspacePreference} {
		d, err := base.Preferences(scope)
		if err != nil {
			t.Fatal(scope, err)
		}
		for _, key := range []string{"system.dataDir", "readonly", "sync.synced"} {
			if _, ok := d.Values[key]; ok {
				t.Fatal("runtime field persisted", key)
			}
		}
		if scope != DevicePreference {
			for _, key := range []string{"repo.key", "sync.s3.secretKey", "editor.fontSize"} {
				if _, ok := d.Values[key]; ok {
					t.Fatal("device field leaked", key)
				}
			}
		}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		d, err = DecodePreferences(raw, scope)
		if err != nil {
			t.Fatal(err)
		}
		result, err = result.ApplyPreferences(d)
		if err != nil {
			t.Fatal(err)
		}
	}
	if result.Repo.SyncIndexTiming != 9007199254740993 || result.System.DataDir != "runtime-only" || string(result.Repo.Key) != "synthetic" || result.Editor.FontSize != 23 {
		t.Fatal("projection changed")
	}
	result.Editor.FontSize = 99
	if base.Editor.FontSize == 99 {
		t.Fatal("shared mutable settings")
	}
}

func TestClassifiedPreferencesRejectInvalidAndWrongOwner(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"scope":"workspace","values":{"editor.fontSize":24}}`,
		`{"version":1,"scope":"workspace","values":{"system.dataDir":"/bad"}}`,
		`{"version":1,"scope":"workspace","values":{"repo.key":"c2VjcmV0"}}`,
		`{"version":1,"scope":"workspace","values":{"export.pandocBin":"/arbitrary-executable"}}`,
		`{"version":1,"scope":"workspace","values":{"editor.markdown.inlineMath":null}}`,
		`{"version":1,"scope":"workspace","values":{"editor.markdown.inlineMath":"yes"}}`,
		`{"version":1,"scope":"workspace","values":{"unknown":1}}`,
		`{"version":1,"scope":"workspace","scope":"device","values":{}}`,
		`{"version":1,"scope":"workspace","values":{},"extra":true}`,
		`{"version":1,"scope":"workspace","values":null}`,
		`{"version":1,"scope":"workspace","values":{}} {}`,
	} {
		if _, err := DecodePreferences([]byte(raw), WorkspacePreference); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
