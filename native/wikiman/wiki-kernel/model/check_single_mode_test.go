// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"wiki-kernel/util"
)

// helper: stand up a router with our middleware and a sentinel handler
// that records whether it ran.
func makeGuardedTestRouter(t *testing.T) (router *gin.Engine, called *bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := false
	called = &c
	router = gin.New()
	router.POST("/api/notebook/createNotebook",
		CheckSingleNotebookMode,
		func(ctx *gin.Context) {
			*called = true
			ctx.JSON(http.StatusOK, gin.H{"code": 0, "msg": "", "data": nil})
		},
	)
	return
}

// setupConfForI18n primes Conf so Conf.Language(275) returns a non-empty
// string. The full lang bundle isn't loaded in unit tests; we directly
// inject a minimal kernel-langs map for "en_US" + set Conf.Lang.
func setupConfForI18n(t *testing.T) func() {
	t.Helper()
	prevConf := Conf
	prevLangs := util.Langs
	Conf = NewAppConf()
	Conf.Lang = "en_US"
	util.Langs = map[string]map[int]string{
		"en_US": {275: "Single notebook mode: this operation is disabled."},
	}
	Conf.business = legacyConfigurationBusiness()
	return func() {
		Conf = prevConf
		util.Langs = prevLangs
	}
}

func TestCheckSingleNotebookMode_MultiModePassthrough(t *testing.T) {
	prevMode := util.SingleNotebookMode
	t.Cleanup(func() { util.SingleNotebookMode = prevMode })
	util.SingleNotebookMode = false
	teardown := setupConfForI18n(t)
	defer teardown()

	router, called := makeGuardedTestRouter(t)
	req := httptest.NewRequest("POST", "/api/notebook/createNotebook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !*called {
		t.Error("expected downstream handler to be called in multi-mode")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestCheckSingleNotebookMode_SingleModeRejected(t *testing.T) {
	prevMode := util.SingleNotebookMode
	t.Cleanup(func() { util.SingleNotebookMode = prevMode })
	util.SingleNotebookMode = true
	teardown := setupConfForI18n(t)
	defer teardown()

	router, called := makeGuardedTestRouter(t)
	req := httptest.NewRequest("POST", "/api/notebook/createNotebook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if *called {
		t.Error("expected downstream handler NOT to be called in single mode")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected HTTP 200 (siyuan convention even on rejection), got %d", w.Code)
	}

	var body struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v\nraw: %s", err, w.Body.String())
	}
	if body.Code != SingleModeErrorCode {
		t.Errorf("expected code=%d (-16), got %d", SingleModeErrorCode, body.Code)
	}
	// (Literal-value enforcement for SingleModeErrorCode lives in
	// TestSingleModeErrorCodeIsMinus16 below — no double-assert here.)
	if body.Msg == "" {
		t.Error("expected non-empty msg from Conf.Language(275)")
	}
	if string(body.Data) != "null" {
		t.Errorf("expected data=null, got %s", string(body.Data))
	}
}

// Sanity check the constant value at compile time; if anyone "tidies" the
// constant from -16 to e.g. -15, this fails immediately rather than only
// failing in cross-side integration.
func TestSingleModeErrorCodeIsMinus16(t *testing.T) {
	if SingleModeErrorCode != -16 {
		t.Errorf("SingleModeErrorCode must be exactly -16 (pinned in api-surface.md §1.3); got %d",
			SingleModeErrorCode)
	}
}
