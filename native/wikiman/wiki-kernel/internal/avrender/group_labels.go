// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"fmt"
	av "wiki-kernel/internal/avmodel"
)

func LabelGroups(view *av.View, groupKey *av.Key, reader func(int) (string, error)) error {
	if reader == nil {
		return ErrEnvironmentUnbound
	}
	var result error
	language := func(id int) string {
		s, err := reader(id)
		if err != nil {
			result = err
		}
		return s
	}
	for _, groupView := range view.Groups {
		groupView.Name = groupView.GetGroupValue()
		switch groupView.Name {
		case groupValueDefault:
			groupView.Name = fmt.Sprintf(language(264), groupKey.Name)
		case groupValueNotInRange:
			groupView.Name = language(265)
		case groupValueLast30Days:
			groupView.Name = fmt.Sprintf(language(259), 30)
		case groupValueLast7Days:
			groupView.Name = fmt.Sprintf(language(259), 7)
		case groupValueYesterday:
			groupView.Name = language(260)
		case groupValueToday:
			groupView.Name = language(261)
		case groupValueTomorrow:
			groupView.Name = language(262)
		case groupValueNext7Days:
			groupView.Name = fmt.Sprintf(language(263), 7)
		case groupValueNext30Days:
			groupView.Name = fmt.Sprintf(language(263), 30)
		}
	}

	return result
}
