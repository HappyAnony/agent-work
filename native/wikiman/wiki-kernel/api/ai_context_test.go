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

func TestOriginalAIHTTPStringEnvelopeMissingKeyMalformedAndCancellation(t *testing.T) {
	prior := model.Conf
	c := model.NewAppConfWithBusiness(conf.NewBusiness(nil, map[string]map[int]string{"en_US": {193: "AI disabled"}}, nil))
	c.Lang = "en_US"
	c.AI = conf.NewAI()
	model.Conf = c
	defer func() { c.CloseAI(context.Background()); model.Conf = prior }()
	r := gin.New()
	r.POST("/chat", chatGPT)
	r.POST("/action", chatGPTWithAction)
	for route, inputs := range map[string][]string{"/chat": {`{"msg":"fixture"}`, `{}`, `{"msg":null}`, `{"msg":123}`}, "/action": {`{"ids":[],"action":"Clear context"}`, `{}`, `{"ids":[123],"action":"x"}`, `{"ids":[],"action":null}`}} {
		for i, body := range inputs {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for _, current := range []context.Context{context.Background(), ctx} {
				req := httptest.NewRequest("POST", route, bytes.NewBufferString(body)).WithContext(current)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				var v map[string]any
				if json.Unmarshal(w.Body.Bytes(), &v) != nil {
					t.Fatal(w.Body.String())
				}
				if i == 0 {
					want := float64(0)
					if current.Err() != nil {
						want = -1
					}
					if v["code"] != want || v["data"] != "" {
						t.Fatal(v)
					}
				} else if v["code"] != float64(-1) || v["data"] != nil {
					t.Fatal(v)
				}
			}
		}
	}
}
