package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
)

func TestOriginalCloudBusinessHTTPAuthCancellationAndUploadShape(t *testing.T) {
	previous := model.Conf
	owner := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {31: "login required", 41: "uploaded %d"}}, nil))
	owner.Lang = "en_US"
	model.Conf = owner
	defer func() { owner.CloseCloudAccount(context.Background()); model.Conf = previous }()
	router := gin.New()
	router.POST("/trial", startFreeTrial)
	router.POST("/deactivate", deactivateUser)
	router.POST("/upload", uploadCloud)
	call := func(route, body string, ctx context.Context) map[string]any {
		request := httptest.NewRequest("POST", route, bytes.NewBufferString(body)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	for _, route := range []string{"/trial", "/deactivate"} {
		v := call(route, `{}`, context.Background())
		if v["code"] != float64(-1) || v["msg"] != "login required" || v["data"] != nil {
			t.Fatal(v)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		v = call(route, `{}`, ctx)
		if v["code"] != float64(-1) || v["msg"] != context.Canceled.Error() {
			t.Fatal(v)
		}
	}
	v := call("/upload", `{"id":[]}`, context.Background())
	if v["code"] != float64(-1) {
		t.Fatal(v)
	}
	v = call("/upload", `{"id":"20261019090002-par0001"}`, context.Background())
	if v["code"] != float64(0) || v["data"] != nil {
		t.Fatal("changed non-subscriber no-op", v)
	}
}
