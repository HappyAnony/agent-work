package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func TestRestoreHTTPKeepsAsyncNullAndCancelsAtAdmission(t *testing.T) {
	previous := model.Conf
	workspace, data, repo, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	os.Mkdir(util.DataDir, 0700)
	c, e := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	c.Sync = conf.NewSync()
	c.Repo = conf.NewRepo()
	c.Repo.Key = make([]byte, 32)
	c.System = conf.NewSystem()
	model.Conf = c
	task.OpenLegacyTasks()
	defer func() {
		c.CloseCloudRepo(context.Background())
		task.CloseLegacyTasks(context.Background())
		model.Conf = previous
		util.WorkspaceDir = workspace
		util.DataDir = data
		util.RepoDir = repo
		util.HistoryDir = history
		util.TempDir = temp
	}()
	r := gin.New()
	r.POST("/restore", checkoutRepo)
	call := func(ctx context.Context) map[string]any {
		raw, _ := json.Marshal(map[string]any{"id": strings.Repeat("a", 40)})
		req := httptest.NewRequest("POST", "/restore", bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal("invalid result")
		}
		return v
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if v := call(ctx); v["code"] != float64(-1) {
		t.Fatal(v)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if v := call(ctx); v["code"] != float64(0) || v["data"] != nil {
		t.Fatal("original async shape changed", v)
	}
	cancel()
	if _, e = os.Stat(util.RepoDir); !os.IsNotExist(e) {
		t.Fatal("queued operation unexpectedly executed", e)
	}
}

func TestRebuildHTTPUsesRequestContextForAdmission(t *testing.T) {
	previous := model.Conf
	workspace, data, repo, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	os.Mkdir(util.DataDir, 0700)
	c, err := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	c.Sync = conf.NewSync()
	c.Repo = conf.NewRepo()
	c.System = conf.NewSystem()
	model.Conf = c
	task.OpenLegacyTasks()
	defer func() {
		c.CloseCloudRepo(context.Background())
		task.CloseLegacyTasks(context.Background())
		model.Conf = previous
		util.WorkspaceDir = workspace
		util.DataDir = data
		util.RepoDir = repo
		util.HistoryDir = history
		util.TempDir = temp
	}()
	router := gin.New()
	router.POST("/rebuild", rebuildDataIndex)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/rebuild", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var result map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["code"] != float64(-1) || result["msg"] != context.Canceled.Error() {
		t.Fatal(result)
	}
}
