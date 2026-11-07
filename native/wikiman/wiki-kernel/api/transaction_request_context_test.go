package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
)

func canceledGinContext(t *testing.T) *gin.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func TestBlockTransactionsRejectCanceledRequestBeforeAdmission(t *testing.T) {
	c := canceledGinContext(t)
	ret := gulu.Ret.NewResult()
	tx := &model.Transaction{DoOperations: []*model.Operation{{Action: "create"}}}
	if performBlockTransactions(c, ret, &[]*model.Transaction{tx}) {
		t.Fatal("canceled request admitted block transaction")
	}
	if ret.Code != -1 || ret.Msg != context.Canceled.Error() {
		t.Fatalf("result = %#v", ret)
	}
}

func TestTransactionPublishRejectsCanceledRequest(t *testing.T) {
	c := canceledGinContext(t)
	ret := gulu.Ret.NewResult()
	broadcastTransactions(c, ret, nil)
	if ret.Code != -1 || ret.Msg != context.Canceled.Error() {
		t.Fatalf("broadcast result = %#v", ret)
	}

	err := pushTransactionsContext(c.Request.Context(), "app", "session", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("push error = %v, want context canceled", err)
	}
}
