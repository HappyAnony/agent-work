package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/server/proxy"
	"wiki-kernel/util"
)

func TestOriginalPublishHTTPPersistsReportsErrorsAndKeepsJWTOnGet(t *testing.T) {
	old, oldURL, oldHost := model.Conf, util.ServerURL, proxy.Host
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if e := proxy.ClosePublishService(ctx); e != nil {
			t.Error(e)
		}
		model.Conf, util.ServerURL, proxy.Host = old, oldURL, oldHost
	}()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		token, e := model.ParseJWT(q.Header.Get(model.XAuthTokenKey))
		if e != nil || !token.Valid || model.GetClaimRole(model.GetTokenClaims(token)) != model.RoleReader {
			http.Error(w, "invalid reader", 403)
			return
		}
		fmt.Fprint(w, "published fixture")
	}))
	defer backend.Close()
	util.ServerURL, _ = url.Parse(backend.URL)
	proxy.Host = "127.0.0.1"
	file := filepath.Join(t.TempDir(), "conf.json")
	owner, e := model.NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	owner.Publish = conf.NewPublish()
	model.Conf = owner
	router := gin.New()
	router.POST("/set", setPublish)
	router.POST("/get", getPublish)
	call := func(path, body string) map[string]any {
		t.Helper()
		q := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		q.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, q)
		var r map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := call("/set", `{"enable":true,"port":0,"auth":{"enable":false,"accounts":[]}}`)
	if r["code"] != float64(0) {
		t.Fatal(r)
	}
	data := r["data"].(map[string]any)
	port := uint16(data["port"].(float64))
	if port == 0 || data["publish"].(map[string]any)["enable"] != true {
		t.Fatal(r)
	}
	token := model.GetBasicAuthAccount("").Token
	r = call("/get", "")
	if r["code"] != float64(0) || r["data"].(map[string]any)["port"] != float64(port) || model.GetBasicAuthAccount("").Token != token {
		t.Fatal("get restarted or revoked auth", r)
	}
	client := http.Client{Timeout: time.Second}
	reply, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(reply.Body)
	reply.Body.Close()
	if e != nil || reply.StatusCode != 200 || string(body) != "published fixture" {
		t.Fatal(reply.StatusCode, string(body), e)
	}
	if r = call("/set", `{"auth":null}`); r["code"] != float64(-1) {
		t.Fatal("nil auth accepted", r)
	}
	if r = call("/set", `{"enable":false,"port":0,"auth":{"enable":false,"accounts":[]}}`); r["code"] != float64(0) || r["data"].(map[string]any)["port"] != float64(0) {
		t.Fatal(r)
	}
	if _, e := model.ParseJWT(token); e == nil {
		t.Fatal("disabled service retained old JWT")
	}
}
