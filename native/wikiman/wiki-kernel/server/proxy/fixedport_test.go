package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"wiki-kernel/util"
)

func TestFixedPortServiceCancellationBeforeDelayAndRepeatedStart(t *testing.T) {
	target, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewFixedPortService(FixedPortConfig{
		Host: "127.0.0.1", FixedPort: freePort(t), ServerPort: "1", Target: *target,
		Delay: time.Hour, ShutdownTimeout: time.Second,
	})
	if err = service.Start(); err != nil {
		t.Fatal(err)
	}
	if err = service.Start(); err != nil {
		t.Fatalf("repeated start = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = service.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFixedPortServiceCapturesGlobalConfiguration(t *testing.T) {
	oldFixed, oldServer, oldURL := util.FixedPort, util.ServerPort, util.ServerURL
	t.Cleanup(func() { util.FixedPort, util.ServerPort, util.ServerURL = oldFixed, oldServer, oldURL })
	original, err := url.Parse("http://127.0.0.1:7101")
	if err != nil {
		t.Fatal(err)
	}
	util.FixedPort, util.ServerPort, util.ServerURL = "7100", "7101", original
	config, err := CaptureFixedPortConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := url.Parse("http://127.0.0.1:7201")
	if err != nil {
		t.Fatal(err)
	}
	util.FixedPort, util.ServerPort, util.ServerURL = "7200", "7201", changed
	if config.FixedPort != "7100" || config.ServerPort != "7101" || config.Target.String() != original.String() {
		t.Fatalf("captured config changed: %#v", config)
	}
}

func TestFixedPortServiceShutdownJoinsActiveRequestAndReleasesPort(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	service := NewFixedPortService(FixedPortConfig{
		Host: "127.0.0.1", FixedPort: port, ServerPort: "1", Target: *targetURL,
		ShutdownTimeout: time.Second,
	})
	if err = service.Start(); err != nil {
		t.Fatal(err)
	}
	proxyURL := "http://127.0.0.1:" + port
	requestDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get(proxyURL)
		if requestErr == nil {
			_, requestErr = io.Copy(io.Discard, response.Body)
			requestErr = errors.Join(requestErr, response.Body.Close())
		}
		requestDone <- requestErr
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy request did not reach target")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- service.Close(context.Background()) }()
	select {
	case err = <-closeDone:
		t.Fatalf("close returned before active request: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err = <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err = <-closeDone; err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("fixed port was not released: %v", err)
	}
	_ = listener.Close()
}

func TestFixedPortServiceReportsListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	target, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewFixedPortService(FixedPortConfig{
		Host: "127.0.0.1", FixedPort: portString(port), ServerPort: "1", Target: *target,
		ShutdownTimeout: time.Second,
	})
	if err = service.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = service.Wait(ctx); err == nil {
		t.Fatal("listen failure was swallowed")
	}
	if err = service.Close(ctx); err == nil {
		t.Fatal("close did not retain listen failure")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	return portString(port)
}

func portString(port int) string {
	return fmt.Sprintf("%d", port)
}
