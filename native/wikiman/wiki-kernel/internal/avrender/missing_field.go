// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import av "wiki-kernel/internal/avmodel"

// RemoveMissingField preserves the original first-match repair for each layout.
func RemoveMissingField(view *av.View, missingKeyID string) bool {
	changed := false
	if nil != view.Table {
		for i, column := range view.Table.Columns {
			if column.ID == missingKeyID {
				view.Table.Columns = append(view.Table.Columns[:i], view.Table.Columns[i+1:]...)
				changed = true
				break
			}
		}
	}

	if nil != view.Gallery {
		for i, cardField := range view.Gallery.CardFields {
			if cardField.ID == missingKeyID {
				view.Gallery.CardFields = append(view.Gallery.CardFields[:i], view.Gallery.CardFields[i+1:]...)
				changed = true
				break
			}
		}
	}

	if nil != view.Kanban {
		for i, kanbanField := range view.Kanban.Fields {
			if kanbanField.ID == missingKeyID {
				view.Kanban.Fields = append(view.Kanban.Fields[:i], view.Kanban.Fields[i+1:]...)
				changed = true
				break
			}
		}
	}

	return changed
}
