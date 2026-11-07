package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogueHTTPRealRequestContext(t *testing.T) {
	for _, tt := range []struct {
		h    func(*gin.Context)
		body string
	}{{getBazaarPackageREAME, `{"repoURL":"https://github.com/owner/repo","repoHash":"commit","packageType":"plugins"}`}, {getBazaarPlugin, `{"frontend":"desktop"}`}, {getInstalledPlugin, `{"frontend":"desktop"}`}, {getBazaarWidget, `{}`}, {getInstalledWidget, `{}`}, {getBazaarIcon, `{}`}, {getInstalledIcon, `{}`}, {getBazaarTemplate, `{}`}, {getInstalledTemplate, `{}`}, {getBazaarTheme, `{}`}, {getInstalledTheme, `{}`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		tt.h(c)
		var ret struct {
			Code int
			Msg  string
		}
		if e := json.Unmarshal(w.Body.Bytes(), &ret); e != nil || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
			t.Fatal(w.Body.String(), e)
		}
	}
}
