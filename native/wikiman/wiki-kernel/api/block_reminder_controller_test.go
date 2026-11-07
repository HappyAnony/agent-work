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
	"wiki-kernel/util"
)

func TestOriginalReminderHTTPSubscriptionMalformedAndCancellation(t *testing.T) {
	previous, container := model.Conf, util.Container
	owner := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {29: "subscription required", 122: "ios subscription required"}}, nil))
	owner.Lang = "en_US"
	model.Conf = owner
	defer func() {
		owner.CloseCloudAccount(context.Background())
		model.Conf = previous
		util.Container = container
	}()
	router := gin.New()
	router.POST("/reminder", setBlockReminder)
	for _, ios := range []bool{false, true} {
		util.Container = "desktop"
		want := "subscription required"
		if ios {
			util.Container = "ios"
			want = "ios subscription required"
		}
		for i, body := range []string{`{"id":"fixture","timed":"0"}`, `{}`, `{"id":null,"timed":"0"}`, `{"id":"fixture","timed":123}`} {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for _, c := range []context.Context{context.Background(), ctx} {
				r := httptest.NewRequest("POST", "/reminder", bytes.NewBufferString(body)).WithContext(c)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				var v map[string]any
				if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
					t.Fatal(e)
				}
				if w.Code != 200 || v["code"] != float64(-1) || v["data"].(map[string]any)["closeTimeout"] != float64(7000) {
					t.Fatal(v)
				}
				if i == 0 {
					msg := want
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
