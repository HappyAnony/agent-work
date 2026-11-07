package workspace

import (
	"encoding/json"
	"time"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

// Read adaptation uses detached JSON only. Ambiguous source IDs are never
// generated on read; old upgrade rules cannot acquire source-save authority.
func decodeNativeAV(raw json.RawMessage, id string, now time.Time) (*av.AttributeView, error) {
	a := &av.AttributeView{}
	if e := json.Unmarshal(raw, a); e != nil {
		return nil, e
	}
	if a.ID != id {
		return nil, avrender.ErrShape
	}
	if len(a.KeyValues) > 512 || len(a.Views) > 128 {
		return nil, ErrBudget
	}
	count := 0
	for _, kv := range a.KeyValues {
		if kv == nil {
			return nil, av.ErrUpgradeShape
		}
		count += len(kv.Values)
		if count > 100000 {
			return nil, ErrBudget
		}
	}
	if a.Spec < av.CurrentSpec {
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil || kv.Key.ID == "" {
				return nil, av.ErrUpgradeShape
			}
			for _, v := range kv.Values {
				if v == nil || v.ID == "" || !documentID.MatchString(v.BlockID) {
					return nil, av.ErrUpgradeShape
				}
			}
		}
		if e := av.UpgradeInMemory(a, av.UpgradeEnvironment{Now: func() time.Time { return now }, NewID: func() string { return "" }}); e != nil {
			return nil, e
		}
	}
	if e := validateNativeAVRender(a); e != nil {
		return nil, e
	}
	return a, nil
}
