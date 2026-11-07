package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
)

func TestRemainingSettingHTTPEmptyRefreshAndDeferredEmojiSave(t *testing.T) {
	old := model.Conf
	defer func() { model.Conf = old }()
	file := filepath.Join(t.TempDir(), "conf.json")
	owner, e := model.NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	owner.Editor = conf.NewEditor()
	owner.Editor.VirtualBlockRef = false
	owner.Export = conf.NewExport()
	model.Conf = owner
	if e = owner.SaveError(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	router.POST("/refresh", refreshVirtualBlockRef)
	router.POST("/emoji", setEmoji)
	router.POST("/include", addVirtualBlockRefInclude)
	router.POST("/export", setExport)
	call := func(path, body string) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(w.Body.String(), e)
		}
		return out
	}
	if r := call("/refresh", ""); r["code"] != float64(0) || r["data"] != nil {
		t.Fatal("empty refresh regressed", r)
	}
	if r := call("/emoji", `{"emoji":["faces/a[bad].png"]}`); r["code"] != float64(0) || owner.Editor.Emoji[0] != "faces/abad.png" {
		t.Fatal(r)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("emoji/refresh unexpectedly saved", e)
	}
	if r := call("/include", `{"keywords":["first","next,word"]}`); r["code"] != float64(0) || r["data"] != nil || owner.Editor.VirtualBlockRefInclude != `first,next\,word` {
		t.Fatal(r)
	}
	after, e = os.ReadFile(file)
	if e != nil || bytes.Equal(before, after) {
		t.Fatal("keywords were not saved", e)
	}
	if r := call("/emoji", `{"emoji":[null]}`); r["code"] != float64(-1) {
		t.Fatal(r)
	}
	if r := call("/export", `{"pandocBin":[]}`); r["code"] != float64(-1) || r["data"].(map[string]any)["closeTimeout"] != float64(5000) {
		t.Fatal(r)
	}
	if r := call("/export", `{}`); r["code"] != float64(0) {
		t.Fatal(r)
	}
}
