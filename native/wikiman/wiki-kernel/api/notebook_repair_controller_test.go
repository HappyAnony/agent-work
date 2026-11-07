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

func TestOriginalNotebookHTTPReadonlyAndCancelledRepair(t *testing.T) {
	old, oldData := model.Conf, util.DataDir
	dir := t.TempDir()
	util.DataDir = dir
	owner, e := model.NewAppConfAt(filepath.Join(t.TempDir(), "conf.json"), true)
	if e != nil {
		t.Fatal(e)
	}
	model.Conf = owner
	defer func() { model.Conf = old; util.DataDir = oldData }()
	owner.FileTree = conf.NewFileTree()
	id := "20261027101000-miss001"
	os.Mkdir(filepath.Join(dir, id), 0700)
	router := gin.New()
	router.POST("/list", lsNotebooks)
	call := func(ctx context.Context) map[string]any {
		request := httptest.NewRequest("POST", "/list", bytes.NewBufferString(`{}`)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	v := call(context.Background())
	if v["code"] != float64(0) || len(v["data"].(map[string]any)["notebooks"].([]any)) != 1 {
		t.Fatal(v)
	}
	if _, e = os.Stat(filepath.Join(dir, id, ".siyuan")); !os.IsNotExist(e) {
		t.Fatal("readonly route repaired source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v = call(ctx)
	if v["code"] != float64(-1) {
		t.Fatal("cancelled route reported success", v)
	}
}
