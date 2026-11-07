package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorageHTTPRealRequestContext(t *testing.T) {
	for _, tt := range []struct {
		h    func(*gin.Context)
		body string
	}{{getCriteria, `{}`}, {getLocalStorage, `{}`}, {getRecentDocs, `{}`}, {setCriterion, `{"criterion":{"name":"x"}}`}, {removeCriterion, `{"name":"x"}`}, {setLocalStorage, `{"val":{},"app":"test"}`}, {setLocalStorageVal, `{"key":"x","val":1,"app":"test"}`}, {removeLocalStorageVals, `{"keys":["x"],"app":"test"}`}, {getOutlineStorage, `{"docID":"x"}`}, {setOutlineStorage, `{"docID":"x","val":{}}`}, {removeOutlineStorage, `{"docID":"x"}`}, {updateRecentDocViewTime, `{"rootID":"x"}`}, {updateRecentDocOpenTime, `{"rootID":"x"}`}, {updateRecentDocCloseTime, `{"rootID":"x"}`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		tt.h(c)
		var ret struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &ret); e != nil || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
			t.Fatal(w.Body.String(), e)
		}
	}
}
