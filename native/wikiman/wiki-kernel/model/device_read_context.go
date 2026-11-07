// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func WorkspaceInfoContext(ctx context.Context) (map[string]interface{}, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	out := map[string]interface{}{"workspaceDir": base.workspace, "siyuanVer": util.Ver}
	return out, base.guard(ctx)
}

func ChangelogContext(ctx context.Context) (out map[string]interface{}, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	working, version := util.WorkingDir, util.Ver
	err = base.run(ctx, func(c, _ context.Context) error {
		out = map[string]interface{}{"show": false, "html": ""}
		var cfg conf.AppData
		if e := base.owner.WithSettingSnapshot(c, func(v conf.AppData) error { cfg = v; return nil }); e != nil {
			return e
		}
		if !cfg.ShowChangelog {
			return nil
		}
		guard := func() error {
			if working != util.WorkingDir {
				return conf.ErrBusinessUnbound
			}
			return base.guard(c)
		}
		folder := filepath.Join(working, "changelogs", "v"+version)
		names := []string{filepath.Join(folder, "v"+version+"_"+cfg.Lang+".md"), filepath.Join(folder, "v"+version+".md")}
		var raw []byte
		found := false
		for _, name := range names {
			if e := guard(); e != nil {
				return e
			}
			if _, e := os.Lstat(name); errors.Is(e, fs.ErrNotExist) {
				continue
			} else if e != nil {
				return e
			}
			var e error
			raw, e = capturePDFWatermarkFile(name).read(16<<20, guard)
			if e != nil {
				return e
			}
			found = true
			break
		}
		if !found {
			return nil
		}
		content := util.LinkTarget(lute.New().MarkdownStr("", string(raw)), "")
		if e := guard(); e != nil {
			return e
		}
		base.owner.m.Lock()
		defer base.owner.m.Unlock()
		if !base.owner.ShowChangelog {
			return nil
		}
		base.owner.ShowChangelog = false
		out = map[string]interface{}{"show": true, "html": content}
		return nil
	})
	if err != nil {
		out = nil
	}
	return
}

// This is an advisory device filename probe, not permission to publish there.
func UniqueHostFilenameContext(ctx context.Context, name string) (out string, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return "", err
	}
	err = base.run(ctx, func(c, _ context.Context) error {
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(filepath.Base(name), ext)
		for i := 0; i < 10000; i++ {
			if e := base.guard(c); e != nil {
				return e
			}
			candidate := name
			if i > 0 {
				candidate = filepath.Join(filepath.Dir(name), stem+" ("+strconv.Itoa(i)+")"+ext)
			}
			if _, e := os.Lstat(candidate); errors.Is(e, fs.ErrNotExist) {
				out = candidate
				return nil
			} else if e != nil {
				return e
			}
		}
		return errors.New("filename_probe_budget")
	})
	if err != nil {
		out = ""
	}
	return
}
