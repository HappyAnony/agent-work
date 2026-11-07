// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"context"
	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"time"
	"wiki-kernel/model"
)

func workspaceRequestOwnership(c *gin.Context) {
	// Process exit/switch owns the drain and cannot wait for itself. WebSockets
	// already own their full connection/command lifecycle, beyond HTTP upgrade.
	p := c.Request.URL.Path
	if p == "/api/system/exit" || strings.HasPrefix(p, "/api/workspace/") || p == "/ws" || strings.HasPrefix(p, "/ws/") {
		c.Next()
		return
	}
	err := model.RunLegacyRequestContext(c.Request.Context(), func(ctx context.Context) error {
		controller := http.NewResponseController(c.Writer)
		finished := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			defer close(finished)
			_ = controller.SetReadDeadline(time.Now())
			_ = controller.SetWriteDeadline(time.Now())
		})
		defer func() {
			if !stop() {
				<-finished
			}
		}()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		return nil
	})
	if err != nil && !c.Writer.Written() {
		ret := gulu.Ret.NewResult()
		ret.Code, ret.Msg = -1, err.Error()
		c.AbortWithStatusJSON(http.StatusOK, ret)
	}
}
