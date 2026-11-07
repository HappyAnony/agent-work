package conf

import (
	"encoding/json"
	"reflect"
	"testing"
)

func settingFixture() AppData {
	return AppData{Editor: NewEditor(), FileTree: NewFileTree(), Search: NewSearch(), Export: NewExport(), AI: NewAI(), Account: NewAccount(), Flashcard: NewFlashcard()}
}

func TestOriginalSettingNormalizationAndEffects(t *testing.T) {
	before := settingFixture()
	before.Editor.VirtualBlockRef = false
	before.Search.VirtualRefName = false
	tests := []struct {
		kind, raw string
		effects   []string
		check     func(AppData) bool
	}{
		{"editor", `{"historyRetentionDays":0,"generateHistoryInterval":21,"virtualBlockRef":true,"readonly":true,"readOnly":true}`, []string{"history", "virtual", "readonly", "markdown"}, func(d AppData) bool {
			return d.Editor.HistoryRetentionDays == 30 && d.Editor.GenerateHistoryInterval == 21 && d.Editor.KaTexMacros == "{}" && d.Editor.PlantUMLServePath != ""
		}},
		{"fileTree", `{"maxOpenTabCount":100,"refCreateSavePath":" / A / B ","docCreateSavePath":" / C "}`, []string{"fileTree"}, func(d AppData) bool {
			return d.FileTree.MaxOpenTabCount == 32 && d.FileTree.RefCreateSavePath == "/A/B/" && d.FileTree.DocCreateSavePath == "/C"
		}},
		{"search", `{"limit":1,"caseSensitive":true,"virtualRefName":true}`, []string{"search", "reindex", "virtual"}, func(d AppData) bool { return d.Search.Limit == 32 && d.Search.CaseSensitive }},
		{"ai", `{"openAI":{"apiTimeout":900,"apiMaxTokens":-1,"apiTemperature":3,"apiMaxContexts":90}}`, nil, func(d AppData) bool {
			return d.AI.OpenAI.APITimeout == 600 && d.AI.OpenAI.APIMaxTokens == 0 && d.AI.OpenAI.APITemperature == 1 && d.AI.OpenAI.APIMaxContexts == 7
		}},
		{"flashcard", `{"newCardLimit":-1,"reviewCardLimit":-1}`, nil, func(d AppData) bool { return d.Flashcard.NewCardLimit == 20 && d.Flashcard.ReviewCardLimit == 200 }},
		{"account", `{"displayVIP":true}`, nil, func(d AppData) bool { return d.Account.DisplayVIP && !d.Account.DisplayTitle }},
		{"snippet", `{"enabledCSS":true}`, []string{"snippet"}, func(d AppData) bool { return d.Snippet.EnabledCSS && !d.Snippet.EnabledJS }},
		{"bazaar", `{}`, nil, func(d AppData) bool { return d.Bazaar != nil }},
		{"keymap", `{}`, nil, func(d AppData) bool { return d.Keymap != nil }},
		{"editorReadOnly", `{"readonly":true}`, []string{"readonly"}, func(d AppData) bool { return d.Editor.ReadOnly }},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			plan, e := before.PrepareSetting(tt.kind, []byte(tt.raw))
			if e != nil {
				t.Fatal(e)
			}
			if !tt.check(plan.Data) {
				t.Fatal("normalization", plan.Data)
			}
			var kinds []string
			for _, effect := range plan.Effects {
				kinds = append(kinds, effect.Kind)
			}
			if !reflect.DeepEqual(kinds, tt.effects) {
				t.Fatal(kinds)
			}
			if tt.kind == "editorReadOnly" || tt.kind == "keymap" {
				if len(plan.Value) != 0 {
					t.Fatal("unexpected response")
				}
			} else if !json.Valid(plan.Value) {
				t.Fatal("invalid original response")
			}
			plan.Data.Editor.FontSize = 99
			if before.Editor.FontSize == 99 {
				t.Fatal("borrowed input")
			}
		})
	}
	low, e := before.PrepareSetting("ai", []byte(`{"openAI":{"apiTimeout":1,"apiTemperature":0,"apiMaxContexts":0}}`))
	if e != nil || low.Data.AI.OpenAI.APITimeout != 5 {
		t.Fatal(e)
	}
	high, e := before.PrepareSetting("editor", []byte(`{"historyRetentionDays":5000}`))
	if e != nil || high.Data.Editor.HistoryRetentionDays != 3650 {
		t.Fatal(e)
	}
}

func TestSettingRejectsMalformedAndClassifiesOnlyPreferenceChanges(t *testing.T) {
	before := settingFixture()
	for _, tt := range []struct{ kind, raw string }{{"editor", "null"}, {"editor", `{"markdown":null}`}, {"ai", "{}"}, {"ai", `{"openAI":null}`}, {"search", "[]"}, {"editorReadOnly", "{}"}, {"editorReadOnly", `{"readonly":"yes"}`}, {"unknown", "{}"}} {
		if _, e := before.PrepareSetting(tt.kind, []byte(tt.raw)); e == nil {
			t.Fatal(tt)
		}
	}
	plan, e := before.PrepareSetting("editor", []byte(`{"fontSize":29,"readOnly":true,"plantUMLServePath":"device-service","markdown":{"inlineMath":false}}`))
	if e != nil {
		t.Fatal(e)
	}
	patches, e := plan.PreferencePatches(before)
	if e != nil {
		t.Fatal(e)
	}
	if string(patches[DevicePreference].Set["editor.fontSize"]) != "29" || string(patches[DevicePreference].Set["editor.plantUMLServePath"]) != `"device-service"` || string(patches[WorkspacePreference].Set["editor.markdown.inlineMath"]) != "false" {
		t.Fatal(patches)
	}
	for _, p := range patches {
		if _, ok := p.Set["editor.readOnly"]; ok {
			t.Fatal("runtime authority persisted")
		}
		for key := range p.Set {
			if _, e := ScopeFor(key); e != nil {
				t.Fatal(key, e)
			}
		}
	}
	readonly, e := before.PrepareSetting("editorReadOnly", []byte(`{"readonly":true}`))
	if e != nil {
		t.Fatal(e)
	}
	p, e := readonly.PreferencePatches(before)
	if e != nil || len(p) != 0 {
		t.Fatal(p, e)
	}
}
