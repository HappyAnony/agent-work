package publishsvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
)

type authFixture struct {
	mu       sync.Mutex
	last     *conf.Publish
	accounts map[string]Account
	sessions map[string]string
	n        int
	label    string
}

func (a *authFixture) ports() AuthPorts {
	return AuthPorts{
		Configure: func(p *conf.Publish) error {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.last != nil && reflect.DeepEqual(a.last.Auth, p.Auth) && a.last.Enable == p.Enable {
				return nil
			}
			copy, _ := clone(p)
			a.last = copy
			a.accounts = map[string]Account{"": {Token: a.label + "-anonymous"}}
			for _, v := range p.Auth.Accounts {
				a.accounts[v.Username] = Account{v.Username, v.Password, a.label + "-" + v.Username}
			}
			a.sessions = map[string]string{}
			return nil
		},
		Account: func(name string) (Account, bool) {
			a.mu.Lock()
			defer a.mu.Unlock()
			v, ok := a.accounts[name]
			return v, ok
		}, Session: func(id string) string { a.mu.Lock(); defer a.mu.Unlock(); return a.sessions[id] }, NewSession: func() string { a.mu.Lock(); defer a.mu.Unlock(); a.n++; return fmt.Sprint(a.label, a.n) }, AddSession: func(id, name string) { a.mu.Lock(); defer a.mu.Unlock(); a.sessions[id] = name }, DeleteSession: func(id string) { a.mu.Lock(); defer a.mu.Unlock(); delete(a.sessions, id) },
	}
}
func openFixture(t *testing.T, handler http.HandlerFunc) (*Service, *authFixture) {
	t.Helper()
	up := httptest.NewServer(handler)
	t.Cleanup(up.Close)
	target, _ := url.Parse(up.URL)
	a := &authFixture{label: "owner"}
	s, e := New(Options{Host: "127.0.0.1", Target: *target, Transport: http.DefaultTransport.(*http.Transport).Clone(), Auth: a.ports()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if e := s.Close(ctx); e != nil {
			t.Error(e)
		}
	})
	return s, a
}
func request(t *testing.T, port uint16, user, password string, cookie *http.Cookie) (int, string, []*http.Cookie) {
	t.Helper()
	q, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/doc?x=1", port), nil)
	if user != "" {
		q.SetBasicAuth(user, password)
	}
	if cookie != nil {
		q.AddCookie(cookie)
	}
	client := http.Client{Timeout: 2 * time.Second}
	r, e := client.Do(q)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	body, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	return r.StatusCode, string(body), r.Cookies()
}

func TestPublishListenerReuseBindFailureAndClosedOwner(t *testing.T) {
	ctx := context.Background()
	s, _ := openFixture(t, func(w http.ResponseWriter, q *http.Request) {
		if q.Header.Get(TokenHeader) != "owner-anonymous" {
			t.Error("foreign token")
		}
		fmt.Fprint(w, q.URL.RequestURI())
	})
	if s.current != nil || s.configuration.Load() != nil {
		t.Fatal("construction started a service")
	}
	p := conf.NewPublish()
	p.Enable = true
	p.Port = 0
	p.Auth.Enable = false
	port, e := s.Apply(ctx, p)
	if e != nil || port == 0 {
		t.Fatal(port, e)
	}
	again, e := s.Apply(ctx, p)
	if e != nil || again != port {
		t.Fatal("port zero restarted listener", again, e)
	}
	if code, body, _ := request(t, port, "", "", nil); code != 200 || body != "/doc?x=1" {
		t.Fatal(code, body)
	}
	busy, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer busy.Close()
	bad := *p
	bad.Port = uint16(busy.Addr().(*net.TCPAddr).Port)
	if _, e = s.Apply(ctx, &bad); e == nil {
		t.Fatal("bind failure swallowed")
	}
	if code, _, _ := request(t, port, "", "", nil); code != 200 {
		t.Fatal("old listener lost")
	}
	p.Enable = false
	if port, e = s.Apply(ctx, p); e != nil || port != 0 {
		t.Fatal(port, e)
	}
	p.Enable = true
	if _, e = s.Apply(ctx, p); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, p); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}

func TestPublishOriginalAuthenticationSessionAndRotation(t *testing.T) {
	s, _ := openFixture(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, q.Header.Get(TokenHeader)) })
	p := conf.NewPublish()
	p.Enable = true
	p.Port = 0
	p.Auth.Accounts = []*conf.BasicAuthAccount{{Username: "reader", Password: "pw"}}
	port, e := s.Apply(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if code, _, _ := request(t, port, "", "", nil); code != 401 {
		t.Fatal(code)
	}
	code, body, cookies := request(t, port, "reader", "pw", nil)
	if code != 200 || body != "owner-reader" || len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal(code, body, cookies)
	}
	if again, e := s.Apply(context.Background(), p); e != nil || again != port {
		t.Fatal(again, e)
	}
	if code, _, _ := request(t, port, "", "", cookies[0]); code != 200 {
		t.Fatal("identical read revoked session", code)
	}
	p.Auth.Accounts[0].Password = "new"
	if _, e = s.Apply(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	if code, _, _ := request(t, port, "", "", cookies[0]); code != 401 {
		t.Fatal("credential change retained cookie", code)
	}
	if code, _, _ := request(t, port, "reader", "pw", nil); code != 401 {
		t.Fatal("old password accepted", code)
	}
	if code, _, _ := request(t, port, "reader", "new", nil); code != 200 {
		t.Fatal(code)
	}
}

func TestPublishDisableCancelsAndJoinsActiveProxyRequest(t *testing.T) {
	entered, exited := make(chan struct{}), make(chan struct{})
	s, _ := openFixture(t, func(w http.ResponseWriter, q *http.Request) { close(entered); <-q.Context().Done(); close(exited) })
	p := conf.NewPublish()
	p.Enable = true
	p.Port = 0
	p.Auth.Enable = false
	port, e := s.Apply(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := http.Client{Timeout: 3 * time.Second}
		r, _ := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if r != nil {
			r.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("backend not entered")
	}
	p.Enable = false
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e = s.Apply(ctx, p); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("proxy client not joined")
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("backend not cancelled")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(q *http.Request) (*http.Response, error) { return f(q) }
func TestPublishMissingDependenciesCancellationAndBackendError(t *testing.T) {
	if _, e := New(Options{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	s, a := openFixture(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := conf.NewPublish()
	p.Enable = true
	p.Port = 0
	if _, e := s.Apply(ctx, p); !errors.Is(e, context.Canceled) || s.current != nil {
		t.Fatal(e)
	}
	a.ports().Configure(p)
	s.configuration.Store(p)
	s.options.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("backend unavailable") })
	p.Auth.Accounts = []*conf.BasicAuthAccount{{Username: "reader", Password: "pw"}}
	a.ports().Configure(p)
	q := httptest.NewRequest("GET", "http://upstream/", nil)
	q.SetBasicAuth("reader", "pw")
	if r, e := (roundTripper{s}).RoundTrip(q); e == nil || r != nil {
		t.Fatal("network error panicked or hidden", r, e)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.sessions) != 0 {
		t.Fatal("failed proxy request leaked session")
	}
}

func TestPublishCloseTimeoutRetryJoinsOwnedHandler(t *testing.T) {
	a := &authFixture{label: "retry"}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	s, e := New(Options{Host: "127.0.0.1", Target: url.URL{Scheme: "http", Host: "127.0.0.1:1"}, Auth: a.ports(), Transport: transportFunc(func(q *http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		unblock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if e := s.Close(ctx); e != nil {
			t.Error(e)
		}
	})
	p := conf.NewPublish()
	p.Enable = true
	p.Port = 0
	p.Auth.Enable = false
	port, e := s.Apply(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c := http.Client{Timeout: 2 * time.Second}
		r, _ := c.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if r != nil {
			r.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request not entered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	unblock()
	fresh, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if e := s.Close(fresh); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-fresh.Done():
		t.Fatal("client not joined")
	}
}
