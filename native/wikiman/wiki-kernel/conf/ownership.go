package conf

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"wiki-kernel/internal/nativeparser"
)

type Scope string

const (
	DevicePreference    Scope = "device"
	StorePreference     Scope = "store"
	WorkspacePreference Scope = "workspace"
	RuntimeValue        Scope = "runtime"
)

// Ownership classifies the complete original contract without moving existing
// persisted files. More-specific entries override a parent. Runtime paths and
// readonly/auth authority must always come from the owner, not imported JSON.
var ownership = map[string]Scope{
	"logLevel": DevicePreference, "appearance": DevicePreference,
	"langs": DevicePreference, "lang": DevicePreference,
	"fileTree": WorkspacePreference, "tag": WorkspacePreference,
	"editor": WorkspacePreference, "export": WorkspacePreference,
	"graph": WorkspacePreference, "uiLayout": DevicePreference,
	"userData": DevicePreference, "account": DevicePreference,
	"readonly": RuntimeValue, "localIPs": RuntimeValue,
	"accessAuthCode": DevicePreference, "system": DevicePreference,
	"keymap": DevicePreference, "sync": StorePreference,
	"search": WorkspacePreference, "flashcard": WorkspacePreference,
	"ai": DevicePreference, "bazaar": DevicePreference,
	"stat": StorePreference, "api": DevicePreference,
	"repo": StorePreference, "publish": DevicePreference,
	"openHelp": DevicePreference, "showChangelog": DevicePreference,
	"cloudRegion": DevicePreference, "snippet": WorkspacePreference,
	"dataIndexState": RuntimeValue,
	"system.id":      RuntimeValue, "system.name": RuntimeValue,
	"system.kernelVersion": RuntimeValue, "system.os": RuntimeValue,
	"system.osPlatform": RuntimeValue, "system.container": RuntimeValue,
	"system.isMicrosoftStore": RuntimeValue, "system.isInsider": RuntimeValue,
	"system.homeDir": RuntimeValue, "system.workspaceDir": RuntimeValue,
	"system.appDir": RuntimeValue, "system.confDir": RuntimeValue,
	"system.dataDir": RuntimeValue, "system.disabledFeatures": RuntimeValue,
	"fileTree.alwaysSelectOpenedFile": DevicePreference,
	"fileTree.openFilesUseCurrentTab": DevicePreference,
	"fileTree.maxOpenTabCount":        DevicePreference,
	"fileTree.closeTabsOnStart":       DevicePreference,
	"editor.fontSize":                 DevicePreference, "editor.fontFamily": DevicePreference,
	"editor.fontSizeScrollZoom": DevicePreference,
	"editor.fullWidth":          DevicePreference, "editor.rtl": DevicePreference,
	"editor.readOnly": RuntimeValue,
	"repo.key":        DevicePreference,
	"sync.s3":         DevicePreference, "sync.webdav": DevicePreference, "sync.local": DevicePreference,
	"sync.synced": RuntimeValue, "sync.stat": RuntimeValue,
	"export.pandocBin":       RuntimeValue,
	"export.docxTemplate":    DevicePreference,
	"export.pdfWatermarkStr": DevicePreference, "export.imageWatermarkStr": DevicePreference,
	"editor.plantUMLServePath": DevicePreference,
}

// FieldScopes returns a detached inventory for diagnostics and migration tools.
func FieldScopes() map[string]Scope {
	ret := make(map[string]Scope, len(ownership))
	for k, v := range ownership {
		ret[k] = v
	}
	return ret
}

func ScopeFor(field string) (Scope, error) {
	// Reject unknown fields rather than silently assigning new configuration to
	// an arbitrary owner. Dynamic dictionary contents inherit their field owner.
	typ := reflect.TypeOf(AppData{})
	for _, name := range strings.Split(field, ".") {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return "", errors.New("unknown_configuration_field")
		}
		var found reflect.Type
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if strings.Split(f.Tag.Get("json"), ",")[0] == name {
				found = f.Type
				break
			}
		}
		if found == nil {
			return "", errors.New("unknown_configuration_field")
		}
		typ = found
	}
	for {
		if scope, ok := ownership[field]; ok {
			return scope, nil
		}
		at := strings.LastIndexByte(field, '.')
		if at < 0 {
			return "", errors.New("unclassified_configuration_field")
		}
		field = field[:at]
	}
}

// Clone preserves the original complete serialized shape and detaches all maps,
// slices and pointers, including the non-persisted user view. It creates no
// defaults, API token, environment override or filesystem content.
func (data AppData) Clone() (AppData, error) {
	var ret AppData
	raw, err := json.Marshal(data)
	if err != nil {
		return ret, err
	}
	if err = json.Unmarshal(raw, &ret); err != nil {
		return ret, err
	}
	if data.User != nil {
		raw, err = json.Marshal(data.User)
		if err != nil {
			return AppData{}, err
		}
		if err = json.Unmarshal(raw, &ret.User); err != nil {
			return AppData{}, err
		}
	}
	return ret, nil
}

func (data AppData) ParserOptions(emojis map[string]string) (nativeparser.EditorOptions, error) {
	if data.Editor == nil || data.Editor.Markdown == nil || data.Export == nil {
		return nativeparser.EditorOptions{}, errors.New("configuration_parser_settings_missing")
	}
	copyEmojis := make(map[string]string, len(emojis))
	for k, v := range emojis {
		copyEmojis[k] = v
	}
	return nativeparser.EditorOptions{Markdown: *data.Editor.Markdown,
		CodeSyntaxHighlightLineNum: data.Editor.CodeSyntaxHighlightLineNum,
		ParagraphBeginningSpace:    data.Export.ParagraphBeginningSpace,
		DisplayNetImgMark:          data.Editor.DisplayNetImgMark,
		Spellcheck:                 data.Editor.Spellcheck, Emojis: copyEmojis}, nil
}
