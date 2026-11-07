// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"github.com/siyuan-note/logging"
	"text/template"
	"time"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/avrender"
)

func legacyAVTemplates() avrender.TemplateEnvironment {
	f := filesys.BuiltInTemplateFuncs()
	SQLTemplateFuncs(&f)
	return avrender.TemplateEnvironment{Funcs: template.FuncMap(f), Now: time.Now, Name: getAttrViewName, Warning: func(e error) { logging.LogWarnf("%s", e) }}
}
