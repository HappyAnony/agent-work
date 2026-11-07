package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

func TestCloudRepoMetadataActualS3SelectionPersistence(t *testing.T) {
	c, file, p := repoMetadataFixture(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=synthetic-key/") {
			t.Error("SDK did not use owner credentials")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Buckets><Bucket><Name>fixture</Name><CreationDate>2026-10-28T00:00:00Z</CreationDate></Bucket></Buckets></ListAllMyBucketsResult>`))
	}))
	defer server.Close()
	c.Sync.Provider = conf.ProviderS3
	c.Sync.CloudName = "old"
	c.Sync.S3 = &conf.S3{Endpoint: server.URL, AccessKey: "synthetic-key", SecretKey: "synthetic-secret", Region: "us-east-1", Bucket: "fixture", Timeout: 30}
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	s, e := c.newCloudRepoService(func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
		return c.executeLegacyMetadata(ctx, v, r, p)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	out, e := s.Query(context.Background(), cloudrepo.Request{Kind: "repos"})
	if e != nil {
		t.Fatal(e)
	}
	var d struct {
		Checked string `json:"checkedSyncDir"`
	}
	json.Unmarshal(out.Data, &d)
	raw, _ := os.ReadFile(file)
	var cfg conf.AppData
	json.Unmarshal(raw, &cfg)
	if d.Checked != "fixture" || c.Sync.CloudName != "fixture" || cfg.Sync.CloudName != "fixture" || requests != 1 {
		t.Fatal("original S3 selection not persisted", d.Checked, c.Sync.CloudName, requests)
	}
	recordCloudBusinessEvidence(t, "cloud-repo-s3-selection.json", map[string]any{"actualSDKRequests": requests, "ownerCredentialsUsed": true, "selectedDirectorySaved": true})
}

func TestCloudRepoMetadataActualThirdPartyCancellation(t *testing.T) {
	for _, provider := range []int{conf.ProviderS3, conf.ProviderWebDAV} {
		t.Run(conf.ProviderToStr(provider), func(t *testing.T) {
			c, _, p := repoMetadataFixture(t)
			started := make(chan struct{}, 10)
			gone := make(chan struct{}, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				started <- struct{}{}
				<-r.Context().Done()
				gone <- struct{}{}
			}))
			defer server.Close()
			c.Sync.Provider = provider
			c.Sync.S3 = &conf.S3{Endpoint: server.URL, AccessKey: "synthetic-key", SecretKey: "synthetic-secret", Region: "us-east-1", Bucket: "fixture", Timeout: 30}
			c.Sync.WebDAV = &conf.WebDAV{Endpoint: server.URL, Username: "synthetic", Password: "synthetic", Timeout: 30}
			s, e := c.newCloudRepoService(func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
				return executeMetadata(ctx, v, r, p)
			})
			if e != nil {
				t.Fatal(e)
			}
			c.repoOwner = s
			done := make(chan error, 1)
			go func() { _, e := s.Query(context.Background(), cloudrepo.Request{Kind: "repos"}); done <- e }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("provider did not start actual HTTP")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if e = s.Close(ctx); e != nil {
				t.Fatal(e)
			}
			if e = <-done; e == nil {
				t.Fatal("cancellation lost")
			}
			select {
			case <-gone:
			case <-ctx.Done():
				t.Fatal("actual HTTP not canceled")
			}
			recordCloudBusinessEvidence(t, "cloud-repo-transport-"+conf.ProviderToStr(provider)+".json", map[string]any{"actualHTTPCanceled": true, "closeJoined": true, "explicitCredentials": true})
		})
	}
}
