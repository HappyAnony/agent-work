// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/internal/workspace"
)

var uiCodeLanguage = regexp.MustCompile(`^[a-zA-Z0-9_+#.-]{1,64}$`)
var uiHistoryWidth = regexp.MustCompile(`^[1-9][0-9]{1,3}px$`)
var uiFontStyleValue = regexp.MustCompile(`^[A-Za-z0-9 ,.()%#-]{0,96}$`)

var uiStorageRoutes = map[string]uiRoute{
	"/api/system/setUILayout": uiRouteFor(func(q struct {
		Layout    json.RawMessage
		ErrorExit bool
	}) (uiCall, error) {
		if !validUILayout(q.Layout) || q.ErrorExit {
			return uiCall{}, ErrArgument
		}
		call := uiLocalWrite(pageprefs.Request{Action: "local.setValue", Key: "ui-layout", Value: q.Layout})
		call.pageWide = true
		return call, nil
	}),
	"/api/storage/getOutlineStorage": uiRouteFor(func(q struct{ DocID string }) (uiCall, error) {
		if !nativeID.MatchString(q.DocID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "storage", options: readStorage{Action: "outline.get", Key: q.DocID}, blocks: []string{q.DocID}}, nil
	}),
	"/api/storage/setOutlineStorage": uiRouteFor(func(q struct {
		DocID string
		Val   struct {
			ExpandIDs []string `json:"expandIds"`
		}
	}) (uiCall, error) {
		if !nativeID.MatchString(q.DocID) || len(q.Val.ExpandIDs) > 2000 ||
			slices.ContainsFunc(q.Val.ExpandIDs, func(id string) bool { return !nativeID.MatchString(id) }) {
			return uiCall{}, ErrArgument
		}
		raw, err := json.Marshal(q.Val)
		if err != nil {
			return uiCall{}, err
		}
		return uiLocalWrite(pageprefs.Request{Action: "outline.set", Key: q.DocID, Value: raw}, q.DocID), nil
	}),
	"/api/storage/getRecentDocs": uiRouteFor(func(q struct{ SortBy string }) (uiCall, error) {
		return uiCall{action: "storage", options: readStorage{Action: "recent.get", SortBy: q.SortBy}, pageWide: true}, nil
	}),
	"/api/storage/updateRecentDocOpenTime":  uiRecentTime("recent.open"),
	"/api/storage/updateRecentDocViewTime":  uiRecentTime("recent.view"),
	"/api/storage/updateRecentDocCloseTime": uiRecentTime("recent.close"),
}

func uiViewStorage(endpoint string, view UIView) uiRoute {
	mode := view.Mode
	get := uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			raw, err := l.NativeReadStorage(ctx, "local.get", "", "")
			if err != nil {
				return nil, err
			}
			var stored map[string]json.RawMessage
			if err := json.Unmarshal(raw, &stored); err != nil {
				return nil, err
			}
			out := map[string]any{}
			var code string
			if json.Unmarshal(stored["local-codelang"], &code) == nil && uiCodeLanguage.MatchString(code) {
				out["local-codelang"] = code
			}
			var outline uiOutlinePreference
			if Decode(bytes.NewReader(stored["local-outline"]), &outline) == nil {
				out["local-outline"] = outline
			}
			for _, key := range []string{"local-fileposition", "local-dialogposition"} {
				value := stored[mode+"."+key]
				if validUIPosition(key, value, view) {
					out[key] = value
				}
			}
			for _, key := range []string{"local-filespaths", "local-singleoutlinecollapsed", "local-history", "local-fontstyles", "local-desktop-tabs"} {
				if value := stored[mode+"."+key]; validUILayoutState(mode, key, value) {
					out[key] = value
				}
			}
			return out, nil
		}}, nil
	})
	set := uiRouteFor(func(q struct {
		App, Key, EntryID string
		Val               json.RawMessage
	}) (uiCall, error) {
		switch q.Key {
		case "local-filespaths", "local-singleoutlinecollapsed", "local-history", "local-fontstyles", "local-desktop-tabs":
			if q.EntryID != "" || !validUILayoutState(mode, q.Key, q.Val) {
				return uiCall{}, ErrArgument
			}
			return uiLocalWrite(pageprefs.Request{Action: "local.setValue", Key: mode + "." + q.Key, Value: q.Val}), nil
		case "local-fileposition", "local-dialogposition":
			action := "local.setValue"
			raw := q.Val
			var entries []string
			if q.EntryID != "" {
				if q.Key != "local-fileposition" {
					return uiCall{}, ErrArgument
				}
				raw, _ = json.Marshal(map[string]json.RawMessage{q.EntryID: q.Val})
				action, entries = "local.setEntry", []string{q.EntryID}
			}
			if !validUIPosition(q.Key, raw, view) {
				return uiCall{}, ErrArgument
			}
			return uiLocalWrite(pageprefs.Request{Action: action, Key: mode + "." + q.Key, Keys: entries, Value: q.Val}), nil
		case "local-codelang":
			var value string
			if json.Unmarshal(q.Val, &value) != nil || !uiCodeLanguage.MatchString(value) {
				return uiCall{}, ErrArgument
			}
		case "local-outline":
			var value *uiOutlinePreference
			if Decode(bytes.NewReader(q.Val), &value) != nil || value == nil {
				return uiCall{}, ErrArgument
			}
		default:
			return uiCall{}, ErrUIEndpoint
		}
		return uiLocalWrite(pageprefs.Request{Action: "local.setValue", Key: q.Key, Value: q.Val}), nil
	})
	if endpoint == "/api/storage/getLocalStorage" {
		return get
	}
	return set
}

func validUILayoutState(mode, key string, raw json.RawMessage) bool {
	if key == "local-desktop-tabs" {
		return mode == "wikiDesktop" && validUIDesktopTabs(raw)
	}
	if key == "local-fontstyles" {
		var styles []string
		if len(raw) > 4096 || json.Unmarshal(raw, &styles) != nil || styles == nil || len(styles) > 8 {
			return false
		}
		for _, style := range styles {
			parts := strings.Split(style, "\u200b")
			if len(parts) < 2 || len(parts) > 3 ||
				!slices.Contains([]string{"color", "backgroundColor", "style1", "style2", "style4", "fontSize", "clear"}, parts[0]) {
				return false
			}
			for _, part := range parts[1:] {
				if !uiFontStyleValue.MatchString(part) {
					return false
				}
			}
		}
		return true
	}
	if key == "local-history" {
		var pref *struct {
			NotebookID, Operation, SideWidth, SideDocWidth, SideDiffWidth string
			Type                                                          int
		}
		return len(raw) <= 1024 && Decode(bytes.NewReader(raw), &pref) == nil && pref != nil &&
			(pref.NotebookID == "%" || nativeID.MatchString(pref.NotebookID)) &&
			slices.Contains([]string{"all", "clean", "update", "delete", "format", "sync", "replace", "outline"}, pref.Operation) &&
			pref.Type >= 0 && pref.Type <= 1 && uiHistoryWidth.MatchString(pref.SideWidth) &&
			uiHistoryWidth.MatchString(pref.SideDocWidth) && uiHistoryWidth.MatchString(pref.SideDiffWidth)
	}
	if key == "local-singleoutlinecollapsed" {
		return mode == "docxSingle" && (string(raw) == "true" || string(raw) == "false")
	}
	var notebooks []struct {
		NotebookID string
		OpenPaths  []string
	}
	if mode != "wikiDesktop" || len(raw) > 64<<10 || Decode(bytes.NewReader(raw), &notebooks) != nil || notebooks == nil || len(notebooks) > 256 {
		return false
	}
	for _, notebook := range notebooks {
		if !nativeID.MatchString(notebook.NotebookID) || len(notebook.OpenPaths) > 1000 {
			return false
		}
		for _, path := range notebook.OpenPaths {
			if path != "/" && (len(path) < 2 || path[0] != '/' || !documentRelativePath(path[1:])) {
				return false
			}
		}
	}
	return true
}

type uiDesktopTabNode struct {
	Tabs []struct {
		BoxID, DocID string
		Pinned       bool
	}
	Active    *int
	Direction string
	Ratio     int
	Children  []*uiDesktopTabNode
}

func validUIDesktopTabs(raw json.RawMessage) bool {
	var value struct {
		Version int
		Root    *uiDesktopTabNode
		Focused int
	}
	if len(raw) > 16<<10 || Decode(bytes.NewReader(raw), &value) != nil || value.Version != 1 || value.Focused < 0 {
		return false
	}
	if value.Root == nil {
		return value.Focused == 0
	}
	tabs, panes := 0, 0
	var valid func(*uiDesktopTabNode, int) bool
	valid = func(node *uiDesktopTabNode, depth int) bool {
		if node == nil || depth > 32 {
			return false
		}
		if node.Direction == "" {
			tabs += len(node.Tabs)
			panes++
			if tabs > 32 || len(node.Tabs) == 0 || len(node.Children) != 0 || node.Ratio != 0 ||
				node.Active == nil || *node.Active < 0 || *node.Active >= len(node.Tabs) {
				return false
			}
			for _, tab := range node.Tabs {
				if !nativeID.MatchString(tab.BoxID) || !nativeID.MatchString(tab.DocID) {
					return false
				}
			}
			return true
		}
		return (node.Direction == "horizontal" || node.Direction == "vertical") && node.Ratio >= 15 && node.Ratio <= 85 &&
			len(node.Children) == 2 && len(node.Tabs) == 0 && node.Active == nil &&
			valid(node.Children[0], depth+1) && valid(node.Children[1], depth+1)
	}
	return valid(value.Root, 1) && value.Focused < panes
}

type uiFilePosition struct {
	RootID, StartID, EndID, FocusID, ZoomInID string
	ScrollTop, FocusStart, FocusEnd           float64
}

type uiDialogPosition struct {
	Width, Height, Left, Top float64
}

func validUIPosition(key string, raw json.RawMessage, view UIView) bool {
	var entries map[string]json.RawMessage
	if len(raw) > 128<<10 || Decode(bytes.NewReader(raw), &entries) != nil || entries == nil || len(entries) > 256 {
		return false
	}
	for id, value := range entries {
		if key == "local-fileposition" {
			var item *uiFilePosition
			if !nativeID.MatchString(id) || view.Mode == "docxSingle" && id != view.Entry.DocID ||
				Decode(bytes.NewReader(value), &item) != nil || item == nil || item.RootID != id ||
				item.ScrollTop < 0 || item.ScrollTop > 1e8 || item.FocusStart < 0 || item.FocusEnd < item.FocusStart || item.FocusEnd > 1e8 {
				return false
			}
			for _, block := range []string{item.StartID, item.EndID, item.FocusID, item.ZoomInID} {
				if block != "" && !nativeID.MatchString(block) {
					return false
				}
			}
		} else {
			var item *uiDialogPosition
			if !uiKeyName.MatchString(id) || Decode(bytes.NewReader(value), &item) != nil || item == nil ||
				item.Width < 0 || item.Width > 32768 || item.Height < 0 || item.Height > 32768 ||
				item.Left < -32768 || item.Left > 32768 || item.Top < -32768 || item.Top > 32768 {
				return false
			}
		}
	}
	return true
}

func validUILayout(raw json.RawMessage) bool {
	var layout map[string]json.RawMessage
	return len(raw) <= 128<<10 && json.Unmarshal(raw, &layout) == nil && layout != nil
}

func loadUILayout(ctx context.Context, l *workspace.Lease) json.RawMessage {
	raw, err := l.NativeReadStorage(ctx, "local.get", "", "")
	if err != nil {
		return json.RawMessage("{}")
	}
	var stored map[string]json.RawMessage
	if json.Unmarshal(raw, &stored) == nil && validUILayout(stored["ui-layout"]) {
		return stored["ui-layout"]
	}
	return json.RawMessage("{}")
}

type uiOutlinePreference struct {
	KeepCurrentExpand bool `json:"keepCurrentExpand"`
}

func uiLocalWrite(request pageprefs.Request, blocks ...string) uiCall {
	return uiCall{blocks: blocks, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
		return nil, l.WriteNativeUIState(ctx, request)
	}}
}

func uiRecentTime(action string) uiRoute {
	return uiRouteFor(func(q struct{ RootID string }) (uiCall, error) {
		if action == "recent.close" && q.RootID == "" {
			return uiCall{run: func(context.Context, *workspace.Lease) (any, error) { return nil, nil }}, nil
		}
		if !nativeID.MatchString(q.RootID) {
			return uiCall{}, ErrArgument
		}
		return uiLocalWrite(pageprefs.Request{Action: action, Key: q.RootID}, q.RootID), nil
	})
}
