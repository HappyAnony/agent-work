// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import (
	"context"
	_ "embed"
	"encoding/json"
)

// read_languages.json is the exact read-label/time subset of the inherited
// wiki-app/appearance/langs/*.json. Embedded values are read-only inputs; no
// process language, workspace config or app installation is consulted.
//
//go:embed read_languages.json
var readLanguageJSON []byte

func NewReadBusiness(reader func(context.Context) ([]Notebook, error)) *Business {
	var source map[string]struct {
		Labels map[int]string             `json:"labels"`
		Time   map[string]string          `json:"time"`
		AV     map[string]json.RawMessage `json:"av"`
	}
	if err := json.Unmarshal(readLanguageJSON, &source); err != nil {
		panic(err)
	}
	labels := map[string]map[int]string{}
	times := map[string]map[string]string{}
	for lang, value := range source {
		labels[lang] = value.Labels
		times[lang] = value.Time
	}
	b := NewBusiness(reader, labels, nil, times)
	b.avLabels = map[string]map[string]json.RawMessage{}
	for lang, value := range source {
		b.avLabels[lang] = value.AV
	}
	return b
}

// AttributeViewLabel reads inherited layout labels from the explicitly bound language resource.
func (b *Business) AttributeViewLabel(lang, key string) string {
	if b == nil {
		return ""
	}
	var text string
	_ = json.Unmarshal(b.avLabels[lang][key], &text)
	return text
}
