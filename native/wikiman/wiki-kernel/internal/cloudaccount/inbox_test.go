package cloudaccount

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/nativeparser"
)

func inboxPresentation() InboxPresentation {
	return InboxPresentation{Parser: nativeparser.NewEditor(nativeparser.EditorOptions{Markdown: nativeparser.Defaults()}), Location: time.FixedZone("fixture", 8*3600)}
}
func inboxItem() map[string]any {
	return map[string]any{"oId": "1760000000000", "shorthandTitle": "fixture title", "shorthandURL": "https://example.invalid/", "shorthandDesc": "  <audio src='a'>x</audio>\n\n<video>x</video> [文件](a.txt)  ", "shorthandContent": "**fixture**\n\n![pic](https://example.invalid/a.png)\n\nfoot[^n]\n\n[^n]: note"}
}
func inboxList() Response {
	return Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "ok", "extra": "preserved", "data": map[string]any{"shorthands": []any{inboxItem()}, "pagination": map[string]any{"paginationRecordCount": float64(19), "paginationPageCount": float64(2)}}}}
}

func TestInboxOriginalProtocolProjectionCopiesAndPagination(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.Region = 1
	next := inboxList()
	route := "/apis/siyuan/inbox/getCloudShorthands?p=2"
	calls := 0
	o.Commit = func(context.Context, Change) error { t.Fatal("inbox saved local account"); return nil }
	o.InboxRequest = func(ctx context.Context, r int, p string, h map[string]string, b map[string]any) (Response, error) {
		calls++
		if r != 1 || p != route || h["Cookie"] != "symphony=cached-token" || b != nil {
			t.Fatal("original wire changed")
		}
		return next, nil
	}
	s := openService(t, *o)
	before, _ := json.Marshal(next.Body)
	v, e := s.GetCloudShorthands(context.Background(), 2, inboxPresentation())
	if e != nil {
		t.Fatal(e)
	}
	item := v["data"].(map[string]any)["shorthands"].([]any)[0].(map[string]any)
	if item["shorthandMd"] != inboxItem()["shorthandContent"] || !strings.Contains(item["shorthandContent"].(string), "<strong>fixture</strong>") || item["shorthandDesc"] != "语音  视频   文件" || item["hCreated"] != time.UnixMilli(1760000000000).In(inboxPresentation().Location).Format("2006-01-02 15:04") || v["extra"] != "preserved" {
		t.Fatal(item, v)
	}
	if !reflect.DeepEqual(v["data"].(map[string]any)["pagination"], next.Body["data"].(map[string]any)["pagination"]) {
		t.Fatal("pagination changed")
	}
	after, _ := json.Marshal(next.Body)
	if string(after) != string(before) {
		t.Fatal("transport response mutated")
	}
	id := "1760000000000&other=x"
	route = "/apis/siyuan/inbox/getCloudShorthand?id=" + url.QueryEscape(id)
	next = Response{Status: 200, Body: map[string]any{"code": float64(0), "data": inboxItem()}}
	v, e = s.GetCloudShorthand(context.Background(), id, inboxPresentation())
	if e != nil || v["shorthandMd"] != inboxItem()["shorthandContent"] || !strings.Contains(v["shorthandContent"].(string), "footnotes") {
		t.Fatal(v, e)
	}
	if v["shorthandDesc"] != inboxItem()["shorthandDesc"] {
		t.Fatal("detail modified description")
	}
	next = inboxList()
	next.Body["data"].(map[string]any)["shorthands"] = []any{}
	route = "/apis/siyuan/inbox/getCloudShorthands?p=1"
	v, e = s.GetCloudShorthands(context.Background(), 1, inboxPresentation())
	if e != nil || len(v["data"].(map[string]any)["shorthands"].([]any)) != 0 || calls != 3 || state.User.UserId != "cached" {
		t.Fatal(v, e, calls)
	}
}

func TestInboxRejectsBadResponsesAndUnboundOrUnauthenticatedCalls(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	next := inboxList()
	calls := 0
	o.InboxRequest = func(context.Context, int, string, map[string]string, map[string]any) (Response, error) {
		calls++
		return next, nil
	}
	s := openService(t, *o)
	for _, r := range []Response{{Status: 503}, {Status: 200}, {Status: 200, Body: map[string]any{"code": "0"}}, {Status: 200, Body: map[string]any{"code": float64(0), "data": nil}}, {Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"shorthands": []any{nil}, "pagination": map[string]any{}}}}} {
		next = r
		if v, e := s.GetCloudShorthands(context.Background(), 1, inboxPresentation()); e == nil || v != nil {
			t.Fatal("accepted bad list", v, e)
		}
	}
	for _, field := range []string{"oId", "shorthandDesc", "shorthandContent"} {
		next = inboxList()
		next.Body["data"].(map[string]any)["shorthands"].([]any)[0].(map[string]any)[field] = nil
		if v, e := s.GetCloudShorthands(context.Background(), 1, inboxPresentation()); e == nil || v != nil {
			t.Fatal(field, v, e)
		}
	}
	next = Response{Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"shorthandContent": 123}}}
	if _, e := s.GetCloudShorthand(context.Background(), "id", inboxPresentation()); !errors.Is(e, ErrResponse) {
		t.Fatal(e)
	}
	next = Response{Status: 401}
	s.upload.valid = true
	if e := s.RemoveCloudShorthands(context.Background(), []string{"id"}); !errors.Is(e, ErrAuthentication) || s.upload.valid {
		t.Fatal(e)
	}
	next = Response{Status: 200, Body: map[string]any{"code": float64(7), "msg": "fixture refused"}}
	var business *BusinessError
	if e := s.RemoveCloudShorthands(context.Background(), []string{"id"}); !errors.As(e, &business) || business.Message != "fixture refused" {
		t.Fatal(e)
	}
	before := calls
	if _, e := s.GetCloudShorthand(context.Background(), "id", InboxPresentation{}); !errors.Is(e, ErrDependency) || calls != before {
		t.Fatal(e)
	}
	// Preserve nil vs empty JSON arrays for direct legacy callers; the HTTP
	// controller historically builds a nil slice for an empty input array.
	prior := s.options.InboxRequest
	for _, ids := range [][]string{nil, {}} {
		s.options.InboxRequest = func(_ context.Context, _ int, _ string, _ map[string]string, b map[string]any) (Response, error) {
			if !reflect.DeepEqual(b["ids"], ids) {
				t.Error("nil/empty payload changed")
			}
			return successfulBusiness(), nil
		}
		if e := s.RemoveCloudShorthands(context.Background(), ids); e != nil {
			t.Fatal(e)
		}
	}
	s.options.InboxRequest = prior
	state.User = nil
	if _, e := s.GetCloudShorthand(context.Background(), "id", inboxPresentation()); !errors.Is(e, ErrAuthentication) || calls != before {
		t.Fatal(e)
	}
	s.options.InboxRequest = nil
	if e := s.RemoveCloudShorthands(context.Background(), nil); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}

func TestInboxRemovalArrayAndCancellationSerializeWithLogout(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	entered, exited := make(chan struct{}), make(chan struct{})
	ids := []string{"second", "first", "second"}
	o.InboxRequest = func(ctx context.Context, _ int, route string, _ map[string]string, body map[string]any) (Response, error) {
		if route != "/apis/siyuan/inbox/removeCloudShorthands" || len(body) != 1 || !reflect.DeepEqual(body["ids"], ids) {
			t.Error("removal array/order changed")
		}
		close(entered)
		<-ctx.Done()
		close(exited)
		return Response{}, errors.New("sensitive transport detail")
	}
	s := openService(t, *o)
	done := make(chan error, 1)
	go func() { done <- s.RemoveCloudShorthands(context.Background(), ids) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("removal did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.Logout(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("logout interleaved", e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("close did not join")
	}
	if e := <-done; !errors.Is(e, context.Canceled) || state.User == nil {
		t.Fatal(e)
	}
}
