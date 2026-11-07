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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

// SingleModeErrorCode is the API response code returned when a write
// operation is rejected because the kernel runs in single-notebook mode.
//
// Pinned in docs/feature/siyuan/Architecture/api-surface.md §1.3 and shared
// with the frontend. Do not change without updating both sides.
const SingleModeErrorCode = -16

// CheckSingleNotebookMode is the Gin middleware that blocks notebook- and
// document-level write operations when the kernel runs in single-notebook
// mode (util.SingleNotebookMode == true).
//
// Middleware chain order (per Architecture §04 §2.1):
//
//	CheckAuth → CheckAdminRole → CheckReadonly → CheckSingleNotebookMode → handler
//
// Behavior:
//   - util.SingleNotebookMode == false : c.Next() (pass-through; no log)
//   - util.SingleNotebookMode == true  : emit single_mode_guard_block INFO
//     log, return HTTP 200 with body {code: -16, msg: localized, data: nil},
//     then c.Abort().
//
// The middleware does NOT read the request body. It does NOT inspect
// request headers beyond what c.ClientIP() reads.
func CheckSingleNotebookMode(c *gin.Context) {
	if !util.SingleNotebookMode {
		c.Next()
		return
	}

	// Structured log for operator diagnosability of Flow E (legacy client
	// hitting a guarded route). INFO level keeps volume reasonable.
	// Request body is intentionally NOT logged to avoid leaking any
	// document content the client may have POSTed.
	logging.LogInfof(
		"event=single_mode_guard_block apiPath=%s remoteAddr=%s",
		c.Request.URL.Path, c.ClientIP(),
	)

	// Build the standard siyuan response envelope — same constructor used
	// by CheckReadonly (model/session.go:187) and other handlers.
	result := util.NewResult()
	result.Code = SingleModeErrorCode
	result.Msg = Conf.Language(275)
	// result.Data stays nil per architecture standard error envelope
	// (api-surface.md §1.2). Existing CheckReadonly sets a closeTimeout
	// hint here; the single-mode bundle silently ignores -16 so a
	// closeTimeout is unnecessary and would be confusing to non-single
	// clients accidentally pointed at this kernel.
	c.JSON(http.StatusOK, result)
	c.Abort()
}
