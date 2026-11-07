package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/indexdb"
)

// Return full original key objects, not a reduced replacement AV schema. Parsing
// is read-only and detached; unknown fields remain in each returned JSON value.
func (l *Lease) NativeAttributeViewKeys(ctx context.Context, id, kind, keyword string, ids []string) (ret []json.RawMessage, err error) {
	ids = append([]string(nil), ids...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		if !documentID.MatchString(id) {
			return ErrDocument
		}
		raw, e := readNativeFile(ctx, l.workspace, "storage/av/"+id+".json", 16<<20)
		if e != nil {
			return e
		}
		var doc struct {
			ID        string `json:"id"`
			KeyValues []struct {
				Key json.RawMessage `json:"key"`
			} `json:"keyValues"`
		}
		if e = json.Unmarshal(raw, &doc); e != nil {
			return e
		}
		if doc.ID != "" && doc.ID != id {
			return ErrDocument
		}
		type item struct {
			Raw  json.RawMessage
			Meta avquery.KeyMeta
		}
		var keys []item
		for _, kv := range doc.KeyValues {
			var key struct {
				ID, Name, Type string
				Relation       json.RawMessage
			}
			if len(kv.Key) == 0 || bytes.Equal(bytes.TrimSpace(kv.Key), []byte("null")) {
				return ErrDocument
			}
			if e = json.Unmarshal(kv.Key, &key); e != nil {
				return e
			}
			relation := bytes.TrimSpace(key.Relation)
			if len(relation) > 0 && !bytes.Equal(relation, []byte("null")) && relation[0] != '{' {
				return ErrDocument
			}
			keys = append(keys, item{Raw: kv.Key, Meta: avquery.KeyMeta{ID: key.ID, Name: key.Name, Type: key.Type, Relation: len(key.Relation) > 0 && !bytes.Equal(bytes.TrimSpace(key.Relation), []byte("null"))}})
		}
		found, e := avquery.SelectKeys(ctx, keys, func(v item) avquery.KeyMeta { return v.Meta }, kind, keyword, ids)
		if e != nil {
			return e
		}
		ret = []json.RawMessage{}
		for _, k := range found {
			ret = append(ret, append(json.RawMessage(nil), k.Raw...))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return
}
