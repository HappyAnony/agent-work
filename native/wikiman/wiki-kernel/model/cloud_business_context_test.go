package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/util"
)

func cloudBusinessFixture(t *testing.T) (*AppConf, string, string) {
	t.Helper()
	c, file := cloudFixture(t)
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "assets"), 0700)
	os.WriteFile(filepath.Join(dir, "assets", "fixture.png"), []byte("synthetic-upload-bytes"), 0600)
	c.CloudRegion = 1
	c.business = conf.NewBusiness(nil, map[string]map[int]string{"en_US": {31: "login required", 27: "upload %v", 94: "upload failed: %s", 247: "file %s exceeds %s"}}, nil)
	c.SetUser(&conf.User{UserId: "fixture-user", UserToken: "fixture-user-token", UserSiYuanProExpireTime: -1, UserSiYuanSubscriptionStatus: 0})
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	previous, data, workspace := Conf, util.DataDir, util.WorkspaceDir
	Conf = c
	util.DataDir = dir
	util.WorkspaceDir = dir
	t.Cleanup(func() {
		c.CloseCloudAccount(context.Background())
		Conf = previous
		util.DataDir = data
		util.WorkspaceDir = workspace
	})
	return c, file, dir
}
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
func TestCloudBusinessActualHTTPTrialDeactivateTokenAndMultipart(t *testing.T) {
	c, file, dir := cloudBusinessFixture(t)
	before, _ := os.ReadFile(file)
	var mu sync.Mutex
	counts := map[string]int{}
	var meta []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("symphony")
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/apis/siyuan/upload" {
			if cookie.Value != "fixture-upload-token" {
				t.Error("wrong upload credential")
			}
			if e := r.ParseMultipartForm(1 << 20); e != nil {
				t.Error(e)
				return
			}
			defer r.MultipartForm.RemoveAll()
			f, header, e := r.FormFile("file[]")
			if e != nil {
				t.Error(e)
				return
			}
			raw, e := io.ReadAll(f)
			f.Close()
			if e != nil || string(raw) != "synthetic-upload-bytes" || header.Filename != "fixture.png" {
				t.Error("multipart mismatch", e)
			}
			if r.URL.Query().Get("ver") != "fixture-version" {
				t.Error("version missing")
			}
			mu.Lock()
			meta = append(meta, r.Header.Get("biz-type")+":"+r.Header.Get("meta-type"))
			mu.Unlock()
			w.Write([]byte(`{"code":0}`))
			return
		}
		if cookie.Value != "fixture-user-token" {
			t.Error("wrong account credential")
		}
		raw, _ := io.ReadAll(r.Body)
		if len(raw) != 0 {
			t.Error("added body to bodyless original request")
		}
		if r.URL.Path == "/apis/siyuan/upload/token" {
			w.Write([]byte(`{"code":0,"data":{"uploadToken":"fixture-upload-token"}}`))
		} else {
			w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	s, e := c.newCloudAccount(func(ctx context.Context, region int, route string, h, b map[string]string) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("borrowed region")
		}
		return requestCloudAccountURL(ctx, server.URL+route, h, b)
	}, func(ctx context.Context, region int, token string, f cloudaccount.UploadFile, biz, mt string) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("borrowed upload region")
		}
		return requestCloudUploadURL(ctx, server.URL+"/apis/siyuan/upload?ver=fixture-version", token, f, biz, mt)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	if e = StartFreeTrialContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = DeactivateUserContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, biz := range []string{cloudaccount.UploadAssetsBusiness, cloudaccount.ExportForumBusiness} {
		n, e := c.uploadCloudAssets(context.Background(), []string{"assets/fixture.png", "assets/fixture.png"}, biz)
		if e != nil || n != 1 {
			t.Fatal(n, e)
		}
	}
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("never upload"), 0600)
	os.Symlink(outside, filepath.Join(dir, "assets", "linked"))
	for _, p := range []string{"assets/../private", "assets/linked"} {
		if _, e := c.uploadCloudAssets(context.Background(), []string{p}, cloudaccount.UploadAssetsBusiness); e == nil {
			t.Fatal("accepted unsafe asset", p)
		}
	}
	after, _ := os.ReadFile(file)
	body, _ := os.ReadFile(filepath.Join(dir, "assets/fixture.png"))
	if !bytes.Equal(before, after) || string(body) != "synthetic-upload-bytes" || c.GetUser() == nil {
		t.Fatal("cloud operations changed source/account")
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["/apis/siyuan/upload/token"] != 1 || counts["/apis/siyuan/upload"] != 2 || len(meta) != 2 || meta[0] != "upload-assets:5" || meta[1] != "export-liandi:4" {
		t.Fatal(counts, meta)
	}
	recordCloudBusinessEvidence(t, "http-wire.json", map[string]any{"requests": counts, "multipartMetadata": meta, "multipartBytesVerified": true, "accountCookieVerified": true, "uploadCookieVerified": true, "configAndAssetUnchanged": true, "remoteIsLoopback": true})
}
func TestCloudBusinessActualMultipartCancellationJoinsOwner(t *testing.T) {
	c, _, _ := cloudBusinessFixture(t)
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":0,"data":{"uploadToken":"fixture-token"}}`))
			return
		}
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	s, e := c.newCloudAccount(func(ctx context.Context, _ int, _ string, h, b map[string]string) (cloudaccount.Response, error) {
		return requestCloudAccountURL(ctx, server.URL+"/token", h, b)
	}, func(ctx context.Context, _ int, token string, f cloudaccount.UploadFile, biz, meta string) (cloudaccount.Response, error) {
		return requestCloudUploadURL(ctx, server.URL+"/upload", token, f, biz, meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	done := make(chan error, 1)
	go func() {
		_, e := c.uploadCloudAssets(context.Background(), []string{"assets/fixture.png"}, cloudaccount.UploadAssetsBusiness)
		done <- e
	}()
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = s.Close(deadline); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-deadline.Done():
		t.Fatal("file request ignored owner cancellation")
	}
}
func TestCloudBusinessTokenFailurePropagatesToOriginalAssetCaller(t *testing.T) {
	c, file, _ := cloudBusinessFixture(t)
	before, _ := os.ReadFile(file)
	uploads := 0
	s, e := c.newCloudAccount(func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error) {
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(9), "msg": "token refused"}}, nil
	}, func(context.Context, int, string, cloudaccount.UploadFile, string, string) (cloudaccount.Response, error) {
		uploads++
		return cloudaccount.Response{}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	n, e := c.uploadCloudAssets(context.Background(), []string{"assets/fixture.png"}, cloudaccount.UploadAssetsBusiness)
	if e == nil || n != 0 || uploads != 0 || !strings.Contains(e.Error(), "token refused") {
		t.Fatal(n, e, uploads)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed token changed source")
	}
	c.SetUser(nil)
	if e = StartFreeTrialContext(context.Background()); e == nil || e.Error() != "login required" {
		t.Fatal(e)
	}
	if e = DeactivateUserContext(context.Background()); e == nil || e.Error() != "login required" {
		t.Fatal(e)
	}
}
