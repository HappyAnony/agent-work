// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"
)

// InboxTransport preserves the original JSON array payload for removal. It must
// honor ctx, not log content/credentials, and send removal only once (a canceled
// request may already have taken effect remotely).
type InboxTransport func(context.Context, int, string, map[string]string, map[string]any) (Response, error)

// Each read owns a fresh parser and a captured device timezone. Neither renderer
// nor remote data are shared with another request/workspace.
type InboxPresentation struct {
	Parser   *lute.Lute
	Location *time.Location
}

var inboxAudio = regexp.MustCompile("<audio.*>.*</audio>")
var inboxVideo = regexp.MustCompile("<video.*>.*</video>")
var inboxFile = regexp.MustCompile(`\[文件]\(.*\)`)

func (s *Service) inbox(ctx context.Context, route string, body map[string]any, present *InboxPresentation, detailID string) (map[string]any, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if s.options.InboxRequest == nil || present != nil && (present.Parser == nil || present.Location == nil) {
		return nil, ErrDependency
	}
	v, e := s.state(ctx)
	if e != nil {
		return nil, e
	}
	if e = authenticated(v); e != nil {
		return nil, e
	}
	cookie := (&http.Cookie{Name: "symphony", Value: v.User.UserToken}).String()
	r, e := s.options.InboxRequest(ctx, v.Region, route, map[string]string{"Cookie": cookie}, body)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e != nil {
		return nil, ErrNetwork
	}
	if r.Status == http.StatusUnauthorized {
		s.upload = uploadCache{}
	}
	if e = businessReply(r, true); e != nil {
		return nil, e
	}
	if present == nil {
		return nil, nil
	}
	// Own the full envelope before UI projection. Do not mutate a transport's
	// reusable response or return a partially projected list after an error.
	raw, e := json.Marshal(r.Body)
	if e != nil {
		return nil, ErrResponse
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		return nil, ErrResponse
	}
	data, ok := out["data"].(map[string]any)
	if !ok {
		return nil, ErrResponse
	}
	p := present.Parser
	p.RenderOptions.ProtyleMarkNetImg = false
	project := func(item map[string]any, id string, list bool) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		md, ok := item["shorthandContent"].(string)
		if !ok {
			return ErrResponse
		}
		if list {
			desc, ok := item["shorthandDesc"].(string)
			if !ok {
				return ErrResponse
			}
			desc = inboxAudio.ReplaceAllString(desc, " 语音 ")
			desc = inboxVideo.ReplaceAllString(desc, " 视频 ")
			desc = inboxFile.ReplaceAllString(desc, " 文件 ")
			item["shorthandDesc"] = strings.TrimSpace(strings.ReplaceAll(desc, "\n\n", ""))
		}
		ms, _ := strconv.ParseInt(id, 10, 64) // Preserve original invalid-ID time fallback.
		item["hCreated"] = time.Unix(ms/1000, ms%1000*int64(time.Millisecond)).In(present.Location).Format("2006-01-02 15:04")
		item["shorthandMd"] = md
		tree := parse.Parse("", []byte(md), p.ParseOptions)
		if e := ctx.Err(); e != nil {
			return e
		}
		item["shorthandContent"] = p.ProtylePreview(tree, p.RenderOptions)
		return ctx.Err()
	}
	if strings.HasPrefix(route, "/apis/siyuan/inbox/getCloudShorthand?") {
		p.SetFootnotes(true) // Detail-only behavior inherited from the original UI.
		if e = project(data, detailID, false); e != nil {
			return nil, e
		}
		return data, nil
	}
	items, ok := data["shorthands"].([]any)
	if !ok {
		return nil, ErrResponse
	}
	pagination, ok := data["pagination"].(map[string]any)
	if !ok {
		return nil, ErrResponse
	}
	for _, name := range []string{"paginationRecordCount", "paginationPageCount"} {
		n, ok := pagination[name].(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n != math.Trunc(n) {
			return nil, ErrResponse
		}
	}
	for _, item := range items {
		v, ok := item.(map[string]any)
		if !ok {
			return nil, ErrResponse
		}
		id, ok := v["oId"].(string)
		if !ok {
			return nil, ErrResponse
		}
		if e = project(v, id, true); e != nil {
			return nil, e
		}
	}
	return out, ctx.Err()
}

func (s *Service) GetCloudShorthand(ctx context.Context, id string, p InboxPresentation) (map[string]any, error) {
	return s.inbox(ctx, "/apis/siyuan/inbox/getCloudShorthand?id="+url.QueryEscape(id), nil, &p, id)
}
func (s *Service) GetCloudShorthands(ctx context.Context, page int, p InboxPresentation) (map[string]any, error) {
	return s.inbox(ctx, "/apis/siyuan/inbox/getCloudShorthands?p="+strconv.Itoa(page), nil, &p, "")
}
func (s *Service) RemoveCloudShorthands(ctx context.Context, ids []string) error {
	_, e := s.inbox(ctx, "/apis/siyuan/inbox/removeCloudShorthands", map[string]any{"ids": slices.Clone(ids)}, nil, "")
	return e
}
