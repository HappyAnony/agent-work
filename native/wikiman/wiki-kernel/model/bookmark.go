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
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/blockview"
)

func RemoveBookmark(bookmark string) error {
	return mutateBookmarkWithEnvironment(bookmark, "", false, legacyBookmarkMutationEnvironment())
}

func RenameBookmark(oldBookmark, newBookmark string) error {
	return mutateBookmarkWithEnvironment(oldBookmark, newBookmark, true, legacyBookmarkMutationEnvironment())
}

type BookmarkLabel = blockview.BookmarkLabel
type BookmarkBlocks = blockview.BookmarkBlocks
type Bookmark = blockview.Bookmark
type Bookmarks = blockview.Bookmarks

func BookmarkLabels() []string {
	ret, err := bookmarkLabelsWithEnvironment(legacyBookmarkReadEnvironment())
	if err != nil {
		logging.LogErrorf("bookmark labels failed: %s", err)
		return []string{}
	}
	return ret
}
func BuildBookmark() *Bookmarks {
	ret, err := buildBookmarksWithEnvironment(legacyBookmarkReadEnvironment())
	if err != nil {
		logging.LogErrorf("bookmark panel failed: %s", err)
		return &Bookmarks{}
	}
	return &ret
}
