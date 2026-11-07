// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package conf

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Notebook preserves the original notebook presentation fields. It conveys no
// filesystem or mutation authority. A reader must be bound to one source owner.
type Notebook struct {
	ID, Name, Icon                                       string
	Sort, SortMode                                       int
	Closed                                               bool
	NewFlashcardCount, DueFlashcardCount, FlashcardCount int
}

var ErrBusinessUnbound = errors.New("configuration_business_unbound")

// Business contains explicit dependencies of the original configuration
// business methods. Languages are copied once; notebook reads remain live and
// must run inside their caller's source admission/lease.
type Business struct {
	avLabels       map[string]map[string]json.RawMessage
	notebooks      func(context.Context) ([]Notebook, error)
	languages      map[string]map[int]string
	accountServers map[int]string
	timeLanguages  map[string]map[string]string
}

func NewBusiness(reader func(context.Context) ([]Notebook, error), languages map[string]map[int]string, accountServers map[int]string, timeLanguages ...map[string]map[string]string) *Business {
	b := &Business{notebooks: reader, languages: make(map[string]map[int]string), accountServers: make(map[int]string)}
	b.timeLanguages = map[string]map[string]string{}
	for _, all := range timeLanguages {
		for lang, labels := range all {
			copy := map[string]string{}
			for k, v := range labels {
				copy[k] = v
			}
			b.timeLanguages[lang] = copy
		}
	}
	for region, server := range accountServers {
		b.accountServers[region] = server
	}
	for lang, entries := range languages {
		copy := make(map[int]string, len(entries))
		for key, value := range entries {
			copy[key] = value
		}
		b.languages[lang] = copy
	}
	return b
}

// Notebooks returns detached values, including all original presentation fields.
// Filtering and the historical GetBoxes projection belong to the caller.
func (b *Business) Notebooks(ctx context.Context) ([]Notebook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil || b.notebooks == nil {
		return nil, ErrBusinessUnbound
	}
	items, err := b.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]Notebook{}, items...), nil
}

func (b *Business) Language(lang string, number int, expandAccount bool, region int) string {
	if b == nil {
		return ""
	}
	value := b.languages[lang][number]
	if value == "" {
		value = b.languages["en_US"][number]
	}
	if expandAccount {
		if region != 0 {
			region = 1
		} // original cloud-region semantics
		value = strings.ReplaceAll(value, "${accountServer}", b.accountServers[region])
	}
	return value
}
