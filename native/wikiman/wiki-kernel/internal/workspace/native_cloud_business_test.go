//go:build fts5

package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
)

func recordCloudBusinessEvidence(t *testing.T, name string, data any) {
	t.Helper()
	dir := os.Getenv("WIKI_CLOUD_BUSINESS_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	raw, e := json.MarshalIndent(data, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}
func businessOptions(t *testing.T, page string) cloudaccount.Options {
	t.Helper()
	state := cloudaccount.State{User: &conf.User{UserId: page, UserToken: "fixture-" + page, UserSiYuanProExpireTime: -1, UserSiYuanSubscriptionStatus: 0}}
	if page == pageB {
		state.Region = 1
	}
	return cloudaccount.Options{Load: func(context.Context) (cloudaccount.State, error) { return state, nil }, Commit: func(ctx context.Context, p cloudaccount.Change) error {
		if p.Region != nil {
			state.Region = *p.Region
		}
		if p.SetUser {
			state.User = p.User
			state.Cipher = p.Cipher
		}
		return ctx.Err()
	}, Request: func(ctx context.Context, r int, route string, h, b map[string]string) (cloudaccount.Response, error) {
		if r != state.Region || h["Cookie"] != "symphony=fixture-"+page {
			t.Error("foreign account request")
		}
		if route == "/apis/siyuan/upload/token" {
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": map[string]any{"uploadToken": "upload-" + page}}}, nil
		}
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0)}}, nil
	}, Decode: func(v string, _ bool) (u *conf.User, e error) { e = json.Unmarshal([]byte(v), &u); return }, Encode: func(u *conf.User) (string, error) { raw, e := json.Marshal(u); return string(raw), e }, Now: time.Now}
}
func TestNativeCloudBusinessActualOwnersAndFrozenUploadBytes(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", "assets", "cloud.png")
		os.MkdirAll(filepath.Dir(file), 0700)
		if e := os.WriteFile(file, []byte("synthetic "+page), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	records := map[string]map[string]any{}
	m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		request := o.Request
		requests := 0
		o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (cloudaccount.Response, error) {
			requests++
			return request(ctx, r, p, h, b)
		}
		o.Upload = func(ctx context.Context, r int, token string, f cloudaccount.UploadFile, biz, meta string) (cloudaccount.Response, error) {
			if token != "upload-"+page || biz != cloudaccount.UploadAssetsBusiness || meta != "5" || string(f.Data) != "synthetic "+page {
				t.Error("foreign asset/credential")
			}
			hash := sha256.Sum256(f.Data)
			records[page] = map[string]any{"region": r, "fileHash": hex.EncodeToString(hash[:]), "requestCount": requests, "metaType": meta}
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0)}}, nil
		}
		return o, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		if e := l.NativeCloudStartFreeTrial(context.Background()); e != nil {
			t.Fatal(e)
		}
		if e := l.NativeCloudDeactivate(context.Background()); e != nil {
			t.Fatal(e)
		}
		if token, e := l.NativeCloudUploadToken(context.Background()); e != nil || !strings.HasPrefix(token, "upload-") {
			t.Fatal(e)
		}
		if n, e := l.NativeCloudUploadAssets(context.Background(), []string{"assets/cloud.png", "assets/cloud.png"}, cloudaccount.UploadAssetsBusiness); e != nil || n != 1 {
			t.Fatal(n, e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(records) != 2 || records[pageA]["fileHash"] == records[pageB]["fileHash"] || digest(t, root) != before {
		t.Fatal("owner leak or source mutation", records)
	}
	recordCloudBusinessEvidence(t, "contexts.json", map[string]any{"owners": records, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "rawCredentialsRecorded": false})
}
func TestNativeCloudUploadReleasesSourceLockAndDrainCancelsTransport(t *testing.T) {
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", "assets", "cloud.png")
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte("frozen asset"), 0600)
	before := digest(t, root)
	entered := make(chan struct{})
	m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		o.Upload = func(ctx context.Context, _ int, _ string, _ cloudaccount.UploadFile, _, _ string) (cloudaccount.Response, error) {
			close(entered)
			<-ctx.Done()
			return cloudaccount.Response{}, ctx.Err()
		}
		return o, nil
	}})
	l := getDocLease(t, m, pageA)
	done := make(chan error, 1)
	go func() {
		_, e := l.NativeCloudUploadAssets(context.Background(), []string{"assets/cloud.png"}, cloudaccount.UploadAssetsBusiness)
		done <- e
	}()
	<-entered
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal("network retained source shared lock", ok, e)
	}
	if e = lock.Unlock(); e != nil {
		t.Fatal(e)
	}
	l.Release()
	deadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = m.Drain(deadline); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-deadline.Done():
		t.Fatal("drain did not cancel upload")
	}
	if digest(t, root) != before {
		t.Fatal("cancel changed source")
	}
	recordCloudBusinessEvidence(t, "lifecycle.json", map[string]any{"exclusiveSourceLockAvailableDuringNetwork": ok, "drainCancelledTransport": true, "tasksAfterDrain": m.Stats().Tasks, "sourceUnchanged": true})
}
func TestNativeCloudBusinessBindingsAndPathRefusal(t *testing.T) {
	m := open(t, fixture(t), Options{})
	l := getDocLease(t, m, pageA)
	if e := l.NativeCloudStartFreeTrial(context.Background()); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	if _, e := l.NativeCloudUploadToken(context.Background()); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", "assets", "cloud.png")
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte("asset"), 0600)
	calls := 0
	bound := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		request := o.Request
		o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (cloudaccount.Response, error) {
			calls++
			return request(ctx, r, p, h, b)
		}
		o.Upload = func(context.Context, int, string, cloudaccount.UploadFile, string, string) (cloudaccount.Response, error) {
			t.Error("unsafe asset uploaded")
			return cloudaccount.Response{}, nil
		}
		return o, nil
	}})
	a := getDocLease(t, bound, pageA)
	for _, name := range []string{"../outside", ".wiki/preferences.json", "assets/../cloud.png"} {
		if _, e := a.NativeCloudUploadAssets(context.Background(), []string{name}, cloudaccount.UploadAssetsBusiness); !errors.Is(e, ErrDocument) {
			t.Fatal(name, e)
		}
	}
	if _, e := a.NativeCloudUploadAssets(context.Background(), make([]string, 129), cloudaccount.UploadAssetsBusiness); !errors.Is(e, ErrBudget) {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("must not upload"), 0600)
	link := filepath.Join(root, "pages", pageA, "data", "assets", "linked")
	os.Symlink(outside, link)
	if _, e := a.NativeCloudUploadAssets(context.Background(), []string{"assets/linked"}, cloudaccount.UploadAssetsBusiness); !errors.Is(e, ErrDocument) {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal("validation contacted cloud", calls)
	}
	raw, _ := os.ReadFile(file)
	if !bytes.Equal(raw, []byte("asset")) {
		t.Fatal("validation changed asset")
	}
}
