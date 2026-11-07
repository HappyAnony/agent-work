package cloudaccount

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func cloudAIReply(reason any) Response {
	return Response{Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": " raw reply "}, "finish_reason": reason}}}}}
}
func TestCloudAIOriginalMessagesFirstChoiceAndMalformedReplies(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.Region = 1
	next := cloudAIReply("length")
	calls := 0
	o.AIRequest = func(ctx context.Context, r int, route string, h map[string]string, b map[string]any) (Response, error) {
		calls++
		if r != 1 || route != "/apis/siyuan/ai/chatGPT" || h["Cookie"] != "symphony=cached-token" || !reflect.DeepEqual(b["messages"], []map[string]any{{"role": "user", "content": "previous"}, {"role": "user", "content": ""}, {"role": "user", "content": "prompt"}}) {
			t.Fatal("cloud protocol changed")
		}
		return next, ctx.Err()
	}
	s := openService(t, *o)
	p, e := s.CloudChatGPT(context.Background(), "prompt", []string{"previous", ""})
	if e != nil || p.Text != " raw reply " || p.Stop {
		t.Fatal(p, e)
	}
	next = cloudAIReply(nil)
	p, e = s.CloudChatGPT(context.Background(), "prompt", []string{"previous", ""})
	if e != nil || !p.Stop {
		t.Fatal(p, e)
	}
	for _, r := range []Response{{Status: 503}, {Status: 401}, {Status: 200}, {Status: 200, Body: map[string]any{"code": "0"}}, cloudAIReply(123), {Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"choices": "bad"}}}} {
		next = r
		if _, e := s.CloudChatGPT(context.Background(), "prompt", []string{"previous", ""}); e == nil {
			t.Fatal("malformed cloud reply accepted")
		}
	}
	state.User = nil
	before := calls
	p, e = s.CloudChatGPT(context.Background(), "prompt", nil)
	if e != nil || p.Stop || p.Text != "" || calls != before {
		t.Fatal("nil-user no-op changed", p, e)
	}
}
func TestCloudAICloseCancelsAndSerializesAccountSwitch(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	entered, exited := make(chan struct{}), make(chan struct{})
	o.AIRequest = func(c context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (Response, error) {
		close(entered)
		<-c.Done()
		close(exited)
		return Response{}, c.Err()
	}
	s := openService(t, *o)
	done := make(chan error, 1)
	go func() { _, e := s.CloudChatGPT(context.Background(), "fixture", nil); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cloud AI not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.Logout(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("account switch interleaved", e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("not joined")
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
