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

func TestOriginalCloudRepoHTTPObjectEnvelopeAndAdmission(t *testing.T) {
	prior := model.Conf
	data, repo, history, temp := util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.DataDir = root
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	c := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {26: "repo key missing"}}, nil))
	c.Lang = "en_US"
	c.Sync = conf.NewSync()
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: filepath.Join(root, "remote")}
	c.Repo = conf.NewRepo()
	c.System = conf.NewSystem()
	model.Conf = c
	defer func() {
		c.CloseCloudRepo(context.Background())
		model.Conf = prior
		util.DataDir = data
		util.RepoDir = repo
		util.HistoryDir = history
		util.TempDir = temp
	}()
	if e := os.MkdirAll(filepath.Join(c.Sync.Local.Endpoint, "main"), 0700); e != nil {
		t.Fatal(e)
	}
	r := gin.New()
	r.POST("/dirs", listCloudSyncDir)
	r.POST("/logs", getCloudRepoSnapshots)
	r.POST("/tags", getCloudRepoTagSnapshots)
	r.POST("/space", getCloudSpace)
	call := func(route, body string, ctx context.Context) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", route, bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal(w.Body.String())
		}
		return v
	}
	v := call("/dirs", "{}", context.Background())
	d, ok := v["data"].(map[string]any)
	if v["code"] != float64(0) || !ok || d["checkedSyncDir"] != "main" || d["hSize"] != "-" {
		t.Fatal("object envelope replaced by JSON string", v)
	}
	dirs, ok := d["syncDirs"].([]any)
	if !ok || len(dirs) != 1 {
		t.Fatal(v)
	}
	for route, body := range map[string]string{"/logs": `{"page":1}`, "/tags": `{}`} {
		v = call(route, body, context.Background())
		if v["code"] != float64(-1) || v["msg"] != "repo key missing" || v["data"] != nil {
			t.Fatal(v)
		}
	}
	for _, body := range []string{`{}`, `{"page":null}`, `{"page":"1"}`, `{"page":1.5}`, `{"page":1000001}`} {
		v = call("/logs", body, context.Background())
		if v["code"] != float64(-1) {
			t.Fatal(v)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, route := range []string{"/space", "/dirs"} {
		v = call(route, "{}", ctx)
		if v["code"] != float64(1) {
			t.Fatal("cancellation lost", v)
		}
	}
}
