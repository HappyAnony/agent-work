package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func TestOriginalAppearanceHTTPMergedMetadataAndDeviceResources(t *testing.T) {
	previous, working, installed, themes, icons, lang, status := model.Conf, util.WorkingDir, util.AppearancePath, util.ThemesPath, util.IconsPath, util.Lang, util.StatusBarCfg
	defer func() {
		model.CloseThemeWatchers()
		model.Conf, util.WorkingDir, util.AppearancePath, util.ThemesPath, util.IconsPath, util.Lang, util.StatusBarCfg = previous, working, installed, themes, icons, lang, status
	}()
	root := t.TempDir()
	util.WorkingDir = filepath.Join(root, "app")
	util.AppearancePath = filepath.Join(root, "device", "appearance")
	util.ThemesPath = filepath.Join(util.AppearancePath, "themes")
	util.IconsPath = filepath.Join(util.AppearancePath, "icons")
	for name, value := range map[string]string{"themes/daylight/theme.json": `{"version":"light-v","modes":["light"]}`, "themes/midnight/theme.json": `{"version":"dark-v","modes":["dark"]}`, "icons/material/icon.json": `{"version":"icon-v"}`, "emojis/conf.json": "[]"} {
		file := filepath.Join(util.WorkingDir, "appearance", name)
		os.MkdirAll(filepath.Dir(file), 0700)
		if e := os.WriteFile(file, []byte(value), 0600); e != nil {
			t.Fatal(e)
		}
	}
	file := filepath.Join(root, "conf.json")
	owner, e := model.NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	owner.Appearance = conf.NewAppearance()
	owner.Lang = "en_US"
	model.Conf = owner
	router := gin.New()
	router.POST("/appearance", setAppearance)
	input := conf.NewAppearance()
	input.Lang = "zh_CN"
	input.ThemeLight = "not-installed"
	raw, _ := json.Marshal(input)
	request := httptest.NewRequest("POST", "/appearance", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	var result struct {
		Code int
		Data *conf.Appearance
	}
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Code != 0 {
		t.Fatal(w.Body.String(), e)
	}
	if result.Data.ThemeLight != "daylight" || result.Data.ThemeVer != "light-v" || result.Data.IconVer != "icon-v" || owner.Lang != "zh_CN" || util.Lang != "zh_CN" {
		t.Fatal(result.Data)
	}
	if _, e = os.Stat(filepath.Join(util.ThemesPath, "daylight", "theme.json")); e != nil {
		t.Fatal(e)
	}
	var saved conf.AppData
	raw, e = os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &saved); e != nil || saved.Appearance.ThemeVer != "light-v" {
		t.Fatal(e)
	}
}
