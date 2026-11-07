package api

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestOriginalRepoKeyHTTPShapesCancellationAndReset(t *testing.T) {
	previous := model.Conf
	workspace, data, repo, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	root := t.TempDir()
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.RepoDir = filepath.Join(root, "repo")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	os.MkdirAll(util.DataDir, 0700)
	source := filepath.Join(util.DataDir, "source.docj")
	os.WriteFile(source, []byte("synthetic repo key API source"), 0600)
	c, e := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	c.Sync = conf.NewSync()
	c.Sync.Enabled = true
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: t.TempDir(), ConcurrentReqs: 2, Timeout: 30}
	c.Repo = conf.NewRepo()
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
	if e = c.SaveError(); e != nil {
		t.Fatal(e)
	}
	r := gin.New()
	r.POST("/import", importRepoKey)
	r.POST("/pass", initRepoKeyFromPassphrase)
	r.POST("/random", initRepoKey)
	r.POST("/reset", resetRepo)
	call := func(ctx context.Context, route string, body map[string]any) map[string]any {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", route, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal("invalid HTTP response")
		}
		return v
	}
	for _, route := range []string{"/import", "/pass"} {
		for _, input := range []any{nil, 123, []any{}} {
			body := map[string]any{"key": input, "pass": input}
			if v := call(context.Background(), route, body); v["code"] != float64(-1) {
				t.Fatal("malformed input accepted", route)
			}
		}
	}
	if v := call(context.Background(), "/import", map[string]any{"key": "invalid"}); v["code"] != float64(-1) || v["data"].(map[string]any)["closeTimeout"] != float64(5000) {
		t.Fatal("original import error shape changed")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if v := call(canceled, "/random", nil); v["code"] != float64(-1) {
		t.Fatal("canceled initialization accepted")
	}
	if _, e = os.Stat(util.RepoDir); !os.IsNotExist(e) {
		t.Fatal("canceled request replaced cache", e)
	}
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	for _, step := range []struct {
		route string
		body  map[string]any
	}{{"/import", map[string]any{"key": " \u200b" + encoded + " "}}, {"/pass", map[string]any{"pass": encoded}}, {"/random", nil}} {
		v := call(context.Background(), step.route, step.body)
		if v["code"] != float64(0) {
			t.Fatal("original initialization failed", step.route)
		}
		d, ok := v["data"].(map[string]any)
		if !ok {
			t.Fatal("key result shape changed")
		}
		raw, ok := d["key"].(string)
		if !ok {
			t.Fatal("key not base64 JSON string")
		}
		key, e := base64.StdEncoding.DecodeString(raw)
		if e != nil || len(key) != 32 {
			t.Fatal("invalid returned key")
		}
		if step.route != "/random" && raw != encoded {
			t.Fatal("import/pass normalization mismatch")
		}
		if _, e = os.Stat(filepath.Join(util.RepoDir, "refs", "latest")); e != nil {
			t.Fatal("initial snapshot missing", e)
		}
	}
	if v := call(context.Background(), "/reset", nil); v["code"] != float64(0) || v["data"] != nil {
		t.Fatal("reset success null shape changed")
	}
	if len(c.Repo.Key) != 0 || c.Sync.Enabled {
		t.Fatal("reset configuration not applied")
	}
	if b, e := os.ReadFile(source); e != nil || string(b) != "synthetic repo key API source" {
		t.Fatal("source changed", e)
	}
	if dir := os.Getenv("WIKI_CLOUD_BUSINESS_EVIDENCE_DIR"); dir != "" {
		os.MkdirAll(dir, 0700)
		raw, _ := json.MarshalIndent(map[string]any{"originalEndpoints": 4, "malformedArgumentsRejected": 6, "canceledRequestNoMutation": true, "base64KeyShapesPreserved": true, "initialSnapshotsCreated": 3, "resetNullAndDisabledSync": true, "sourceUnchanged": true}, "", "  ")
		if e = os.WriteFile(filepath.Join(dir, "repokey-api.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
