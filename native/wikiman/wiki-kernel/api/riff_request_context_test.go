package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRiffWriteHandlersUseRequestContext(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		handler func(*gin.Context)
	}{
		{"batch-due", `{"cardDues":[]}`, batchSetRiffCardsDueTime},
		{"reset", `{"type":"deck","id":"deck","deckID":"deck","blockIDs":[]}`, resetRiffCards},
		{"review", `{"deckID":"deck","cardID":"card","rating":1,"reviewedCards":[]}`, reviewRiffCard},
		{"skip", `{"deckID":"deck","cardID":"card"}`, skipReviewRiffCard},
		{"rename", `{"deckID":"deck","name":"name"}`, renameRiffDeck},
		{"remove", `{"deckID":"deck"}`, removeRiffDeck},
		{"create", `{"name":"name"}`, createRiffDeck},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/riff/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			test.handler(c)

			var result struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != -1 || result.Msg != context.Canceled.Error() {
				t.Fatalf("result = %#v, body = %s", result, w.Body.String())
			}
		})
	}
}

func TestFlashcardReadHandlersUseRequestContext(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		handler func(*gin.Context)
	}{
		{"blocks", `{"blockIDs":[]}`, getRiffCardsByBlockIDs},
		{"notebook-page", `{"id":"box","page":1,"pageSize":20}`, getNotebookRiffCards},
		{"tree-page", `{"id":"tree","page":1,"pageSize":20}`, getTreeRiffCards},
		{"deck-page", `{"id":"deck","page":1,"pageSize":20}`, getRiffCards},
		{"notebook-due", `{"notebook":"box","reviewedCards":[]}`, getNotebookRiffDueCards},
		{"tree-due", `{"rootID":"tree","reviewedCards":[]}`, getTreeRiffDueCards},
		{"deck-due", `{"deckID":"deck","reviewedCards":[]}`, getRiffDueCards},
		{"decks", ``, getRiffDecks},
		{"notebooks", `{"flashcard":true}`, lsNotebooks},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/flashcard/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			test.handler(c)

			var result struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != -1 || result.Msg != context.Canceled.Error() {
				t.Fatalf("result = %#v, body = %s", result, w.Body.String())
			}
		})
	}
}
