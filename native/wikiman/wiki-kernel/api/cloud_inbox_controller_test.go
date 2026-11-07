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

func TestOriginalInboxHTTPAuthMalformedAndCancelledShape(t *testing.T) {
	previous := model.Conf
	owner := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {31: "login required"}}, nil))
	owner.Lang = "en_US"
	owner.Editor = conf.NewEditor()
	owner.Export = conf.NewExport()
	model.Conf = owner
	defer func() { owner.CloseCloudAccount(context.Background()); model.Conf = previous }()
	router := gin.New()
	router.POST("/list", getShorthands)
	router.POST("/detail", getShorthand)
	router.POST("/remove", removeShorthands)
	for route, inputs := range map[string][]string{"/list": {`{"page":1}`, `{}`, `{"page":null}`, `{"page":[]}`, `{"page":1.5}`, `{"page":1e30}`}, "/detail": {`{"id":"fixture"}`, `{}`, `{"id":null}`, `{"id":[]}`, `{"id":123}`}, "/remove": {`{"ids":["fixture"]}`, `{}`, `{"ids":null}`, `{"ids":"fixture"}`, `{"ids":["fixture",123]}`}} {
		for i, body := range inputs {
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
				if w.Code != 200 || v["code"] != float64(1) || v["data"] != nil {
					t.Fatal(route, v)
				}
				if i == 0 {
					want := "login required"
					if c.Err() != nil {
						want = c.Err().Error()
					}
					if v["msg"] != want {
						t.Fatal(route, v)
					}
				}
			}
		}
	}
}
