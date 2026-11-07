package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
)

func TestHTTPServiceShutdownFromControlRequestJoinsOthersAndReturnsResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	workEntered, workCancelled, releaseWork := make(chan struct{}), make(chan struct{}), make(chan struct{})
	controlEntered, beginClose, closeReturned, releaseResponse := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan struct{})
	var service *httpService
	service = newHTTPService(context.Background(), listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/work" {
			close(workEntered)
			<-r.Context().Done()
			close(workCancelled)
			<-releaseWork
			return
		}
		close(controlEntered)
		<-beginClose
		closeReturned <- service.Close(context.WithoutCancel(r.Context()))
		<-releaseResponse
		_, _ = w.Write([]byte("closed"))
	}), nil)
	release := func(c chan struct{}) {
		select {
		case <-c:
		default:
			close(c)
		}
	}
	t.Cleanup(func() {
		release(beginClose)
		release(releaseWork)
		release(releaseResponse)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	go func() { _ = service.Run() }()
	client := &http.Client{Timeout: 3 * time.Second}
	request := func(route string, done chan error) {
		response, e := client.Post("http://"+listener.Addr().String()+route, "application/json", strings.NewReader("{}"))
		if e == nil {
			_, e = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				e = errors.New("unexpected status")
			}
		}
		done <- e
	}
	workDone, responseDone := make(chan error, 1), make(chan error, 1)
	go request("/work", workDone)
	<-workEntered
	go request("/api/system/exit", responseDone)
	<-controlEntered
	duplicate, err := client.Post("http://"+listener.Addr().String()+"/api/system/exit", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = duplicate.Body.Close()
	if duplicate.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("duplicate shutdown control was admitted")
	}
	close(beginClose)
	<-workCancelled
	select {
	case e := <-closeReturned:
		t.Fatal("close skipped active business handler", e)
	default:
	}
	close(releaseWork)
	select {
	case err = <-closeReturned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown waited for its initiating handler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err = service.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("external close did not join control response", err)
	}
	cancel()
	close(releaseResponse)
	if err = <-responseDone; err != nil {
		t.Fatal(err)
	}
	if err = <-workDone; err != nil {
		t.Fatal(err)
	}
	if err = service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPServiceOrdinaryRequestCannotSkipItsOwnJoin(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	var service *httpService
	service = newHTTPService(context.Background(), listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Millisecond)
		defer cancel()
		result <- service.Close(ctx)
	}), nil)
	go func() { _ = service.Run() }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/ordinary")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if err = <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("ordinary request bypassed drain", err)
	}
	if err = service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPServiceCancelsRequestsAndJoinsServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, returned := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(returned)
	})
	service := newHTTPService(context.Background(), listener, handler, nil)
	runDone := make(chan error, 1)
	go func() { runDone <- service.Run() }()
	requestDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if response != nil {
			_ = response.Body.Close()
		}
		requestDone <- requestErr
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not enter")
	}
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = service.Close(deadline); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	default:
		t.Fatal("request callback was not joined")
	}
	if err = <-runDone; err != nil {
		t.Fatal(err)
	}
	<-requestDone
}

func TestHTTPServiceCloseTimeoutIsRetryable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	service := newHTTPService(context.Background(), listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("done"))
	}), nil)
	go func() { _ = service.Run() }()
	go func() {
		response, _ := http.Get("http://" + listener.Addr().String())
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err = service.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		cancel()
		t.Fatalf("expected retryable timeout, got %v", err)
	}
	cancel()
	close(release)
	if err = service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMelodyShutdownWaitsForDisconnect(t *testing.T) {
	m := melody.New()
	var active atomic.Int32
	connected := make(chan struct{})
	m.HandleConnect(func(*melody.Session) { active.Add(1); close(connected) })
	m.HandleDisconnect(func(*melody.Session) { active.Add(-1) })
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = m.HandleRequest(w, r)
	}))
	defer httpServer.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+httpServer.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	<-connected
	drain := &melodyShutdown{server: m, count: func() int { return int(active.Load()) }}
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = drain.Close(deadline); err != nil {
		t.Fatal(err)
	}
	if active.Load() != 0 || !m.IsClosed() {
		t.Fatalf("websocket drain incomplete: active=%d closed=%v", active.Load(), m.IsClosed())
	}
}

func TestLegacyHTTPStartOwnsAndJoinsRunner(t *testing.T) {
	entered, returned := make(chan struct{}), make(chan struct{})
	oldRunner := legacyHTTPRunner
	legacyHTTPRunner = func(ctx context.Context, _ bool) error {
		close(entered)
		<-ctx.Done()
		close(returned)
		return ctx.Err()
	}
	legacyHTTP.Lock()
	oldOwner, oldService := legacyHTTP.owner, legacyHTTP.service
	oldClosed, oldRunning, oldErr := legacyHTTP.closed, legacyHTTP.running, legacyHTTP.err
	legacyHTTP.owner, legacyHTTP.service = nil, nil
	legacyHTTP.closed, legacyHTTP.running, legacyHTTP.err = false, false, nil
	legacyHTTP.Unlock()
	t.Cleanup(func() {
		_ = CloseLegacyHTTPService(context.Background())
		legacyHTTPRunner = oldRunner
		legacyHTTP.Lock()
		legacyHTTP.owner, legacyHTTP.service = oldOwner, oldService
		legacyHTTP.closed, legacyHTTP.running, legacyHTTP.err = oldClosed, oldRunning, oldErr
		legacyHTTP.Unlock()
	})
	if err := Start(true); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := CloseLegacyHTTPService(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	default:
		t.Fatal("HTTP runner was not joined")
	}
	if err := Start(true); !errors.Is(err, errHTTPServiceClosed) {
		t.Fatalf("closed owner accepted late start: %v", err)
	}
}

func TestWebSocketDrainJoinsUnregisteredHandlerAndRejectsLateUpgrade(t *testing.T) {
	owner := &melodyShutdown{server: melody.New()}
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		owner.handle(func() { close(entered); <-release })
		close(returned)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := owner.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unregistered handler was not joined: %v", err)
	}
	if owner.handle(func() { t.Error("late upgrade entered") }) {
		t.Fatal("draining owner admitted upgrade")
	}
	close(release)
	<-returned
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWebSocketDrainWaitsForActualDisconnectReturn(t *testing.T) {
	m := melody.New()
	owner := &melodyShutdown{server: m}
	connected, disconnecting, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m.HandleConnect(func(*melody.Session) { close(connected) })
	m.HandleDisconnect(func(*melody.Session) { close(disconnecting); <-release })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner.handle(func() { _ = m.HandleRequest(w, r) })
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	<-connected
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err = owner.Close(ctx)
	select {
	case <-disconnecting:
	default:
		close(release)
		t.Fatal("disconnect did not start")
	}
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("disconnect return was not joined: %v", err)
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type blockingProtocol struct{ entered, release chan struct{} }

func (p *blockingProtocol) Close(context.Context) error { close(p.entered); <-p.release; return nil }

func TestHTTPServiceConcurrentCloseRespectsDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	protocol := &blockingProtocol{make(chan struct{}), make(chan struct{})}
	service := newHTTPService(context.Background(), listener, http.NotFoundHandler(), protocol)
	firstDone := make(chan error, 1)
	go func() { firstDone <- service.Close(context.Background()) }()
	<-protocol.entered
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- service.Close(ctx) }()
	cancel()
	select {
	case err = <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("concurrent close: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("concurrent close ignored cancellation")
	}
	close(protocol.release)
	if err = <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPServiceClosedBeforeRunRejectsStartAndClosesIdempotently(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := newHTTPService(context.Background(), listener, http.NotFoundHandler(), nil)
	for i := 0; i < 2; i++ {
		if err = service.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err = service.Run(); !errors.Is(err, errHTTPServiceClosed) {
		t.Fatalf("late Run: %v", err)
	}
}

func TestHTTPShutdownNormalizationPreservesJoinedFailure(t *testing.T) {
	for _, benign := range []error{net.ErrClosed, http.ErrServerClosed} {
		err := normalizeHTTPShutdownError(errors.Join(benign, context.DeadlineExceeded))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("failure lost: %v", err)
		}
		if err = normalizeHTTPShutdownError(benign); err != nil {
			t.Fatalf("normal close failed: %v", err)
		}
	}
}
