package api

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/model"
)

func requestOwner(t *testing.T) *model.AppConf {
	t.Helper()
	old := model.Conf
	owner, e := model.NewAppConfAt(filepath.Join(t.TempDir(), "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	model.Conf = owner
	t.Cleanup(func() { owner.CloseCloudRepo(context.Background()); model.Conf = old })
	return owner
}

func TestLegacyRequestOwnershipJoinsActualHandlerAndExcludesSelfDrain(t *testing.T) {
	owner := requestOwner(t)
	engine := gin.New()
	engine.Use(workspaceRequestOwnership)
	started, release := make(chan struct{}), make(chan struct{})
	engine.GET("/work", func(c *gin.Context) { close(started); <-release; c.JSON(http.StatusOK, gin.H{"finished": true}) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/work", nil))
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if e := owner.CloseCloudRepo(ctx); !errors.Is(e, context.DeadlineExceeded) {
		close(release)
		t.Fatal("handler detached", e)
	}
	close(release)
	<-done
	if e := owner.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	// Use a fresh owner: exit must not register the request which it drains.
	owner2, e := model.NewAppConfAt(filepath.Join(t.TempDir(), "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	model.Conf = owner2
	defer owner2.CloseCloudRepo(context.Background())
	engine.POST("/api/system/exit", func(c *gin.Context) {
		if e := owner2.CloseCloudRepo(c.Request.Context()); e != nil {
			t.Error(e)
		}
		c.Status(http.StatusOK)
	})
	exit := make(chan struct{})
	go func() {
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/system/exit", nil))
		close(exit)
	}()
	select {
	case <-exit:
	case <-time.After(time.Second):
		t.Fatal("exit waited for itself")
	}
}

func TestLegacyRequestCancellationInterruptsActualNetworkBody(t *testing.T) {
	owner := requestOwner(t)
	engine := gin.New()
	engine.Use(workspaceRequestOwnership)
	started := make(chan struct{})
	engine.POST("/read", func(c *gin.Context) {
		var b [1]byte
		if _, e := c.Request.Body.Read(b[:]); e != nil {
			t.Error(e)
			return
		}
		close(started)
		_, _ = c.Request.Body.Read(b[:])
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	address, _ := url.Parse(server.URL)
	connection, e := net.Dial("tcp", address.Host)
	if e != nil {
		t.Fatal(e)
	}
	defer connection.Close()
	if _, e = connection.Write([]byte("POST /read HTTP/1.1\r\nHost: test\r\nContent-Length: 1000000\r\n\r\nx")); e != nil {
		t.Fatal(e)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = owner.CloseCloudRepo(ctx); e != nil {
		t.Fatal("blocked body prevented real owner drain", e)
	}
}
