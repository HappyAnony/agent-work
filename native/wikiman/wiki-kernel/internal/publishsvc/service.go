// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original publishing proxy with explicit dependencies and owned lifetimes.
package publishsvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"wiki-kernel/conf"
)

var ErrDependency = errors.New("publish_service_unbound")
var ErrClosed = errors.New("publish_service_closed")

type Account struct{ Username, Password, Token string }
type AuthPorts struct {
	// Configure must atomically replace the authority; errors retain old state.
	// Callbacks are bounded and cannot reenter the owning service.
	Configure     func(*conf.Publish) error
	Account       func(string) (Account, bool)
	Session       func(string) string
	NewSession    func() string
	AddSession    func(string, string)
	DeleteSession func(string)
	Close         func() error // Optional when the auth authority is owned by this service.
}
type Options struct {
	Host      string
	Target    url.URL
	Transport http.RoundTripper
	Auth      AuthPorts
}

type Service struct {
	authGate      sync.RWMutex
	gate          chan struct{}
	options       Options
	configuration atomic.Pointer[conf.Publish]
	current       *run
	retiring      []*run
	closed        bool
	authClosed    bool
}
type run struct {
	server   *http.Server
	listener net.Listener
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	stopping bool
	requests sync.WaitGroup
	err      error
	port     uint16
}

func New(options Options) (*Service, error) {
	a := options.Auth
	if options.Host == "" || options.Target.Host == "" || (options.Target.Scheme != "http" && options.Target.Scheme != "https") || options.Transport == nil || a.Configure == nil || a.Account == nil || a.Session == nil || a.NewSession == nil || a.AddSession == nil || a.DeleteSession == nil {
		return nil, ErrDependency
	}
	s := &Service{options: options, gate: make(chan struct{}, 1)}
	return s, nil
}
func (s *Service) enter(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) leave() { <-s.gate }
func clone(p *conf.Publish) (*conf.Publish, error) {
	if p == nil || p.Auth == nil {
		return nil, ErrDependency
	}
	out := *p
	auth := *p.Auth
	out.Auth = &auth
	auth.Accounts = nil
	for _, a := range p.Auth.Accounts {
		if a == nil {
			return nil, ErrDependency
		}
		v := *a
		auth.Accounts = append(auth.Accounts, &v)
	}
	return &out, nil
}

// Apply binds a replacement before retiring the old listener. Binding failures
// retain the working service and do not reconfigure credentials. Port 0 records
// the requested port separately from the actual assigned port, so reads reuse it.
func (s *Service) Apply(ctx context.Context, input *conf.Publish) (uint16, error) {
	p, e := clone(input)
	if e != nil {
		return 0, e
	}
	if e = s.enter(ctx); e != nil {
		return 0, e
	}
	defer s.leave()
	if e = ctx.Err(); e != nil {
		return 0, e
	}
	if s.closed {
		return 0, ErrClosed
	}
	old := s.configuration.Load()
	active := s.current
	if active != nil {
		select {
		case <-active.done:
			s.current = nil
			s.retiring = append(s.retiring, active)
			active = nil
		default:
		}
	}
	reuse := p.Enable && active != nil && old != nil && (old.Port == p.Port || active.port == p.Port)
	var next *run
	if p.Enable && !reuse {
		listener, e := net.Listen("tcp", net.JoinHostPort(s.options.Host, strconv.Itoa(int(p.Port))))
		if e != nil {
			return 0, e
		}
		_, port, e := net.SplitHostPort(listener.Addr().String())
		if e != nil {
			listener.Close()
			return 0, e
		}
		n, e := strconv.ParseUint(port, 10, 16)
		if e != nil {
			listener.Close()
			return 0, e
		}
		next = &run{listener: listener, port: uint16(n), done: make(chan struct{})}
	}
	if e = ctx.Err(); e != nil {
		if next != nil {
			next.listener.Close()
		}
		return 0, e
	}
	s.authGate.Lock()
	authInput, _ := clone(p)
	if e = s.options.Auth.Configure(authInput); e != nil {
		s.authGate.Unlock()
		if next != nil {
			next.listener.Close()
		}
		return 0, e
	}
	s.configuration.Store(p)
	s.authGate.Unlock()
	if next != nil {
		s.start(next)
		s.current = next
	} else if !p.Enable {
		s.current = nil
	}
	if active != nil && !reuse {
		s.retiring = append(s.retiring, active)
	}
	// Runtime publication is accepted. Always complete teardown, even if its HTTP
	// caller disconnects; a bounded close error remains retryable through Close.
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	e = s.reap(drain)
	if s.current == nil {
		return 0, e
	}
	return s.current.port, e
}
func (s *Service) start(r *run) {
	owner, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	target := s.options.Target
	proxy := &httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) { p.SetURL(&target); p.SetXForwarded() }, Transport: roundTripper{s}}
	r.server = &http.Server{ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return owner }, Handler: http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		if r.stopping {
			r.mu.Unlock()
			http.Error(w, "publish service closed", http.StatusServiceUnavailable)
			return
		}
		r.requests.Add(1)
		r.mu.Unlock()
		defer r.requests.Done()
		proxy.ServeHTTP(w, q)
	})}
	go func() {
		e := r.server.Serve(r.listener)
		r.stopAdmission()
		r.cancel()
		_ = r.server.Close()
		r.requests.Wait()
		if !errors.Is(e, http.ErrServerClosed) && !errors.Is(e, net.ErrClosed) {
			r.err = e
		}
		close(r.done)
	}()
}
func (r *run) stopAdmission() { r.mu.Lock(); r.stopping = true; r.mu.Unlock() }
func (r *run) stop(ctx context.Context) error {
	r.stopAdmission()
	r.cancel()
	e := r.server.Close()
	if errors.Is(e, net.ErrClosed) {
		e = nil
	}
	select {
	case <-r.done:
		return errors.Join(e, r.err)
	case <-ctx.Done():
		return errors.Join(e, ctx.Err())
	}
}
func (s *Service) reap(ctx context.Context) error {
	var pending []*run
	var result error
	for _, r := range s.retiring {
		e := r.stop(ctx)
		result = errors.Join(result, e)
		select {
		case <-r.done:
		default:
			pending = append(pending, r)
		}
	}
	s.retiring = pending
	return result
}
func (s *Service) Close(ctx context.Context) error {
	if e := s.enter(ctx); e != nil {
		return e
	}
	defer s.leave()
	s.closed = true
	if s.current != nil {
		s.retiring = append(s.retiring, s.current)
		s.current = nil
	}
	e := s.reap(ctx)
	if len(s.retiring) == 0 {
		if !s.authClosed && s.options.Auth.Close != nil {
			if err := s.options.Auth.Close(); err != nil {
				e = errors.Join(e, err)
			} else {
				s.authClosed = true
			}
		}
		if t, ok := s.options.Transport.(interface{ CloseIdleConnections() }); ok {
			t.CloseIdleConnections()
		}
	}
	return e
}

const TokenHeader = "X-Auth-Token"
const SessionCookie = "publish-visitor-session-id"
const Challenge = `Basic realm="SiYuan Authorization Require", charset="UTF-8"`

type roundTripper struct{ s *Service }

func (t roundTripper) RoundTrip(q *http.Request) (*http.Response, error) {
	account, cookie, rejection, e := t.authorize(q)
	if e != nil || rejection != nil {
		return rejection, e
	}
	out := q.Clone(q.Context())
	out.Header.Set(TokenHeader, account.Token)
	response, e := t.s.options.Transport.RoundTrip(out)
	if e != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		t.cleanupCookie(cookie)
		return response, e
	}
	if response == nil {
		t.cleanupCookie(cookie)
		return nil, fmt.Errorf("publish_backend_empty_response")
	}
	if cookie != nil {
		if response.Header == nil {
			response.Header = make(http.Header)
		}
		response.Header.Add("Set-Cookie", cookie.String())
	}
	return response, nil
}

func (t roundTripper) cleanupCookie(cookie *http.Cookie) {
	if cookie != nil {
		t.s.authGate.RLock()
		defer t.s.authGate.RUnlock()
		t.s.options.Auth.DeleteSession(cookie.Value)
	}
}

// Session creation and account lookup linearize with credential replacement.
// Release before network IO so disabling can cancel an in-flight backend call.
func (t roundTripper) authorize(q *http.Request) (Account, *http.Cookie, *http.Response, error) {
	t.s.authGate.RLock()
	defer t.s.authGate.RUnlock()
	p := t.s.configuration.Load()
	if p == nil {
		return Account{}, nil, nil, ErrDependency
	}
	a := t.s.options.Auth
	if !p.Auth.Enable {
		account, ok := a.Account("")
		if !ok {
			return Account{}, nil, nil, ErrDependency
		}
		return account, nil, nil, nil
	}
	if cookie, e := q.Cookie(SessionCookie); e == nil {
		if name := a.Session(cookie.Value); name != "" {
			if account, ok := a.Account(name); ok {
				return account, nil, nil, nil
			}
			a.DeleteSession(cookie.Value)
		}
	}
	name, password, ok := q.BasicAuth()
	account, found := a.Account(name)
	if !ok || !found || account.Username == "" || account.Password != password {
		return Account{}, nil, &http.Response{StatusCode: http.StatusUnauthorized, Status: http.StatusText(http.StatusUnauthorized), Proto: q.Proto, ProtoMajor: q.ProtoMajor, ProtoMinor: q.ProtoMinor, Request: q, Header: http.Header{"WWW-Authenticate": {Challenge}}, Body: io.NopCloser(http.NoBody), ContentLength: -1}, nil
	}
	id := a.NewSession()
	if id == "" {
		return Account{}, nil, nil, ErrDependency
	}
	a.AddSession(id, name)
	return account, &http.Cookie{Name: SessionCookie, Value: id, Path: "/", HttpOnly: true}, nil, nil
}
