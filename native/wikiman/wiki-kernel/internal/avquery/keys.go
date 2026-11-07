// SPDX-License-Identifier: AGPL-3.0-or-later
package avquery

import (
	"context"
	"errors"
	"strings"
)

type KeyMeta struct {
	ID, Name, Type string
	Relation       bool
}

// Original key-order and duplicate requested-ID behavior are retained. The
// generic carrier keeps complete legacy keys and unknown source fields intact.
func SelectKeys[T any](ctx context.Context, keys []T, meta func(T) KeyMeta, kind, keyword string, ids []string) (ret []T, err error) {
	if meta == nil {
		return nil, errors.New("av_key_dependency_unbound")
	}
	if kind != "ids" && kind != "nonrelation" && kind != "rollupdest" && kind != "relation" {
		return nil, errors.New("invalid_av_key_query")
	}
	ret = []T{}
	for _, key := range keys {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		m := meta(key)
		if kind == "ids" {
			if len(ids) == 0 {
				ret = append(ret, key)
			} else {
				for _, id := range ids {
					if id == m.ID {
						ret = append(ret, key)
					}
				}
			}
			continue
		}
		allowed := false
		switch kind {
		case "nonrelation":
			allowed = m.Type != "relation" && m.Type != "rollup" && m.Type != "lineNumber"
		case "rollupdest":
			allowed = m.Type != "rollup" && m.Type != "lineNumber"
		case "relation":
			allowed = m.Type == "relation" && m.Relation
		}
		if allowed && strings.Contains(strings.ToLower(m.Name), strings.ToLower(keyword)) {
			ret = append(ret, key)
		}
	}
	return ret, ctx.Err()
}
