package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func TestCreateSnapshotOriginalHTTPContextAndArguments(t *testing.T) {
	previous := model.Conf
	workspace, data, repo, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	os.MkdirAll(util.DataDir, 0700)
	os.WriteFile(filepath.Join(util.DataDir, "source.docj"), []byte("synthetic API source"), 0600)
	c, e := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	c.Sync = conf.NewSync()
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: t.TempDir(), ConcurrentReqs: 4, Timeout: 30}
	c.Repo = conf.NewRepo()
	c.Repo.Key = bytes.Repeat([]byte{7}, 32)
	c.System = conf.NewSystem()
	model.Conf = c
	defer func() {
		c.CloseCloudRepo(context.Background())
		model.Conf = previous
		util.WorkspaceDir = workspace
		util.DataDir = data
		util.RepoDir = repo
		util.HistoryDir = history
		util.TempDir = temp
	}()
	r := gin.New()
	r.POST("/create", createSnapshot)
	call := func(ctx context.Context, body string) map[string]any {
		req := httptest.NewRequest("POST", "/create", bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(w.Body.String())
		}
		return v
	}
	for _, b := range []string{`{}`, `{"memo":2}`, `{"memo":null}`, `{"memo":" "}`} {
		if v := call(context.Background(), b); v["code"] != float64(-1) {
			t.Fatal(v)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if v := call(canceled, `{"memo":"canceled"}`); v["code"] != float64(-1) {
		t.Fatal(v)
	}
	if _, e = os.Stat(filepath.Join(util.RepoDir, "refs", "latest")); !os.IsNotExist(e) {
		t.Fatal("canceled index published", e)
	}
	if v := call(context.Background(), `{"memo":"synthetic"}`); v["code"] != float64(0) || v["data"] != nil {
		t.Fatal(v)
	}
	if _, e = os.Stat(filepath.Join(util.RepoDir, "refs", "latest")); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(filepath.Join(util.DataDir, "source.docj")); e != nil || string(b) != "synthetic API source" {
		t.Fatal("source changed", e)
	}
}
