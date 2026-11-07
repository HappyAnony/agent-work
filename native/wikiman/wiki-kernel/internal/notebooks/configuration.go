// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/gulu"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/textprefs"
)

var ErrDailyPath = errors.New("notebook_daily_path_invalid")
var ErrNameLength = errors.New("notebook_name_too_long")

type MetadataRequest struct {
	Kind, ID, Name, Icon, Untitled string
	IDs                            []string
	Patch                          json.RawMessage
	Closed                         bool
}
type MetadataChange struct {
	ID, Path      string
	Before, After []byte
}
type MetadataPlan struct {
	Changes       []MetadataChange
	Configuration *conf.BoxConf
	CreatedID     string
}

func PrepareCreate(ctx context.Context, source indexrepair.Source, mode, name, untitled string, order int, newID func() string) (out MetadataPlan, err error) {
	defer func() {
		if err != nil {
			out = MetadataPlan{}
		}
	}()
	if ctx == nil || newID == nil {
		return out, ErrPermission
	}
	name = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(name))
	if utf8.RuneCountInString(name) > 512 {
		return out, ErrNameLength
	}
	if name == "" {
		name = untitled
	}
	listed, e := Scan(ctx, source.Data, mode, order)
	if e != nil {
		return out, e
	}
	if len(listed.Repairs) > 0 {
		return out, ErrChanged
	}
	for i, box := range listed.Notebooks {
		value, before, e := Configuration(ctx, source, box.ID, mode)
		if e != nil {
			return out, e
		}
		value.Sort = i + 1
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return out, e
		}
		p, e := metadata(source.Data, box.ID, mode)
		if e != nil {
			return out, e
		}
		out.Changes = append(out.Changes, MetadataChange{ID: box.ID, Path: p, Before: before, After: raw})
	}
	id := newID()
	if !idPattern.MatchString(id) {
		return out, ErrPath
	}
	if e = source.Check(id, true); e != nil {
		return out, e
	}
	if _, e = source.Data.Lstat(id); !indexrepair.IsSourceNotExist(e) {
		if e == nil {
			return out, ErrChanged
		}
		return out, e
	}
	created, e := PrepareEmptyCreate(ctx, mode, name, untitled, id)
	if e != nil {
		return out, e
	}
	created.Changes = append(out.Changes, created.Changes...)
	return created, nil
}

// PrepareEmptyCreate stages a new notebook for an owner with no data directory.
func PrepareEmptyCreate(ctx context.Context, mode, name, untitled, id string) (out MetadataPlan, err error) {
	if ctx == nil || !idPattern.MatchString(id) {
		return out, ErrPath
	}
	name = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(name))
	if utf8.RuneCountInString(name) > 512 {
		return out, ErrNameLength
	}
	if name == "" {
		name = untitled
	}
	chosen := mode
	if chosen == "auto" {
		chosen = ".wiki"
	}
	if chosen != ".wiki" && chosen != ".siyuan" {
		return out, ErrPath
	}
	value := conf.NewBoxConf()
	value.Name = name
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return out, e
	}
	p := path.Join(id, chosen, "conf.json")
	if !fs.ValidPath(p) {
		return out, ErrPath
	}
	out.Changes = append(out.Changes, MetadataChange{ID: id, Path: p, After: raw})
	out.CreatedID = id
	out.Configuration = value
	return out, ctx.Err()
}

func trimPath(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "/")
}
func NormalizeConfiguration(v *conf.BoxConf) error {
	v.RefCreateSavePath = trimPath(v.RefCreateSavePath)
	if v.RefCreateSavePath != "" && !strings.HasSuffix(v.RefCreateSavePath, "/") {
		v.RefCreateSavePath += "/"
	}
	v.DailyNoteSavePath = trimPath(v.DailyNoteSavePath)
	if v.DailyNoteSavePath != "" && !strings.HasPrefix(v.DailyNoteSavePath, "/") {
		v.DailyNoteSavePath = "/" + v.DailyNoteSavePath
	}
	if v.DailyNoteSavePath == "/" {
		return ErrDailyPath
	}
	v.DailyNoteTemplatePath = trimPath(v.DailyNoteTemplatePath)
	if v.DailyNoteTemplatePath != "" {
		if !strings.HasSuffix(v.DailyNoteTemplatePath, ".md") {
			v.DailyNoteTemplatePath += ".md"
		}
		if !strings.HasPrefix(v.DailyNoteTemplatePath, "/") {
			v.DailyNoteTemplatePath = "/" + v.DailyNoteTemplatePath
		}
	}
	v.DocCreateSavePath = trimPath(v.DocCreateSavePath)
	return nil
}
func PrepareMetadata(ctx context.Context, source indexrepair.Source, mode string, q MetadataRequest) (out MetadataPlan, err error) {
	defer func() {
		if err != nil {
			out = MetadataPlan{}
		}
	}()
	ids := append([]string(nil), q.IDs...)
	positions := map[string]int{}
	if q.Kind != "sort" {
		ids = []string{q.ID}
	}
	for i, id := range ids {
		if e := source.Check(id, false); e != nil {
			return out, e
		}
		value, before, e := Configuration(ctx, source, id, mode)
		if e != nil {
			return out, e
		}
		switch q.Kind {
		case "closed":
			value.Closed = q.Closed
		case "sort":
			value.Sort = i + 1
		case "icon":
			value.Icon = q.Icon
			if strings.Contains(value.Icon, ".") {
				value.Icon = textprefs.FilterUploadEmojiFileName(value.Icon)
			}
		case "rename":
			if utf8.RuneCountInString(q.Name) > 512 {
				return out, ErrNameLength
			}
			value.Name = q.Name
			if value.Name == "" {
				value.Name = q.Untitled
			}
		case "configuration":
			if !valid(q.Patch, value) {
				return out, ErrPath
			}
			if e = NormalizeConfiguration(value); e != nil {
				return out, e
			}
		default:
			return out, ErrPath
		}
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return out, e
		}
		name, e := metadata(source.Data, id, mode)
		if e != nil {
			return out, e
		}
		change := MetadataChange{ID: id, Path: name, Before: before, After: raw}
		if at, ok := positions[id]; ok {
			out.Changes[at] = change
		} else {
			positions[id] = len(out.Changes)
			out.Changes = append(out.Changes, change)
		}
		out.Configuration = value
	}
	return out, ctx.Err()
}

// Configuration preserves original default values only for genuine absence.
// Selected malformed/unreadable metadata never grants a default overwrite.
func Configuration(ctx context.Context, source indexrepair.Source, id, mode string) (value *conf.BoxConf, before []byte, err error) {
	name, e := metadata(source.Data, id, mode)
	if e != nil {
		return nil, nil, e
	}
	raw, e := source.ReadLimited(ctx, name, 1<<20)
	if indexrepair.IsSourceNotExist(e) {
		return conf.NewBoxConf(), nil, ctx.Err()
	}
	if e != nil {
		return nil, nil, e
	}
	value = conf.NewBoxConf()
	if !valid(raw, value) {
		return nil, nil, ErrPath
	}
	if strings.Contains(value.Icon, ".") {
		value.Icon = textprefs.FilterUploadEmojiFileName(value.Icon)
	}
	return value, raw, ctx.Err()
}
