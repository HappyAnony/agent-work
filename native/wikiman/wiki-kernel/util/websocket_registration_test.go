package util

import (
	"github.com/olahol/melody"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestWebSocketReconnectRetainsReplacement(t *testing.T) {
	old := &melody.Session{Request: httptest.NewRequest("GET", "/ws?app=lifecycle-test&id=same", nil)}
	next := &melody.Session{Request: httptest.NewRequest("GET", "/ws?app=lifecycle-test&id=same", nil)}
	AddPushChan(old)
	AddPushChan(next)
	t.Cleanup(func() { RemovePushChan(next) })
	RemovePushChan(old)
	group, ok := sessions.Load("lifecycle-test")
	if !ok {
		t.Fatal("old disconnect removed replacement app")
	}
	if got, present := group.(*sync.Map).Load("same"); !present || got != next {
		t.Fatal("replacement registration lost")
	}
	RemovePushChan(next)
	if _, ok = sessions.Load("lifecycle-test"); ok {
		t.Fatal("final disconnect retained app")
	}
}
