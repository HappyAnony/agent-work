package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func TestOriginalCloudAccountHTTPValidationCacheAndLogout(t *testing.T) {
	old := model.Conf
	file := filepath.Join(t.TempDir(), "conf.json")
	owner, e := model.NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	model.Conf = owner
	defer func() { _ = owner.CloseCloudAccount(context.Background()); model.Conf = old }()
	u := &conf.User{UserId: "fixture", UserToken: "synthetic-token", UserTokenExpireTime: strconv.FormatInt(time.Now().Add(96*time.Hour).Unix(), 10)}
	raw, _ := json.Marshal(u)
	owner.UserData = util.AESEncrypt(string(raw))
	if e = owner.SaveError(); e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(model.RoleContextKey, model.RoleAdministrator); c.Next() })
	router.POST("/login", login)
	router.POST("/2fa", login2faCloudUser)
	router.POST("/get", getCloudUser)
	router.POST("/logout", logoutCloudUser)
	call := func(route, body string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		request := httptest.NewRequest("POST", route, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, request)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(w.Body.String())
		}
		return v
	}
	before, _ := os.ReadFile(file)
	for _, c := range [][2]string{{"/login", `{"userName":null}`}, {"/2fa", `{"token":[],"code":"x"}`}, {"/get", `{"token":[]}`}} {
		v := call(c[0], c[1])
		if v["code"] != float64(-1) {
			t.Fatal(c, v)
		}
	}
	v := call("/get", `{}`)
	if v["code"] != float64(0) || v["data"].(map[string]any)["userId"] != "fixture" {
		t.Fatal(v)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("cached read persisted")
	}
	v = call("/logout", `{}`)
	if v["code"] != float64(0) || owner.GetUser() != nil || owner.UserData != "" {
		t.Fatal(v)
	}
	v = call("/get", `{}`)
	if v["data"] != nil {
		t.Fatal(v)
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(file, 0700); e != nil {
		t.Fatal(e)
	}
	owner.SetUser(u)
	v = call("/logout", `{}`)
	if v["code"] != float64(-1) || owner.GetUser() == nil {
		t.Fatal("failed save reported success", v)
	}
}
