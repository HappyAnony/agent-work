package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
)

func TestSettingHTTPOriginalJSONAndOwnerErrors(t *testing.T) {
	old := model.Conf
	defer func() { model.Conf = old }()
	root := t.TempDir()
	owner, e := model.NewAppConfAt(filepath.Join(root, "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	owner.AI = conf.NewAI()
	model.Conf = owner
	router := gin.New()
	router.POST("/ai", setAI)
	router.POST("/keymap", setKeymap)
	call := func(ctx context.Context, path, body string) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(w.Body.String(), e)
		}
		return out
	}
	reply := call(context.Background(), "/ai", `{"openAI":{"apiTimeout":0,"apiMaxContexts":90}}`)
	if reply["code"] != float64(0) {
		t.Fatal(reply)
	}
	value, ok := reply["data"].(map[string]any)
	if !ok || value["openAI"].(map[string]any)["apiTimeout"] != float64(5) {
		t.Fatal("response JSON shape", reply)
	}
	keymap := call(context.Background(), "/keymap", `{"data":{}}`)
	if keymap["code"] != float64(0) || keymap["data"] != nil {
		t.Fatal("keymap response changed", keymap)
	}
	if reply = call(context.Background(), "/ai", `{}`); reply["code"] != float64(-1) || owner.AI.OpenAI.APITimeout != 5 {
		t.Fatal("bad AI changed configuration", reply)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reply = call(ctx, "/ai", `{"openAI":{"apiTimeout":60}}`); reply["code"] != float64(-1) || owner.AI.OpenAI.APITimeout != 5 {
		t.Fatal("cancelled HTTP updated configuration", reply)
	}
	bad, e := model.NewAppConfAt(filepath.Join(root, "missing", "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	bad.AI = conf.NewAI()
	model.Conf = bad
	if reply = call(context.Background(), "/ai", `{"openAI":{"apiTimeout":60}}`); reply["code"] != float64(-1) || bad.AI.OpenAI.APITimeout == 60 {
		t.Fatal("save failure reported success", reply)
	}
}
