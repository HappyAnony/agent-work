package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	du "github.com/siyuan-note/dejavu/util"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func TestOriginalSnapshotHTTPActualOpsAndMalformedArguments(t *testing.T) {
	previous := model.Conf
	workspace, data, repo, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	os.MkdirAll(util.DataDir, 0700)
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
	c.User = &conf.User{UserSiYuanProExpireTime: -1}
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
	store, e := dejavu.NewStore(util.RepoDir, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	blob := []byte("synthetic API bytes")
	chunk := &entity.Chunk{ID: du.Hash(blob), Data: blob}
	f := entity.NewFile("source.docj", int64(len(blob)), 1000)
	f.Chunks = []string{chunk.ID}
	id := strings.Repeat("a", 40)
	store.PutChunk(chunk)
	store.PutFile(f)
	store.PutIndex(&entity.Index{ID: id, Files: []string{f.ID}, Size: int64(len(blob)), Created: 1000})
	r := gin.New()
	r.POST("/tag", tagSnapshot)
	r.POST("/untag", removeRepoTagSnapshot)
	r.POST("/upload", uploadCloudSnapshot)
	r.POST("/download", downloadCloudSnapshot)
	r.POST("/cloud-untag", removeCloudRepoTagSnapshot)
	r.POST("/purge-local", purgeRepo)
	r.POST("/purge-cloud", purgeCloudRepo)
	call := func(route string, b map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(b)
		req := httptest.NewRequest("POST", route, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal(w.Body.String())
		}
		return v
	}
	for _, route := range []string{"/tag", "/untag", "/upload", "/download", "/cloud-untag"} {
		if v := call(route, map[string]any{"id": nil, "tag": 123, "name": nil}); v["code"] != float64(-1) {
			t.Fatal(route, v)
		}
	}
	for _, step := range []struct {
		route string
		body  map[string]any
	}{{"/tag", map[string]any{"id": id, "name": "backup"}}, {"/upload", map[string]any{"id": id, "tag": "backup"}}, {"/download", map[string]any{"id": id, "tag": "restored"}}, {"/untag", map[string]any{"tag": "backup"}}, {"/cloud-untag", map[string]any{"tag": "backup"}}, {"/purge-cloud", nil}, {"/purge-local", nil}} {
		v := call(step.route, step.body)
		if v["code"] != float64(0) || v["data"] != nil {
			t.Fatal(step.route, v)
		}
	}
	if v := call("/tag", map[string]any{"id": id, "name": "../outside"}); v["code"] != float64(-1) || v["data"].(map[string]any)["closeTimeout"] != float64(5000) {
		t.Fatal(v)
	}
}
