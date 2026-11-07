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

package model

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

// NewAppConfWithBusiness creates a detached original configuration with explicit
// business dependencies. Persistence still requires NewAppConfAt.
func NewAppConfWithBusiness(business *conf.Business) *AppConf {
	ret := NewAppConf()
	ret.business = business
	return ret
}

// Only compatibility boot may construct this process-scoped adapter. Context
// services never fall back to ListNotebooks or the process's selected source.
func legacyConfigurationBusiness() *conf.Business {
	times := map[string]map[string]string{}
	for lang, labels := range util.TimeLangs {
		copy := map[string]string{}
		for key, value := range labels {
			if s, ok := value.(string); ok {
				copy[key] = s
			}
		}
		times[lang] = copy
	}
	return conf.NewBusiness(func(ctx context.Context) ([]conf.Notebook, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items, err := ListNotebooksContext(ctx)
		if err != nil {
			return nil, err
		}
		ret := make([]conf.Notebook, 0, len(items))
		for _, item := range items {
			ret = append(ret, conf.Notebook{ID: item.ID, Name: item.Name, Icon: item.Icon, Sort: item.Sort, SortMode: item.SortMode, Closed: item.Closed, NewFlashcardCount: item.NewFlashcardCount, DueFlashcardCount: item.DueFlashcardCount, FlashcardCount: item.FlashcardCount})
		}
		return ret, nil
	}, util.Langs, map[int]string{0: util.GetCloudAccountServerForRegion(0), 1: util.GetCloudAccountServerForRegion(1)}, times)
}

func (c *AppConf) notebooks(ctx context.Context) ([]*Box, error) {
	items, err := c.business.Notebooks(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]*Box, 0, len(items))
	for _, item := range items {
		ret = append(ret, &Box{ID: item.ID, Name: item.Name, Icon: item.Icon, Sort: item.Sort, SortMode: item.SortMode, Closed: item.Closed, NewFlashcardCount: item.NewFlashcardCount, DueFlashcardCount: item.DueFlashcardCount, FlashcardCount: item.FlashcardCount})
	}
	return ret, nil
}
