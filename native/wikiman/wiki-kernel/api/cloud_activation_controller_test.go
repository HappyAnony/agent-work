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

func TestOriginalActivationHTTPAuthMalformedAndCancellation(t *testing.T) {
	previous := model.Conf
	owner := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {31: "login required"}}, nil))
	owner.Lang = "en_US"
	model.Conf = owner
	defer func() { owner.CloseCloudAccount(context.Background()); model.Conf = previous }()
	router := gin.New()
	router.POST("/check", checkActivationcode)
	router.POST("/use", useActivationcode)
	for _, route := range []string{"/check", "/use"} {
		want := float64(-1)
		if route == "/check" {
			want = 1
		}
		for _, body := range []string{`{"data":"fixture"}`, `{}`, `{"data":null}`, `{"data":[]}`, `{"data":123}`} {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for _, c := range []context.Context{context.Background(), ctx} {
				r := httptest.NewRequest("POST", route, bytes.NewBufferString(body)).WithContext(c)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				var v map[string]any
				if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
					t.Fatal(e)
				}
				if w.Code != 200 || v["code"] != want || v["data"] != nil {
					t.Fatal(route, body, v)
				}
				if body == `{"data":"fixture"}` {
					msg := "login required"
					if c.Err() != nil {
						msg = c.Err().Error()
					}
					if v["msg"] != msg {
						t.Fatal(v)
					}
				}
			}
		}
	}
}
