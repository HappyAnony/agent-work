// SPDX-License-Identifier: AGPL-3.0-or-later
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/olahol/melody"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/model"
)

var errHTTPServiceClosed = errors.New("http_service_closed")

type protocolShutdown interface {
	Close(context.Context) error
}

type httpService struct {
	listener net.Listener
	server   *http.Server
	protocol protocolShutdown
	cancel   context.CancelFunc
	done     chan struct{}

	mu            sync.Mutex
	closeGate     chan struct{}
	closed        bool
	started       bool
	serveErr      error
	requests      map[*httpServiceRequest]struct{}
	controlActive bool
	shutdownDone  chan struct{}
	shutdownErr   error
}

type httpServiceRequest struct {
	done    chan struct{}
	control bool
}
type httpServiceRequestKey struct{}

func newHTTPService(parent context.Context, listener net.Listener, handler http.Handler, protocol protocolShutdown) *httpService {
	base, cancel := context.WithCancel(parent)
	s := &httpService{listener: listener, protocol: protocol, cancel: cancel, done: make(chan struct{}), closeGate: make(chan struct{}, 1), requests: map[*httpServiceRequest]struct{}{}}
	service := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.handleRequest(handler, w, r) }),
		BaseContext: func(net.Listener) context.Context {
			return base
		},
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.server = service
	return s
}

func (s *httpService) handleRequest(handler http.Handler, w http.ResponseWriter, r *http.Request) {
	control := r.Method == http.MethodPost && (r.URL.Path == "/api/system/exit" || r.URL.Path == "/api/system/setWorkspaceDir")
	h := &httpServiceRequest{done: make(chan struct{}), control: control}
	s.mu.Lock()
	if s.closed || control && s.controlActive {
		s.mu.Unlock()
		http.Error(w, "http_service_closed", http.StatusServiceUnavailable)
		return
	}
	s.requests[h] = struct{}{}
	if control {
		s.controlActive = true
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.requests, h)
		if control {
			s.controlActive = false
		}
		close(h.done)
		s.mu.Unlock()
	}()
	handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httpServiceRequestKey{}, h)))
}

func (s *httpService) Run() error {
	if s == nil || s.listener == nil || s.server == nil {
		return errHTTPServiceClosed
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errHTTPServiceClosed
	}
	if s.started {
		s.mu.Unlock()
		return errors.New("http_service_running")
	}
	s.started = true
	s.mu.Unlock()
	err := s.server.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		err = nil
	}
	s.mu.Lock()
	s.serveErr = err
	s.mu.Unlock()
	close(s.done)
	return err
}

func (s *httpService) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return errHTTPServiceClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.closeGate <- struct{}{}:
		defer func() { <-s.closeGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.closed = true
	initiator, _ := ctx.Value(httpServiceRequestKey{}).(*httpServiceRequest)
	if _, present := s.requests[initiator]; !present || !initiator.control {
		initiator = nil
	}
	var requests []*httpServiceRequest
	for h := range s.requests {
		if h != initiator {
			requests = append(requests, h)
		}
	}
	started := s.started
	if s.shutdownDone == nil {
		s.shutdownDone = make(chan struct{})
		go func() {
			err := s.server.Shutdown(context.Background())
			if !started {
				err = errors.Join(err, s.listener.Close())
			}
			s.mu.Lock()
			s.shutdownErr = normalizeHTTPShutdownError(err)
			close(s.shutdownDone)
			s.mu.Unlock()
		}()
	}
	shutdownDone := s.shutdownDone
	s.mu.Unlock()
	s.cancel()
	var protocolErr error
	if s.protocol != nil {
		protocolErr = s.protocol.Close(ctx)
	}
	for _, h := range requests {
		select {
		case <-h.done:
		case <-ctx.Done():
			return errors.Join(protocolErr, ctx.Err())
		}
	}
	if started {
		select {
		case <-s.done:
		case <-ctx.Done():
			return errors.Join(protocolErr, ctx.Err())
		}
	}
	// Shutdown itself also waits for the initiating handler. Its work here is
	// only transport response/cleanup; all other handlers are already joined.
	// External close callers still wait for the complete HTTP shutdown.
	if initiator == nil {
		select {
		case <-shutdownDone:
		case <-ctx.Done():
			return errors.Join(protocolErr, ctx.Err())
		}
		s.mu.Lock()
		protocolErr = errors.Join(protocolErr, s.shutdownErr)
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return errors.Join(protocolErr, s.serveErr)
}

func normalizeHTTPShutdownError(err error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var remaining error
		for _, cause := range joined.Unwrap() {
			remaining = errors.Join(remaining, normalizeHTTPShutdownError(cause))
		}
		return remaining
	}
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

type melodyShutdown struct {
	server   *melody.Melody
	count    func() int
	mu       sync.Mutex
	set      bool
	active   []*melody.Session
	draining bool
	handlers int
}

// Track the entire upgrade handler, including authentication and disconnect.
// The application session registry alone cannot account for these callbacks.
func (s *melodyShutdown) handle(fn func()) bool {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		return false
	}
	s.handlers++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.handlers--; s.mu.Unlock() }()
	fn()
	return true
}

func (s *melodyShutdown) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	if ctx == nil {
		return errHTTPServiceClosed
	}
	s.mu.Lock()
	s.draining = true
	if !s.set {
		s.active, _ = s.server.Sessions()
		if !s.server.IsClosed() {
			_ = s.server.Close()
		}
		s.set = true
	}
	active := append([]*melody.Session(nil), s.active...)
	s.mu.Unlock()
	for {
		closed := true
		for _, session := range active {
			if !session.IsClosed() {
				closed = false
				break
			}
		}
		s.mu.Lock()
		pending := s.handlers
		s.mu.Unlock()
		if closed && pending == 0 && (s.count == nil || s.count() == 0) {
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

var legacyHTTP struct {
	sync.Mutex
	owner   *taskgroup.Group
	service *httpService
	closed  bool
	running bool
	err     error
}

var legacyHTTPRunner = serveLegacyHTTP

func Start(fastMode bool) error {
	if err := model.RegisterLegacyServiceCloser("main-http", CloseLegacyHTTPService); err != nil {
		return err
	}
	legacyHTTP.Lock()
	defer legacyHTTP.Unlock()
	if legacyHTTP.closed {
		return errHTTPServiceClosed
	}
	if legacyHTTP.running {
		return nil
	}
	owner := taskgroup.Open()
	legacyHTTP.owner = owner
	legacyHTTP.running = true
	if !owner.Go(func(ctx context.Context) {
		defer owner.Stop()
		err := legacyHTTPRunner(ctx, fastMode)
		legacyHTTP.Lock()
		legacyHTTP.err = err
		legacyHTTP.Unlock()
		if err != nil && ctx.Err() == nil {
			if fastMode {
				logging.LogWarnf("boot fast kernel server failed: %s", err)
			} else {
				logging.LogErrorf("boot kernel failed: %s", err)
				os.Exit(logging.ExitCodeUnavailablePort)
			}
		}
	}) {
		legacyHTTP.running = false
		return errHTTPServiceClosed
	}
	return nil
}

// Serve preserves the blocking compatibility entry. New launch sites use Start
// so startup admission exists before the goroutine is created.
func Serve(fastMode bool) {
	if err := Start(fastMode); err != nil {
		logging.LogErrorf("start kernel server failed: %s", err)
		return
	}
	legacyHTTP.Lock()
	owner := legacyHTTP.owner
	legacyHTTP.Unlock()
	if owner != nil {
		_ = owner.Wait(context.Background())
	}
}

func publishLegacyHTTPService(ctx context.Context, service *httpService) error {
	legacyHTTP.Lock()
	defer legacyHTTP.Unlock()
	if legacyHTTP.closed || ctx.Err() != nil || legacyHTTP.service != nil {
		return errHTTPServiceClosed
	}
	legacyHTTP.service = service
	return nil
}

func CloseLegacyHTTPService(ctx context.Context) error {
	if ctx == nil {
		return errHTTPServiceClosed
	}
	legacyHTTP.Lock()
	legacyHTTP.closed = true
	owner, service := legacyHTTP.owner, legacyHTTP.service
	if owner != nil {
		owner.Stop()
	}
	legacyHTTP.Unlock()
	var err error
	if service != nil {
		err = service.Close(ctx)
	}
	if owner != nil {
		err = errors.Join(err, owner.Wait(ctx))
	}
	return err
}
