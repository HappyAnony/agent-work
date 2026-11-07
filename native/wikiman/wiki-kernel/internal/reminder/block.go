// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package reminder

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/araddon/dateparse"
	"strings"
	"time"
)

const Attribute = "custom-reminder-wechat"

var ErrDependency = errors.New("reminder_dependencies_unbound")
var ErrMissing = errors.New("reminder_block_missing")

type Payload struct {
	ID, Content  string
	Milliseconds int64
}
type Plan struct {
	Payload            Payload
	Timed, TargetID    string
	Tree               *parse.Tree
	Node               *ast.Node
	Attrs              map[string]string
	Notice             string
	NoticeMilliseconds int
}
type Services struct {
	Location *time.Location
	Content  func(context.Context, *ast.Node) (string, error)
	Language func(int) string
}

func FormatNotice(format string, t time.Time) string {
	return fmt.Sprintf(format, t.Format("2006-01-02 15:04"))
}

// Prepare only reads this owner's tree. A container targets its original first
// non-marker leaf, while the remote payload and IAL cache retain the request ID.
func Prepare(ctx context.Context, id, timed string, tree *parse.Tree, attrs map[string]string, s Services) (Plan, error) {
	if e := ctx.Err(); e != nil {
		return Plan{}, e
	}
	if s.Location == nil || s.Content == nil || s.Language == nil {
		return Plan{}, ErrDependency
	}
	var ms int64
	if timed != "0" {
		t, e := dateparse.ParseIn(timed, s.Location)
		if e != nil {
			return Plan{}, e
		}
		ms = t.UnixMilli()
	}
	if tree == nil || tree.Root == nil {
		return Plan{}, ErrMissing
	}
	var n *ast.Node
	ast.Walk(tree.Root, func(v *ast.Node, entering bool) ast.WalkStatus {
		if entering && v.ID == id {
			n = v
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if n == nil {
		return Plan{}, ErrMissing
	}
	if n.Type != ast.NodeDocument && n.IsContainerBlock() {
		var first *ast.Node
		ast.Walk(n, func(v *ast.Node, entering bool) ast.WalkStatus {
			if entering && !v.IsMarker() && !v.IsContainerBlock() {
				first = v
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		n = first
	}
	if n == nil {
		return Plan{}, ErrMissing
	}
	content, e := s.Content(ctx, n)
	if e != nil {
		return Plan{}, e
	}
	content = strings.ReplaceAll(gulu.Str.SubStr(content, 128), editor.Zwsp, "")
	p := Plan{Payload: Payload{id, content, ms}, Timed: timed, TargetID: n.ID, Tree: tree, Node: n, Attrs: map[string]string{}}
	for k, v := range attrs {
		p.Attrs[k] = v
	}
	if timed == "0" {
		delete(p.Attrs, Attribute)
		if old, e := dateparse.ParseIn(n.IALAttr(Attribute), s.Location); e == nil {
			p.Notice = s.Language(109)
			p.Notice = FormatNotice(p.Notice, old)
			p.NoticeMilliseconds = 3000
		}
	} else {
		p.Attrs[Attribute] = timed
		p.Notice = FormatNotice(s.Language(101), time.UnixMilli(ms).In(s.Location))
		p.NoticeMilliseconds = 5000
	}
	return p, ctx.Err()
}
