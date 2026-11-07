// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"net/http"
	"wiki-kernel/internal/aiflow"
)

type AITransport func(context.Context, int, string, map[string]string, map[string]any) (Response, error)

func (s *Service) CloudChatGPT(ctx context.Context, msg string, history []string) (aiflow.Part, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return aiflow.Part{Stop: true}, e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return aiflow.Part{Stop: true}, e
	}
	if v.User == nil {
		return aiflow.Part{}, nil
	} // Original low-level nil-account no-op.
	if s.options.AIRequest == nil {
		return aiflow.Part{Stop: true}, ErrDependency
	}
	if e = authenticated(v); e != nil {
		return aiflow.Part{Stop: true}, e
	}
	messages := make([]map[string]any, 0, len(history)+1)
	for _, v := range history {
		messages = append(messages, map[string]any{"role": "user", "content": v})
	}
	messages = append(messages, map[string]any{"role": "user", "content": msg})
	cookie := (&http.Cookie{Name: "symphony", Value: v.User.UserToken}).String()
	r, e := s.options.AIRequest(ctx, v.Region, "/apis/siyuan/ai/chatGPT", map[string]string{"Cookie": cookie}, map[string]any{"messages": messages})
	if ctx.Err() != nil {
		return aiflow.Part{Stop: true}, ctx.Err()
	}
	if e != nil {
		return aiflow.Part{Stop: true}, ErrNetwork
	}
	if r.Status == 401 {
		s.upload = uploadCache{}
	}
	if e = businessReply(r, true); e != nil {
		return aiflow.Part{Stop: true}, e
	}
	data, ok := r.Body["data"].(map[string]any)
	if !ok {
		return aiflow.Part{Stop: true}, ErrResponse
	}
	choices, ok := data["choices"].([]any)
	if !ok {
		return aiflow.Part{Stop: true}, ErrResponse
	}
	if len(choices) == 0 {
		return aiflow.Part{Stop: true}, nil
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return aiflow.Part{Stop: true}, ErrResponse
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return aiflow.Part{Stop: true}, ErrResponse
	}
	text, ok := message["content"].(string)
	if !ok {
		return aiflow.Part{Stop: true}, ErrResponse
	}
	stop := true
	if reason := choice["finish_reason"]; reason != nil {
		v, ok := reason.(string)
		if !ok {
			return aiflow.Part{Stop: true}, ErrResponse
		}
		stop = v != "length"
	}
	return aiflow.Part{Text: text, Stop: stop}, nil
}
