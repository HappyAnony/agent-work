// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

type File struct {
	Path         string `json:"path"`
	Name         string `json:"name"` // 标题，即 ial["title"]
	Icon         string `json:"icon"`
	Name1        string `json:"name1"` // 命名，即 ial["name"]
	Alias        string `json:"alias"`
	Memo         string `json:"memo"`
	Bookmark     string `json:"bookmark"`
	ID           string `json:"id"`
	Count        int    `json:"count"`
	Size         uint64 `json:"size"`
	HSize        string `json:"hSize"`
	Mtime        int64  `json:"mtime"`
	CTime        int64  `json:"ctime"`
	HMtime       string `json:"hMtime"`
	HCtime       string `json:"hCtime"`
	Sort         int    `json:"sort"`
	SubFileCount int    `json:"subFileCount"`
	Hidden       bool   `json:"hidden"`

	NewFlashcardCount int `json:"newFlashcardCount"`
	DueFlashcardCount int `json:"dueFlashcardCount"`
	FlashcardCount    int `json:"flashcardCount"`
}

type Entry struct {
	Path, Name string
	Size       int64
	Directory  bool
}
