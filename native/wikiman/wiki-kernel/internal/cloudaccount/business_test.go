package cloudaccount

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func successfulBusiness() Response {
	return Response{Status: 200, Body: map[string]any{"code": float64(0)}}
}
func tokenReply(value string) Response {
	return Response{Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"uploadToken": value}}}
}
func filePorts() UploadPorts {
	return UploadPorts{Resolve: func(ctx context.Context, p string) (string, error) { return p, ctx.Err() }, Open: func(ctx context.Context, p string) (io.ReadCloser, int64, error) {
		return io.NopCloser(strings.NewReader(p)), int64(len(p)), ctx.Err()
	}}
}
func TestCloudBusinessTrialDeactivateProtocolAndNoLocalMutation(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.Region = 1
	calls := 0
	next := successfulBusiness()
	o.Request = func(ctx context.Context, region int, route string, h, b map[string]string) (Response, error) {
		calls++
		if region != 1 || h["Cookie"] != "symphony=cached-token" || b != nil {
			t.Fatal("wire mismatch", region, route)
		}
		return next, nil
	}
	s := openService(t, *o)
	if e := s.StartFreeTrial(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := s.Deactivate(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 2 || state.User.UserId != "cached" || state.Cipher != "" {
		t.Fatal("remote operations changed local identity")
	}
	next = Response{Status: 401}
	if e := s.Deactivate(context.Background()); !errors.Is(e, ErrAuthentication) {
		t.Fatal(e)
	}
	if e := s.StartFreeTrial(context.Background()); !errors.Is(e, ErrNetwork) {
		t.Fatal(e)
	}
	next = Response{Status: 200, Body: map[string]any{"code": float64(7), "msg": "business denied"}}
	var remote *BusinessError
	if e := s.Deactivate(context.Background()); !errors.As(e, &remote) || remote.Message != "business denied" {
		t.Fatal(e)
	}
	state.User = nil
	before := calls
	if e := s.Deactivate(context.Background()); !errors.Is(e, ErrAuthentication) || calls != before {
		t.Fatal(e)
	}
}
func TestCloudBusinessTokenTTLIdentityAndRevocation(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	now := time.Unix(0, 0)
	o.Now = func() time.Time { return now }
	calls := 0
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		calls++
		return tokenReply("token"), nil
	}
	s := openService(t, *o)
	get := func(expected int) {
		t.Helper()
		if value, e := s.UploadToken(context.Background()); e != nil || value != "token" || calls != expected {
			t.Fatal(value, e, calls, expected)
		}
	}
	get(1)
	get(1)
	now = now.Add(time.Hour)
	get(1)
	now = now.Add(time.Nanosecond)
	get(2)
	now = now.Add(-time.Minute)
	get(3)
	state.User.UserToken = "rotated"
	get(4)
	state.User.UserId = "another"
	get(5)
	state.Region = 1
	get(6)
	state.User.UserSiYuanSubscriptionStatus = 1
	get(7)
	if e := s.StartFreeTrial(context.Background()); e != nil {
		t.Fatal(e)
	}
	get(9)
	if e := s.Deactivate(context.Background()); e != nil {
		t.Fatal(e)
	}
	get(11)
	if e := s.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.UploadToken(context.Background()); !errors.Is(e, ErrAuthentication) || calls != 11 {
		t.Fatal(e, calls)
	}
}
func TestCloudBusinessTokenFailureNeverUploadsOrReusesStaleValue(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	now := o.Now()
	o.Now = func() time.Time { return now }
	response := tokenReply("first")
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return response, nil
	}
	uploads := 0
	o.Upload = func(context.Context, int, string, UploadFile, string, string) (Response, error) {
		uploads++
		return successfulBusiness(), nil
	}
	s := openService(t, *o)
	if _, e := s.UploadToken(context.Background()); e != nil {
		t.Fatal(e)
	}
	previousRequest := s.options.Request
	cancelled, cancel := context.WithCancel(context.Background())
	s.options.Request = func(ctx context.Context, _ int, _ string, _, _ map[string]string) (Response, error) {
		cancel()
		return Response{}, ctx.Err()
	}
	if e := s.Deactivate(cancelled); !errors.Is(e, context.Canceled) || s.upload.valid {
		t.Fatal("uncertain deactivation retained pre-change credential", e)
	}
	s.options.Request = previousRequest
	now = now.Add(2 * time.Hour)
	for _, r := range []Response{{Status: 200, Body: map[string]any{"code": float64(9), "msg": "refused"}}, {Status: 200, Body: map[string]any{"code": float64(0), "data": nil}}, tokenReply(""), {Status: 401}, {Status: 503}, {Status: 200, Body: map[string]any{"code": "0"}}} {
		response = r
		if n, e := s.UploadAssets(context.Background(), []string{"asset"}, ExportForumBusiness, filePorts()); e == nil || n != 0 || uploads != 0 {
			t.Fatal(n, e, uploads)
		}
	}
	response = tokenReply("second")
	o.Upload = func(context.Context, int, string, UploadFile, string, string) (Response, error) {
		return Response{Status: 401}, nil
	}
	s.options.Upload = o.Upload
	if _, e := s.UploadAssets(context.Background(), []string{"asset"}, ExportForumBusiness, filePorts()); !errors.Is(e, ErrAuthentication) || s.upload.valid {
		t.Fatal("401 retained token", e)
	}
	s.options.Upload = func(context.Context, int, string, UploadFile, string, string) (Response, error) {
		t.Error("changed file was uploaded")
		return successfulBusiness(), nil
	}
	p := filePorts()
	p.Open = func(context.Context, string) (io.ReadCloser, int64, error) {
		return io.NopCloser(strings.NewReader("changed")), 2, nil
	}
	if _, e := s.UploadAssets(context.Background(), []string{"asset"}, ExportForumBusiness, p); !errors.Is(e, ErrAssetChanged) {
		t.Fatal(e)
	}
}
func TestCloudBusinessUploadOriginalDedupLimitsAndMetadata(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return tokenReply("upload-only"), nil
	}
	var files []UploadFile
	var metadata []string
	o.Upload = func(ctx context.Context, region int, token string, f UploadFile, biz, meta string) (Response, error) {
		if token != "upload-only" {
			t.Fatal("used account credential as upload token")
		}
		files = append(files, f)
		metadata = append(metadata, biz+":"+meta)
		return successfulBusiness(), nil
	}
	s := openService(t, *o)
	p := filePorts()
	p.Resolve = func(ctx context.Context, name string) (string, error) {
		if name == "missing" {
			return "", nil
		}
		if name == "alias" {
			return "ok", nil
		}
		return name, nil
	}
	p.Open = func(ctx context.Context, name string) (io.ReadCloser, int64, error) {
		if strings.HasPrefix(name, "large") {
			return io.NopCloser(bytes.NewReader(nil)), 3<<20 + 1, nil
		}
		return io.NopCloser(strings.NewReader("data")), 4, nil
	}
	oversize, end := 0, 0
	p.Event = func(ctx context.Context, e UploadEvent) error {
		if e.Kind == "oversize" {
			oversize++
		}
		if e.Kind == "end" {
			end++
		}
		return nil
	}
	n, e := s.UploadAssets(context.Background(), []string{"missing", "ok", "alias", "large1", "large2", "large3", "large4", "other"}, ExportForumBusiness, p)
	if e != nil || n != 2 || len(files) != 2 || oversize != 3 || end != 1 || metadata[0] != ExportForumBusiness+":4" {
		t.Fatal(n, e, oversize, end, metadata)
	}
	state.User.UserSiYuanProExpireTime = -1
	state.User.UserSiYuanSubscriptionStatus = 0
	p = filePorts()
	p.Open = func(context.Context, string) (io.ReadCloser, int64, error) {
		return io.NopCloser(bytes.NewReader(make([]byte, 4<<20))), 4 << 20, nil
	}
	if n, e = s.UploadAssets(context.Background(), []string{"subscriber.png"}, UploadAssetsBusiness, p); e != nil || n != 1 || metadata[len(metadata)-1] != UploadAssetsBusiness+":5" {
		t.Fatal(n, e)
	}
}
func TestCloudBusinessUploadCancellationJoinAndLogoutOrdering(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return tokenReply("opaque"), nil
	}
	entered := make(chan struct{})
	o.Upload = func(ctx context.Context, _ int, _ string, f UploadFile, _, _ string) (Response, error) {
		if f.Name == "first" {
			return successfulBusiness(), nil
		}
		close(entered)
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	s := openService(t, *o)
	type result struct {
		n int
		e error
	}
	done := make(chan result, 1)
	go func() {
		n, e := s.UploadAssets(context.Background(), []string{"first", "second"}, ExportForumBusiness, filePorts())
		done <- result{n, e}
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Logout(ctx); !errors.Is(e, context.DeadlineExceeded) || state.User == nil {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	r := <-done
	if r.n != 1 || !errors.Is(r.e, context.Canceled) {
		t.Fatal(r)
	}
	if _, e := s.UploadToken(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
func TestCloudBusinessConcurrentTokenCallsShareOnlyTheirOwner(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	calls := 0
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		calls++
		return tokenReply("owner"), nil
	}
	s := openService(t, *o)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, e := s.UploadToken(context.Background())
			if e != nil || value != "owner" {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatal(calls)
	}
	state2, o2 := fixture(t)
	state2.User = cached(o2)
	o2.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return tokenReply("other-owner"), nil
	}
	other := openService(t, *o2)
	value, e := other.UploadToken(context.Background())
	if e != nil || value != "other-owner" {
		t.Fatal(value, e)
	}
}
