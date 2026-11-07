// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pageprefs

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrDependency = errors.New("page_preferences_unbound")
var ErrValue = errors.New("page_preferences_invalid")

type Request struct {
	Action, Key, SortBy string
	Keys                []string
	Value               json.RawMessage
}
type Backend struct {
	Read          func(context.Context, string) ([]byte, error)
	Write         func(context.Context, string, []byte) error
	Paths         func(context.Context, []string) (map[string]string, error)
	Now           func() time.Time
	BadName       func() error
	RepairRecent  bool
	RefreshRecent bool
}
type Service struct {
	once sync.Once
	gate chan struct{}
}

func (s *Service) Do(ctx context.Context, q Request, b Backend) (out any, err error) {
	if ctx == nil || b.Read == nil || b.Write == nil || b.Now == nil {
		return nil, ErrDependency
	}
	s.once.Do(func() { s.gate = make(chan struct{}, 1) })
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	q.Value = append(json.RawMessage(nil), q.Value...)
	q.Keys = append([]string(nil), q.Keys...)
	family, action, ok := strings.Cut(q.Action, ".")
	if !ok {
		return nil, ErrValue
	}
	names := map[string]string{"recent": "recent-doc.json", "criteria": "criteria.json", "local": "local.json", "outline": "outline.json"}
	name, ok := names[family]
	if !ok {
		return nil, ErrValue
	}
	raw, err := b.Read(ctx, name)
	if errors.Is(err, fs.ErrNotExist) {
		raw = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var value any
	save, compact := false, family == "outline"
	switch family {
	case "recent":
		var docs []*RecentDoc
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &docs); err != nil {
				if !b.RepairRecent {
					return nil, err
				}
				docs = []*RecentDoc{}
				save = true
			}
		}
		if len(docs) > 0 && b.RefreshRecent {
			if b.Paths == nil {
				return nil, ErrDependency
			}
			ids := make([]string, 0, len(docs))
			for _, d := range docs {
				if d == nil {
					return nil, ErrValue
				}
				ids = append(ids, d.RootID)
			}
			paths, e := b.Paths(ctx, ids)
			if e != nil {
				return nil, e
			}
			kept := docs[:0]
			for _, d := range docs {
				if p, found := paths[d.RootID]; found {
					d.Title = path.Base(p)
					kept = append(kept, d)
				} else if b.RepairRecent {
					save = true
				}
			}
			docs = kept
		}
		switch action {
		case "get":
			SortRecent(docs, q.SortBy)
		case "compact":
			if docs == nil {
				docs = []*RecentDoc{}
			}
			SortRecent(docs, q.SortBy)
			save = true
		case "remove":
			keys := map[string]bool{}
			for _, id := range q.Keys {
				keys[id] = true
			}
			next := docs[:0]
			for _, d := range docs {
				if !keys[d.RootID] {
					next = append(next, d)
				}
			}
			docs = next
			save = true
		case "record":
			d := &RecentDoc{}
			if err = json.Unmarshal(q.Value, d); err != nil {
				return nil, err
			}
			now := b.Now().Unix()
			d.ViewedAt, d.OpenAt, d.ClosedAt = now, now, 0
			next := []*RecentDoc{d}
			for _, v := range docs {
				if v.RootID != d.RootID && len(next) < 32 {
					next = append(next, v)
				}
			}
			docs = next
			save = true
		case "open", "view", "close":
			for _, d := range docs {
				if d.RootID != q.Key {
					continue
				}
				now := b.Now().Unix()
				switch action {
				case "open":
					d.OpenAt = now
				case "view":
					d.ViewedAt = now
				case "close":
					d.ClosedAt = now
				}
				save = true
				break
			}
			if action == "view" {
				sort.Slice(docs, func(i, j int) bool { return docs[i].ViewedAt > docs[j].ViewedAt })
			}
		default:
			return nil, ErrValue
		}
		value = docs
		out = docs
	case "criteria":
		docs := []*Criterion{}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &docs); err != nil {
				return nil, err
			}
		}
		for _, v := range docs {
			if v == nil {
				return nil, ErrValue
			}
		}
		switch action {
		case "get":
		case "remove":
			for i, v := range docs {
				if v.Name == q.Key {
					docs = append(docs[:i], docs[i+1:]...)
					break
				}
			}
			save = true
		case "set":
			v := &Criterion{}
			if err = json.Unmarshal(q.Value, v); err != nil {
				return nil, err
			}
			if v.Name == "" {
				if b.BadName != nil {
					return nil, b.BadName()
				}
				return nil, ErrValue
			}
			found := false
			for i, old := range docs {
				if old.Name == v.Name {
					docs[i] = v
					found = true
					break
				}
			}
			if !found {
				docs = append(docs, v)
			}
			save = true
		default:
			return nil, ErrValue
		}
		value = docs
		out = docs
	case "local":
		v := map[string]interface{}{}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
		}
		if v == nil {
			v = map[string]interface{}{}
		}
		switch action {
		case "get":
		case "remove":
			for _, k := range q.Keys {
				delete(v, k)
			}
			save = true
		case "setEntry":
			if len(q.Keys) != 1 || q.Keys[0] == "" || !json.Valid(q.Value) {
				return nil, ErrValue
			}
			entries, _ := v[q.Key].(map[string]interface{})
			if entries == nil {
				entries = map[string]interface{}{}
			}
			if _, exists := entries[q.Keys[0]]; !exists && len(entries) >= 256 {
				return nil, ErrValue
			}
			entries[q.Keys[0]] = q.Value
			v[q.Key], save = entries, true
		case "setValue":
			var x interface{}
			if err = json.Unmarshal(q.Value, &x); err != nil {
				return nil, err
			}
			v[q.Key] = x
			save = true
		case "set":
			if err = json.Unmarshal(q.Value, &v); err != nil || v == nil {
				return nil, ErrValue
			}
			save = true
		default:
			return nil, ErrValue
		}
		value = v
		out = v
	case "outline":
		docs := []*OutlineDoc{}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &docs); err != nil {
				return nil, err
			}
		}
		for _, v := range docs {
			if v == nil {
				return nil, ErrValue
			}
		}
		out = map[string]interface{}{}
		switch action {
		case "get":
			for _, d := range docs {
				if d.DocID == q.Key {
					out = d.Data
					break
				}
			}
		case "set", "remove":
			for i, d := range docs {
				if d.DocID == q.Key {
					docs = append(docs[:i], docs[i+1:]...)
					break
				}
			}
			if action == "set" {
				v := map[string]interface{}{}
				if err = json.Unmarshal(q.Value, &v); err != nil {
					return nil, err
				}
				if v == nil {
					v = map[string]interface{}{}
				}
				docs = append([]*OutlineDoc{{DocID: q.Key, Data: v}}, docs...)
				if len(docs) > 2000 {
					docs = docs[:2000]
				}
			}
			save = true
		default:
			return nil, ErrValue
		}
		value = docs
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if save {
		if compact {
			raw, err = json.Marshal(value)
		} else {
			raw, err = json.MarshalIndent(value, "", "  ")
		}
		if err != nil {
			return nil, err
		}
		if err = b.Write(ctx, name, raw); err != nil {
			return nil, err
		}
	}
	return out, ctx.Err()
}

func SortRecent(docs []*RecentDoc, sortBy string) {
	sort.Slice(docs, func(i, j int) bool {
		a, b := docs[i], docs[j]
		switch sortBy {
		case "closedAt":
			if a.ClosedAt == 0 && b.ClosedAt == 0 {
				return a.ViewedAt > b.ViewedAt
			}
			if a.ClosedAt == 0 {
				return false
			}
			if b.ClosedAt == 0 {
				return true
			}
			return a.ClosedAt > b.ClosedAt
		case "openAt":
			if a.OpenAt == 0 && b.OpenAt == 0 {
				return a.RootID > b.RootID
			}
			if a.OpenAt == 0 {
				return false
			}
			if b.OpenAt == 0 {
				return true
			}
			return a.OpenAt > b.OpenAt
		default:
			if a.ViewedAt == 0 && b.ViewedAt == 0 {
				return a.RootID > b.RootID
			}
			if a.ViewedAt == 0 {
				return false
			}
			if b.ViewedAt == 0 {
				return true
			}
			return a.ViewedAt > b.ViewedAt
		}
	})
}
