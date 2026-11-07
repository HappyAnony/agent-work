package model

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestAppConfClassifiedImportPreservesAuthority(t *testing.T) {
	c := NewAppConfWithBusiness(conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return []conf.Notebook{{ID: "A"}}, nil }, nil, nil))
	c.Editor = conf.NewEditor()
	c.ReadOnly = true
	doc := conf.PreferenceDocument{Version: 1, Scope: conf.WorkspacePreference, Values: map[string]json.RawMessage{"editor.markdown.inlineMath": json.RawMessage(`false`)}}
	if err := c.ImportPreferences(doc); err != nil {
		t.Fatal(err)
	}
	if c.Editor.Markdown.InlineMath || !c.ReadOnly || c.Box("A") == nil {
		t.Fatal("private binding/runtime authority changed")
	}
	if err := c.SaveError(); !errors.Is(err, ErrUnboundAppConf) {
		t.Fatal(err)
	}
	if _, err := c.Preferences(conf.WorkspacePreference); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalTransactionCapturesEditorOptionsAtAdmission(t *testing.T) {
	previous := Conf
	defer func() { Conf = previous }()
	Conf = NewAppConfWithBusiness(nil)
	Conf.Editor = conf.NewEditor()
	Conf.Export = conf.NewExport()
	Conf.Editor.Markdown.InlineMath = false
	environment := legacyTransactionEnvironment()
	Conf.Editor.Markdown.InlineMath = true
	if environment.editorParser().ParseOptions.InlineMath {
		t.Fatal("queued transaction read later global editor settings")
	}
	if !NewLute().ParseOptions.InlineMath {
		t.Fatal("original parser no longer uses current configuration")
	}
}
