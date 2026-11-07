// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
	"time"
	av "wiki-kernel/internal/avmodel"
)

var ErrTemplateCycle = errors.New("AV template dependency cycle or resolution limit")

type TemplateEnvironment struct {
	Context   context.Context
	MaxOutput int
	Funcs     template.FuncMap
	Now       func() time.Time
	Name      func(*av.AttributeView) string
	Warning   func(error)
	IAL       func(*av.AttributeView, string) (map[string]string, error)
	// Resolve is operation-local; nil preserves the original eager evaluator.
	Resolve func(string) error
	Charge  func(int) error
	// Operation-local raw-value projection; nil keeps original eager identity.
	RawValue func(*av.Key, *av.Value) *av.Value
}

func (e TemplateEnvironment) warn(err error) {
	if e.Warning != nil {
		e.Warning(err)
	}
}
func RenderTemplateField(ial map[string]string, keyValues []*av.KeyValues, tplContent string, env TemplateEnvironment) (ret string, err error) {
	if env.Funcs == nil || env.Now == nil {
		return "", ErrEnvironmentUnbound
	}
	goTpl := template.New("").Delims(".action{", "}")
	tplFuncMap := template.FuncMap{}
	for name, fn := range env.Funcs {
		tplFuncMap[name] = fn
	}
	goTpl = goTpl.Funcs(tplFuncMap)
	tpl, err := goTpl.Parse(tplContent)
	if err != nil {
		env.warn(fmt.Errorf("parse template [%s] failed: %s", tplContent, err))
		return
	}

	dataModel := templateDataModel(ial, keyValues, env)
	if env.Resolve != nil {
		if err = bindLazyTemplate(tpl, dataModel, ial, keyValues, env); err != nil {
			return "", err
		}
	}
	buf := &bytes.Buffer{}
	if err = tpl.Execute(templateWriter{buf, env.Context, env.MaxOutput}, dataModel); err != nil {
		env.warn(fmt.Errorf("execute template [%s] failed: %s", tplContent, err))
		return
	}
	ret = buf.String()
	if ret == "<no value>" {
		ret = ""
		return
	}

	if HasUnclosedHtmlTag(ret) {
		ret = EscapeHTML(ret)
	}
	return
}

// templateDataModel preserves original assignment order and typed values.
func templateDataModel(ial map[string]string, keyValues []*av.KeyValues, env TemplateEnvironment) map[string]interface{} {
	copyIAL := map[string]string{}
	for k, v := range ial {
		copyIAL[k] = v
	}
	ial = copyIAL
	if "" == ial["id"] {
		block := av.GetKeyBlockValue(keyValues)
		if nil != block {
			if nil != block.Block {
				ial["id"] = block.Block.ID
			}
			if "" == ial["id"] {
				ial["id"] = block.BlockID
			}
		}
	}
	if "" == ial["updated"] {
		block := av.GetKeyBlockValue(keyValues)
		if nil != block && nil != block.Block {
			ial["updated"] = time.UnixMilli(block.Block.Updated).Format("20060102150405")
		}
	}

	dataModel := map[string]interface{}{} // 复制一份 IAL 以避免修改原始数据
	for k, v := range ial {
		dataModel[k] = v

		// Database template column supports `created` and `updated` built-in variables https://github.com/siyuan-note/siyuan/issues/9364
		createdStr := ial["id"]
		if len(createdStr) >= len("20060102150405") {
			createdStr = createdStr[:len("20060102150405")]
		}
		created, parseErr := time.ParseInLocation("20060102150405", createdStr, time.Local)
		if nil == parseErr {
			dataModel["created"] = created
		} else {
			errMsg := parseErr.Error()
			//env.warn(fmt.Errorf("parse created [%s] failed: %s", createdStr, errMsg))
			if strings.Contains(errMsg, "minute out of range") {
				// parsing time "20240709158553": minute out of range
				// 将分秒部分置为 0000
				createdStr = createdStr[:len("2006010215")] + "0000"
			} else if strings.Contains(errMsg, "second out of range") {
				// parsing time "20240709154592": second out of range
				// 将秒部分置为 00
				createdStr = createdStr[:len("200601021504")] + "00"
			}
			created, parseErr = time.ParseInLocation("20060102150405", createdStr, time.Local)
		}
		if nil != parseErr {
			env.warn(fmt.Errorf("parse created [%s] failed: %s", createdStr, parseErr))
			dataModel["created"] = env.Now()
		}
		updatedStr := ial["updated"]
		updated, parseErr := time.ParseInLocation("20060102150405", updatedStr, time.Local)
		if nil == parseErr {
			dataModel["updated"] = updated
		} else {
			dataModel["updated"] = env.Now()
		}
	}

	dataModel["id_mod"] = map[string]any{}
	dataModel["id_mod_raw"] = map[string]any{}
	for _, keyValue := range keyValues {
		if 1 > len(keyValue.Values) {
			continue
		}

		v := keyValue.Values[0]
		if av.KeyTypeNumber == v.Type {
			if nil != v.Number && v.Number.IsNotEmpty {
				dataModel[keyValue.Key.Name] = v.Number.Content
			}
		} else if av.KeyTypeDate == v.Type {
			if nil != v.Date {
				if v.Date.IsNotEmpty {
					dataModel[keyValue.Key.Name] = time.UnixMilli(v.Date.Content)
				}
				if v.Date.IsNotEmpty2 {
					dataModel[keyValue.Key.Name+"_end"] = time.UnixMilli(v.Date.Content2)
				}
			}
		} else if av.KeyTypeRollup == v.Type {
			if 0 < len(v.Rollup.Contents) {
				var numbers []float64
				var contents []string
				for _, content := range v.Rollup.Contents {
					if av.KeyTypeNumber == content.Type {
						numbers = append(numbers, content.Number.Content)
					} else if av.KeyTypeMSelect == content.Type {
						for _, s := range content.MSelect {
							contents = append(contents, s.Content)
						}
					} else {
						contents = append(contents, content.String(true))
					}
				}

				if 0 < len(numbers) {
					dataModel[keyValue.Key.Name] = numbers
				} else {
					dataModel[keyValue.Key.Name] = contents
				}
			}
		} else if av.KeyTypeRelation == v.Type {
			if 0 < len(v.Relation.Contents) {
				var contents []string
				for _, content := range v.Relation.Contents {
					contents = append(contents, content.String(true))
				}
				dataModel[keyValue.Key.Name] = contents
			}
		} else if av.KeyTypeBlock == v.Type {
			dataModel[keyValue.Key.Name+"_created"] = env.Now()
			if nil != v.Block {
				dataModel["entryCreated"] = time.UnixMilli(v.Block.Created)
			}
			dataModel["entryUpdated"] = env.Now()
			if nil != v.Block {
				dataModel["entryUpdated"] = time.UnixMilli(v.Block.Updated)
			}
			dataModel[keyValue.Key.Name] = v.String(true)
		} else if av.KeyTypeMSelect == v.Type {
			dataModel[keyValue.Key.Name+"_str"] = v.String(true)
			var contents []string
			for _, s := range v.MSelect {
				contents = append(contents, s.Content)
			}
			dataModel[keyValue.Key.Name] = contents
		} else {
			dataModel[keyValue.Key.Name] = v.String(true)
		}

		// Database template fields support access to the raw value https://github.com/siyuan-note/siyuan/issues/14903
		raw := v
		if env.RawValue != nil {
			raw = env.RawValue(keyValue.Key, v)
		}
		dataModel[keyValue.Key.Name+"_raw"] = raw

		// Database template fields support access by ID https://github.com/siyuan-note/siyuan/issues/11237
		dataModel["id_mod"].(map[string]any)[keyValue.Key.ID] = dataModel[keyValue.Key.Name]
		dataModel["id_mod_raw"].(map[string]any)[keyValue.Key.ID] = raw
	}

	return dataModel
}

type templateWriter struct {
	buffer *bytes.Buffer
	ctx    context.Context
	limit  int
}

func (w templateWriter) Write(p []byte) (int, error) {
	if w.ctx != nil {
		if e := w.ctx.Err(); e != nil {
			return 0, e
		}
	}
	if w.limit > 0 && len(p) > w.limit-w.buffer.Len() {
		return 0, ErrLimit
	}
	return w.buffer.Write(p)
}

var _ io.Writer = templateWriter{}

func FillTemplateValues(attrView *av.AttributeView, view *av.View, collection av.Collection, ials map[string]map[string]string, env TemplateEnvironment) (err error) {
	if env.Funcs == nil || env.Now == nil || env.Name == nil {
		return ErrEnvironmentUnbound
	}
	items := GenerateItems(attrView, view)
	existTemplateField := false
	for _, kVals := range attrView.KeyValues {
		if av.KeyTypeTemplate == kVals.Key.Type {
			existTemplateField = true
			break
		}
	}
	if !existTemplateField {
		return
	}

	templateKeys, resolved := GetTemplateKeysByResolutionOrder(attrView, env)
	if !resolved {
		return ErrTemplateCycle
	}
	for _, templateKey := range templateKeys {
		for _, item := range collection.GetItems() {
			value := item.GetValue(templateKey.ID)
			if nil == value || nil == value.Template {
				continue
			}

			keyValues := items[item.GetID()]
			var ial map[string]string
			blockVal := item.GetBlockValue()
			if nil != blockVal && blockVal.Block != nil {
				ial = ials[blockVal.Block.ID]
			}
			if nil == ial {
				ial = map[string]string{}
			}

			content, renderErr := RenderTemplateField(ial, keyValues, value.Template.Content, env)
			if nil != renderErr {
				key, _ := attrView.GetKey(value.KeyID)
				keyName := ""
				if nil != key {
					keyName = key.Name
				}
				err = fmt.Errorf("database [%s] template field [%s] rendering failed: %w", env.Name(attrView), keyName, renderErr)
			}

			value.Template.Content = content
			items[item.GetID()] = append(keyValues, &av.KeyValues{Key: templateKey, Values: []*av.Value{value}})
		}
	}
	return
}

func GetTemplateKeysByResolutionOrder(attrView *av.AttributeView, env TemplateEnvironment) (ret []*av.Key, resolved bool) {
	ret = []*av.Key{}

	resolvedTemplateKeys := map[string]bool{}
	for i := 0; i < 7; i++ {
		templateKeyCount := 0
		for _, keyValues := range attrView.KeyValues {
			if av.KeyTypeTemplate != keyValues.Key.Type {
				continue
			}

			templateKeyCount++
			if resolvedTemplateKeys[keyValues.Key.ID] {
				continue
			}
			vars, err := getTemplateVars(keyValues.Key.Template, env)
			if nil != err {
				resolvedTemplateKeys[keyValues.Key.ID] = true
				ret = append(ret, keyValues.Key)
				continue
			}

			currentTemplateKeyResolved := true
			for _, kValues := range attrView.KeyValues {
				if slices.Contains(vars, kValues.Key.Name) {
					if av.KeyTypeTemplate == kValues.Key.Type {
						if _, ok := resolvedTemplateKeys[kValues.Key.ID]; !ok {
							currentTemplateKeyResolved = false
							break
						}
					}
				}
			}
			if currentTemplateKeyResolved {
				resolvedTemplateKeys[keyValues.Key.ID] = true
				ret = append(ret, keyValues.Key)
			}
		}

		resolved = len(resolvedTemplateKeys) == templateKeyCount
		if resolved {
			break
		}
	}
	return
}

func GetTemplateKeyRelevantKeys(attrView *av.AttributeView, templateKey *av.Key, env TemplateEnvironment) (ret []*av.Key) {
	ret = []*av.Key{}
	if nil == templateKey || "" == templateKey.Template {
		return
	}

	vars, err := getTemplateVars(templateKey.Template, env)
	if nil != err {
		return
	}

	for _, kValues := range attrView.KeyValues {
		if slices.Contains(vars, kValues.Key.Name) {
			ret = append(ret, kValues.Key)
		}
	}

	if 1 > len(ret) {
		// 没有相关字段情况下直接尝试解析模板，如果能解析成功则返回模板字段本身 https://github.com/siyuan-note/siyuan/issues/15560#issuecomment-3182691193
		goTpl := template.New("").Delims(".action{", "}")
		tplFuncMap := env.Funcs
		goTpl = goTpl.Funcs(tplFuncMap)
		_, parseErr := goTpl.Funcs(tplFuncMap).Parse(templateKey.Template)
		if nil != parseErr {
			return
		}
		ret = append(ret, templateKey)
	}
	return
}

func getTemplateVars(tplContent string, env TemplateEnvironment) ([]string, error) {
	goTpl := template.New("").Delims(".action{", "}")
	tplFuncMap := env.Funcs
	goTpl = goTpl.Funcs(tplFuncMap)
	tpl, parseErr := goTpl.Funcs(tplFuncMap).Parse(tplContent)
	if parseErr != nil {
		return nil, parseErr
	}
	vars := make(map[string]struct{})
	collectVars(tpl.Tree.Root, vars)
	var result []string
	for v := range vars {
		result = append(result, v)
	}
	return result, nil
}

func collectVars(node parse.Node, vars map[string]struct{}) {
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			collectVars(child, vars)
		}
	case *parse.ActionNode:
		collectVars(n.Pipe, vars)
	case *parse.PipeNode:
		for _, cmd := range n.Cmds {
			collectVars(cmd, vars)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			collectVars(arg, vars)
		}

		if 3 <= len(n.Args) && n.Args[0].Type() == parse.NodeIdentifier && n.Args[1].Type() == parse.NodeDot && n.Args[2].Type() == parse.NodeString {
			vars[n.Args[2].(*parse.StringNode).Text] = struct{}{}
		}

	case *parse.FieldNode:
		if len(n.Ident) > 0 {
			vars[n.Ident[0]] = struct{}{}
		}
	case *parse.VariableNode:
		if len(n.Ident) > 0 {
			vars[n.Ident[0]] = struct{}{}
		}
	}
}
func EscapeHTML(s string) (ret string) {
	ret = s
	if "" == strings.TrimSpace(ret) {
		return
	}

	ret = html.EscapeString(ret)
	return
}
func HasUnclosedHtmlTag(htmlStr string) bool {
	// 检查未闭合注释
	openIdx := 0
	for {
		start := strings.Index(htmlStr[openIdx:], "<!--")
		if start == -1 {
			break
		}
		start += openIdx
		end := strings.Index(htmlStr[start+4:], "-->")
		if end == -1 {
			return true // 存在未闭合注释
		}
		openIdx = start + 4 + end + 3
	}

	// 去除所有注释内容
	commentRe := regexp.MustCompile(`<!--[\s\S]*?-->`)
	htmlStr = commentRe.ReplaceAllString(htmlStr, "")

	tagRe := regexp.MustCompile(`<(/?)([a-zA-Z0-9]+)[^>]*?>`)
	selfClosing := map[string]bool{
		"br": true, "img": true, "hr": true, "input": true, "meta": true, "link": true,
	}
	stack := []string{}
	matches := tagRe.FindAllStringSubmatch(htmlStr, -1)
	for _, m := range matches {
		isClose := m[1] == "/"
		tag := strings.ToLower(m[2])
		if selfClosing[tag] {
			continue
		}
		if !isClose {
			stack = append(stack, tag)
		} else {
			if len(stack) == 0 || stack[len(stack)-1] != tag {
				return true // 闭合标签不匹配
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) != 0
}
