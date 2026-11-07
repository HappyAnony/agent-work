package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckoutHTTPMalformedArgumentsRejectedBeforeQueue(t *testing.T) {
	r := gin.New()
	r.POST("/checkout", checkoutRepo)
	for _, id := range []any{nil, 123, []any{}, "", "../escape", "short", strings.Repeat("g", 40)} {
		raw, _ := json.Marshal(map[string]any{"id": id})
		req := httptest.NewRequest("POST", "/checkout", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil || v["code"] != float64(-1) {
			t.Fatal("bad checkout request accepted")
		}
	}
}
