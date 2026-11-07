// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"bytes"
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HistorySource struct {
	Raw               []byte
	Selected, Missing bool
}
type HistorySourcePorts struct {
	Directories func(context.Context) ([]string, error)
	History     func(context.Context, string) ([]byte, error)
	Current     func(context.Context, string) ([]byte, error)
	Missing     func(error) bool
	Location    *time.Location
}

// HistoryAttributeViewSource preserves first-prefix selection and the original
// current-data fallback only for an explicitly missing history/current file.
func HistoryAttributeViewSource(ctx context.Context, created, id string, p HistorySourcePorts) (out HistorySource, err error) {
	defer func() {
		if err != nil {
			out = HistorySource{}
		}
	}()
	if ctx == nil || p.Directories == nil || p.History == nil || p.Current == nil || p.Missing == nil || p.Location == nil {
		return out, ErrShape
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	stamp, e := strconv.ParseInt(created, 10, 64)
	if e != nil {
		return out, e
	}
	prefix := time.Unix(stamp, 0).In(p.Location).Format("2006-01-02-150405")
	dirs, e := p.Directories(ctx)
	if e != nil {
		return out, e
	}
	dirs = append([]string(nil), dirs...)
	sort.Strings(dirs)
	chosen := ""
	for _, name := range dirs {
		if strings.HasPrefix(name, prefix) {
			chosen = name
			break
		}
	}
	if chosen == "" {
		return out, ctx.Err()
	}
	raw, e := p.History(ctx, chosen+"/storage/av/"+id+".json")
	if e != nil {
		if !p.Missing(e) {
			return out, e
		}
		raw, e = p.Current(ctx, id)
		if e != nil {
			if !p.Missing(e) {
				return out, e
			}
			return HistorySource{Selected: true, Missing: true}, ctx.Err()
		}
	}
	return HistorySource{Raw: bytes.Clone(raw), Selected: true}, ctx.Err()
}
