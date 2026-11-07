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

func TestOriginalCloudDirectoryHTTPShapesPersistenceAndInvalidArguments(t *testing.T) {
	previous := model.Conf
	data, repo, history, temp := util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.DataDir = filepath.Join(root, "data")
	if e := os.MkdirAll(util.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	c, e := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	c.Sync = conf.NewSync()
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: filepath.Join(root, "remote")}
	c.System = conf.NewSystem()
	c.Repo = conf.NewRepo()
	model.Conf = c
	defer func() {
		c.CloseCloudRepo(context.Background())
		model.Conf = previous
		util.DataDir = data
		util.RepoDir = repo
		util.HistoryDir = history
		util.TempDir = temp
	}()
	r := gin.New()
	r.POST("/create", createCloudSyncDir)
	r.POST("/remove", removeCloudSyncDir)
	r.POST("/set", setCloudSyncDir)
	call := func(route, body string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", route, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal(w.Body.String())
		}
		return v
	}
	for _, route := range []string{"/create", "/remove", "/set"} {
		for _, body := range []string{`{}`, `{"name":null}`, `{"name":123}`} {
			if v := call(route, body); v["code"] != float64(-1) {
				t.Fatal(v)
			}
		}
	}
	if v := call("/create", `{"name":"new"}`); v["code"] != float64(0) || v["data"] != nil {
		t.Fatal(v)
	}
	if v := call("/set", `{"name":"new"}`); v["code"] != float64(0) || v["data"] != nil || c.Sync.CloudName != "new" {
		t.Fatal(v)
	}
	if v := call("/remove", `{"name":"new"}`); v["code"] != float64(0) || v["data"] != "main" {
		t.Fatal(v)
	}
	for _, route := range []string{"/create", "/remove"} {
		if v := call(route, `{"name":"../outside"}`); v["code"] != float64(-1) || v["data"].(map[string]any)["closeTimeout"] != float64(5000) {
			t.Fatal(v)
		}
	}
	// Original set keeps its successful/null envelope for a bad directory name
	// and reports the validation error through its notification path.
	if v := call("/set", `{"name":"bad name"}`); v["code"] != float64(0) || v["data"] != nil || c.Sync.CloudName != "main" {
		t.Fatal(v)
	}
	if _, e = os.Stat(filepath.Join(c.Sync.Local.Endpoint, "new")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
}
