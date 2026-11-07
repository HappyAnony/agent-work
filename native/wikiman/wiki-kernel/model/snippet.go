// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/conf"
)

func LoadSnippets() ([]*conf.Snippet, error) { return LoadSnippetsContext(context.Background()) }
func SetSnippet(s []*conf.Snippet) error     { return SetSnippetContext(context.Background(), s) }
func RemoveSnippet(id string) (*conf.Snippet, error) {
	return RemoveSnippetContext(context.Background(), id)
}
