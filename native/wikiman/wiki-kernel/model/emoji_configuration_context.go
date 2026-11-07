// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// The compatibility catalogue retains its one-level rename policy, with an
// explicit write capability. Native parser manifests continue to refuse repair.
func EmojiConfigurationContext(ctx context.Context) (out []map[string]interface{}, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	built := capturePDFWatermarkFile(filepath.Join(util.AppearancePath, "emojis", "conf.json"))
	err = base.run(ctx, func(c, _ context.Context) (result error) {
		raw, e := built.read(16<<20, func() error { return base.guard(c) })
		if e != nil {
			return e
		}
		var bundled []map[string]interface{}
		if e = json.Unmarshal(raw, &bundled); e != nil {
			return e
		}
		root, e := os.OpenRoot(base.data)
		if e != nil {
			return e
		}
		identity, e := root.Stat(".")
		if e != nil {
			return errors.Join(e, root.Close())
		}
		guard := func(c context.Context) error {
			if e := base.guard(c); e != nil {
				return e
			}
			current, e := os.Lstat(base.data)
			if e != nil {
				return e
			}
			if current.Mode()&fs.ModeSymlink != 0 || !os.SameFile(identity, current) {
				return indexrepair.ErrSourceChanged
			}
			return nil
		}
		closed := false
		closeRoot := func() error {
			if closed {
				return nil
			}
			closed = true
			return root.Close()
		}
		defer func() { result = errors.Join(result, closeRoot()) }()
		source := indexrepair.Source{Data: root, Authorize: func(c context.Context) error {
			if e := guard(c); e != nil {
				return e
			}
			if base.owner.persistenceReadOnly {
				return ErrSettingReadOnly
			}
			return nil
		}}
		entries, e := source.ReadDir(c, "emojis")
		if indexrepair.IsSourceNotExist(e) {
			entries = nil
			e = nil
		}
		if e != nil {
			return e
		}
		items := []map[string]interface{}{}
		values := map[string]string{}
		add := func(name string) {
			short := strings.TrimSuffix(name, path.Ext(name))
			items = append(items, map[string]interface{}{"unicode": name, "description": short, "description_zh_cn": short, "description_ja_jp": short, "keywords": short})
			values[short] = "/emojis/" + name
		}
		clean := func(parent, name string) (string, error) {
			if util.IsValidUploadFileName(html.UnescapeString(name)) {
				return name, nil
			}
			filtered := util.FilterUploadEmojiFileName(name)
			if filtered == "" || path.Base(filtered) != filtered {
				return "", indexrepair.ErrSourcePath
			}
			if e := source.Rename(c, path.Join(parent, name), path.Join(parent, filtered)); e != nil {
				return "", e
			}
			return filtered, nil
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if e = base.guard(c); e != nil {
				return e
			}
			name, e := clean("emojis", entry.Name())
			if e != nil {
				return e
			}
			info, e := root.Lstat(path.Join("emojis", name))
			if e != nil {
				return e
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return indexrepair.ErrSourcePath
			}
			if info.IsDir() {
				children, e := source.ReadDir(c, path.Join("emojis", name))
				if e != nil {
					return e
				}
				for _, child := range children {
					if child.IsDir() || strings.HasPrefix(child.Name(), ".") {
						continue
					}
					if child.Type()&fs.ModeSymlink != 0 {
						return indexrepair.ErrSourcePath
					}
					sub, e := clean(path.Join("emojis", name), child.Name())
					if e != nil {
						return e
					}
					add(name + "/" + sub)
				}
			} else if info.Mode().IsRegular() {
				add(name)
			} else {
				return indexrepair.ErrSourcePath
			}
			if len(items) > 10000 {
				return errors.New("emoji_catalogue_budget")
			}
		}
		if e = closeRoot(); e != nil {
			return e
		}
		if e = guard(c); e != nil {
			return e
		}
		base.owner.customEmojis.Replace(values)
		custom := map[string]interface{}{"id": "custom", "title": "Custom", "title_zh_cn": "自定义", "title_ja_jp": "カスタム", "items": items}
		out = append([]map[string]interface{}{custom}, bundled...)
		return nil
	})
	if err != nil {
		out = nil
	}
	return
}
