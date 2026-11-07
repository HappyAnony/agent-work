// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/editcore"
)

// Same label precedence as the original block helper, with owner reads supplied.
func attributeViewBlockTextWithServices(n *ast.Node, avID string, reference func(*ast.Node) (string, error), untitled func() string) (icon, content string, err error) {
	return editcore.AttributeViewBlockTextWithServices(n, avID, reference, untitled)
}
