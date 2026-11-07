//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestLiandiExportActualTransportAndOwnedAttributePublication(t *testing.T) {
	m, box := markdownFixture(t)
	id := "20261105010000-forum01"
	markdownDoc(t, m, box, "/"+id+".docj")
	Conf.User = &conf.User{UserId: "synthetic", UserToken: "synthetic-token"}
	file := filepath.Join(util.DataDir, box, id+".docj")
	before, _ := os.ReadFile(file)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cookie, e := r.Cookie("symphony")
		if e != nil || cookie.Value != "synthetic-token" {
			t.Error("missing captured account")
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || !bytes.Contains(raw, []byte("Prepared content")) {
			t.Errorf("unexpected request %s %s", r.Method, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"code":0,"msg":"","data":"synthetic-article"}`)
	}))
	defer server.Close()
	ports := liandiPorts{upload: func(context.Context, []string) error { return nil }, request: func(c context.Context, method, address, token string, body map[string]interface{}) (liandiReply, error) {
		return requestLiandiArticle(c, method, server.URL, token, body)
	}}
	if e := export2LiandiContextWithPorts(context.Background(), id, ports); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if bytes.Equal(before, after) || !bytes.Contains(after, []byte("synthetic-article")) || calls.Load() != 1 {
		t.Fatal("article receipt not persisted", string(after), calls.Load())
	}
	if e := Conf.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestLiandiExportTransportFailureAndCloseDrain(t *testing.T) {
	for _, mode := range []string{"failure", "close"} {
		t.Run(mode, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := "20261105010000-forum02"
			markdownDoc(t, m, box, "/"+id+".docj")
			Conf.User = &conf.User{UserId: "synthetic", UserToken: "synthetic-token"}
			file := filepath.Join(util.DataDir, box, id+".docj")
			before, _ := os.ReadFile(file)
			failure := errors.New("transport_failed")
			started, release := make(chan struct{}), make(chan struct{})
			ports := liandiPorts{upload: func(context.Context, []string) error { return nil }, request: func(c context.Context, _, _, _ string, _ map[string]interface{}) (liandiReply, error) {
				if mode == "failure" {
					return liandiReply{}, failure
				}
				close(started)
				<-release
				return liandiReply{}, c.Err()
			}}
			if mode == "failure" {
				if e := export2LiandiContextWithPorts(context.Background(), id, ports); !errors.Is(e, failure) {
					t.Fatal("network error lost", e)
				}
			} else {
				done := make(chan error, 1)
				owner := Conf
				go func() { done <- export2LiandiContextWithPorts(context.Background(), id, ports) }()
				<-started
				c, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
				defer cancel()
				if e := owner.CloseCloudRepo(c); !errors.Is(e, context.DeadlineExceeded) {
					close(release)
					t.Fatal("close detached request", e)
				}
				close(release)
				if e := <-done; !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("failed transport changed source")
			}
			if e := Conf.CloseCloudRepo(context.Background()); e != nil {
				t.Fatal(e)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := Export2LiandiContext(ctx, "missing"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
