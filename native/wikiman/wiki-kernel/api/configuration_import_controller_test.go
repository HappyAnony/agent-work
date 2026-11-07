package api

import (
	ziparchive "archive/zip"
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
)

func TestOriginalConfigurationImportJSONZipAndFailedSave(t *testing.T) {
	previous := model.Conf
	file := filepath.Join(t.TempDir(), "conf.json")
	owner, e := model.NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	owner.Search = conf.NewSearch()
	owner.UserData = "private"
	model.Conf = owner
	defer func() { model.Conf = previous }()
	router := gin.New()
	router.POST("/import", importConf)
	call := func(name string, payload []byte) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		f, e := w.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = f.Write(payload)
		w.Close()
		req := httptest.NewRequest("POST", "/import", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var result map[string]any
		if e = json.Unmarshal(response.Body.Bytes(), &result); e != nil {
			t.Fatal(response.Body.String())
		}
		return result
	}
	v := call("conf.json", []byte(`{"search":{"limit":17},"userData":"foreign"}`))
	if v["code"] != float64(0) || owner.Search.Limit != 17 || owner.UserData != "private" || v["data"] != nil {
		t.Fatal(v)
	}
	archive := func(name string) []byte {
		var buf bytes.Buffer
		z := ziparchive.NewWriter(&buf)
		f, _ := z.Create(name)
		_, _ = f.Write([]byte(`{"search":{"limit":18}}`))
		z.Close()
		return buf.Bytes()
	}
	v = call("conf.zip", archive("conf.json"))
	if v["code"] != float64(0) || owner.Search.Limit != 18 {
		t.Fatal(v)
	}
	before, _ := os.ReadFile(file)
	for _, input := range []struct {
		name string
		body []byte
	}{{"f.zip", archive("../escape.json")}, {"f.json", []byte(`{"editor":null}`)}, {"f.txt", []byte(`{}`)}} {
		v = call(input.name, input.body)
		if v["code"] != float64(-1) {
			t.Fatal(v)
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid archive altered state")
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	os.Mkdir(file, 0700)
	v = call("f.json", []byte(`{"search":{"limit":99}}`))
	if v["code"] != float64(-1) || owner.Search.Limit != 18 {
		t.Fatal("save failure published candidate", v)
	}
}
