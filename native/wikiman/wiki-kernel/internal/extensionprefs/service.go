// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package extensionprefs

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"
	"sync"
	"wiki-kernel/conf"
)

var ErrDependency = errors.New("extension_preferences_unbound")
var ErrValue = errors.New("extension_preferences_invalid")
var ErrIncompatible = errors.New("extension_plugin_incompatible")
var ErrRepairRequired = errors.New("extension_preferences_repair_required")

type Petal struct {
	Name              string                 `json:"name"`
	DisplayName       string                 `json:"displayName"`
	Enabled           bool                   `json:"enabled"`
	Incompatible      bool                   `json:"incompatible"`
	DisabledInPublish bool                   `json:"disabledInPublish"`
	JS                string                 `json:"js"`
	CSS               string                 `json:"css"`
	I18n              map[string]interface{} `json:"i18n"`
}
type Package struct {
	Name              string            `json:"name"`
	DisplayName       map[string]string `json:"displayName"`
	Backends          []string          `json:"backends"`
	Frontends         []string          `json:"frontends"`
	DisabledInPublish bool              `json:"disabledInPublish"`
}

func Incompatible(p Package, backend, frontend string) bool {
	if len(p.Backends) == 0 {
		return false
	}
	b, f := false, false
	for _, v := range p.Backends {
		if v == backend || v == "all" {
			b = true
		}
	}
	for _, v := range p.Frontends {
		if v == frontend || v == "all" {
			f = true
		}
	}
	return !b || !f
}
func PreferredName(p Package, lang string) string {
	if p.DisplayName == nil {
		return p.Name
	}
	ret := p.DisplayName["default"]
	switch lang {
	case "ar_SA", "de_DE", "en_US", "es_ES", "fr_FR", "he_IL", "it_IT", "ja_JP", "pl_PL", "pt_BR", "ru_RU", "zh_CHT", "zh_CN":
	default:
		lang = "en_US"
	}
	if p.DisplayName[lang] != "" {
		ret = p.DisplayName[lang]
	}
	if ret == "" {
		ret = p.Name
	}
	return ret
}

type Request struct {
	Action, Name, Frontend string
	Enabled, Publish       bool
	Value                  json.RawMessage
	FilterSnippets         bool
	SnippetType, Keyword   string
	SnippetEnabled         int
}
type Ports struct {
	Read                                    func(context.Context, string) ([]byte, error)
	Write                                   func(context.Context, string, []byte) error
	ReadDir                                 func(context.Context, string) ([]fs.DirEntry, error)
	NewID                                   func() string
	Language, Backend                       string
	Trusted, Disabled, RequireTrust, Repair bool
}
type Service struct {
	once sync.Once
	gate chan struct{}
}

func (s *Service) Do(ctx context.Context, q Request, p Ports) (out any, err error) {
	if ctx == nil || p.Read == nil || p.Write == nil || p.ReadDir == nil || p.NewID == nil {
		return nil, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	s.once.Do(func() { s.gate = make(chan struct{}, 1) })
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	q.Value = append(json.RawMessage(nil), q.Value...)
	if strings.HasPrefix(q.Action, "snippet.") {
		v, e := s.snippets(ctx, q, p)
		if e != nil {
			return nil, e
		}
		if q.Action == "snippet.list" && q.FilterSnippets {
			return FilterSnippets(v.([]*conf.Snippet), q.SnippetType, q.SnippetEnabled, q.Keyword, q.Publish), nil
		}
		return v, nil
	}
	if q.Action != "petal.list" && q.Action != "petal.set" && q.Action != "petal.metadata" && q.Action != "petal.replace" {
		return nil, ErrValue
	}
	if q.Action == "petal.list" && (p.Disabled || (p.RequireTrust && !p.Trusted)) {
		return []*Petal{}, nil
	}
	const filename = "storage/petal/petals.json"
	raw, err := p.Read(ctx, filename)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return nil, err
	}
	petals := []*Petal{}
	if !missing {
		if petals, err = DecodePetals(raw); err != nil {
			return nil, err
		}
	}
	filtered := []*Petal{}
	changed := missing
	for _, v := range petals {
		if v == nil || !validName(v.Name) {
			return nil, ErrValue
		}
		entries, e := p.ReadDir(ctx, path.Join("plugins", v.Name))
		if errors.Is(e, fs.ErrNotExist) {
			changed = true
			continue
		}
		if e != nil {
			return nil, e
		}
		has := false
		for _, d := range entries {
			if !d.IsDir() {
				has = true
				break
			}
		}
		if has {
			filtered = append(filtered, v)
		} else {
			changed = true
		}
	}
	petals = filtered
	if q.Action == "petal.replace" {
		if err = json.Unmarshal(q.Value, &petals); err != nil {
			return nil, err
		}
		for _, v := range petals {
			if v == nil || !validName(v.Name) {
				return nil, ErrValue
			}
		}
		changed = true
	}
	var selected *Petal
	if q.Action == "petal.set" {
		if !validName(q.Name) {
			return nil, ErrValue
		}
		pkg, e := readPackage(ctx, p, q.Name)
		if e != nil {
			return nil, e
		}
		for _, v := range petals {
			if v.Name == q.Name {
				selected = v
				break
			}
		}
		if selected == nil {
			selected = &Petal{Name: q.Name}
			petals = append(petals, selected)
		}
		projectMetadata(selected, pkg, p, q.Frontend)
		if selected.Incompatible {
			return nil, ErrIncompatible
		}
		selected.Enabled = q.Enabled
		changed = true
	}
	if changed && (p.Repair || q.Action == "petal.set" || q.Action == "petal.replace") {
		if petals == nil {
			petals = []*Petal{}
		}
		raw, err = json.MarshalIndent(petals, "", "\t")
		if err != nil {
			return nil, err
		}
		if err = p.Write(ctx, filename, raw); err != nil {
			return nil, err
		}
	}
	if q.Action == "petal.set" {
		if err = loadCode(ctx, selected, p); err != nil {
			return nil, err
		}
		return selected, ctx.Err()
	}
	if q.Action == "petal.metadata" || q.Action == "petal.replace" {
		return petals, ctx.Err()
	}
	ret := []*Petal{}
	for _, v := range petals {
		pkg, e := readPackage(ctx, p, v.Name)
		if e != nil {
			return nil, e
		}
		projectMetadata(v, pkg, p, q.Frontend)
		if !v.Enabled || v.Incompatible || (q.Publish && v.DisabledInPublish) {
			continue
		}
		if e = loadCode(ctx, v, p); e != nil {
			return nil, e
		}
		ret = append(ret, v)
	}
	return ret, ctx.Err()
}
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && path.Base(s) == s && !strings.ContainsAny(s, "\\\x00")
}

func FilterSnippets(values []*conf.Snippet, typ string, enabled int, keyword string, publish bool) []*conf.Snippet {
	ret := []*conf.Snippet{}
	keyword = strings.ToLower(keyword)
	for _, s := range values {
		if s == nil || publish && s.DisabledInPublish || typ != "all" && s.Type != typ || enabled != 2 && s.Enabled != (enabled != 0) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(s.Name), keyword) && !strings.Contains(strings.ToLower(s.Content), keyword) {
			continue
		}
		ret = append(ret, s)
	}
	return ret
}
func DecodePetals(raw []byte) ([]*Petal, error) {
	petals := []*Petal{}
	if e := json.Unmarshal(raw, &petals); e != nil {
		return nil, e
	}
	for _, v := range petals {
		if v == nil || !validName(v.Name) {
			return nil, ErrValue
		}
	}
	if petals == nil {
		petals = []*Petal{}
	}
	return petals, nil
}

func readPackage(ctx context.Context, p Ports, name string) (pkg Package, err error) {
	raw, err := p.Read(ctx, path.Join("plugins", name, "plugin.json"))
	if err != nil {
		return pkg, err
	}
	err = json.Unmarshal(raw, &pkg)
	return pkg, err
}
func projectMetadata(v *Petal, pkg Package, p Ports, frontend string) {
	v.DisplayName = PreferredName(pkg, p.Language)
	v.Incompatible = Incompatible(pkg, p.Backend, frontend)
	v.DisabledInPublish = pkg.DisabledInPublish
}
func loadCode(ctx context.Context, v *Petal, p Ports) error {
	base := path.Join("plugins", v.Name)
	raw, err := p.Read(ctx, path.Join(base, "index.js"))
	if err != nil {
		return err
	}
	v.JS = string(raw)
	raw, err = p.Read(ctx, path.Join(base, "index.css"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		v.CSS = string(raw)
	}
	dir := path.Join(base, "i18n")
	entries, err := p.ReadDir(ctx, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	preferred := p.Language + ".json"
	if path.Base(preferred) != preferred || strings.Contains(preferred, "\\") {
		return ErrValue
	}
	found, en, zh := false, false, false
	for _, e := range entries {
		switch e.Name() {
		case preferred:
			found = true
		case "en_US.json":
			en = true
		case "zh_CN.json":
			zh = true
		}
	}
	if !found {
		switch {
		case en:
			preferred = "en_US.json"
		case zh:
			preferred = "zh_CN.json"
		default:
			preferred = entries[0].Name()
		}
	}
	raw, err = p.Read(ctx, path.Join(dir, preferred))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &v.I18n)
}
func (s *Service) snippets(ctx context.Context, q Request, p Ports) (any, error) {
	const filename = "snippets/conf.json"
	raw, err := p.Read(ctx, filename)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	snippets := []*conf.Snippet{}
	if err == nil {
		if err = json.Unmarshal(raw, &snippets); err != nil {
			return nil, err
		}
	}
	changed := false
	for _, v := range snippets {
		if v == nil {
			return nil, ErrValue
		}
		if v.ID == "" {
			if !p.Repair && q.Action == "snippet.list" {
				return nil, ErrRepairRequired
			}
			v.ID = p.NewID()
			if v.ID == "" {
				return nil, ErrDependency
			}
			changed = p.Repair
		}
	}
	var selected *conf.Snippet
	switch q.Action {
	case "snippet.list":
	case "snippet.set":
		if err = json.Unmarshal(q.Value, &snippets); err != nil {
			return nil, err
		}
		for _, v := range snippets {
			if v == nil {
				return nil, ErrValue
			}
			if v.ID == "" {
				v.ID = p.NewID()
			}
		}
		changed = true
	case "snippet.remove":
		for i, v := range snippets {
			if v.ID == q.Name {
				selected = v
				snippets = append(snippets[:i], snippets[i+1:]...)
				break
			}
		}
		changed = true
	default:
		return nil, ErrValue
	}
	if changed {
		raw, err = json.MarshalIndent(snippets, "", "  ")
		if err != nil {
			return nil, err
		}
		if err = p.Write(ctx, filename, raw); err != nil {
			return nil, err
		}
	}
	if q.Action == "snippet.remove" {
		return selected, ctx.Err()
	}
	return snippets, ctx.Err()
}
