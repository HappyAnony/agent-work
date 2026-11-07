// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

var (
	errFixedPortClosed  = errors.New("fixed_port_service_closed")
	errFixedPortRunning = errors.New("fixed_port_service_running")
)

type FixedPortConfig struct {
	Host            string
	FixedPort       string
	ServerPort      string
	Target          url.URL
	Delay           time.Duration
	ShutdownTimeout time.Duration
}

func CaptureFixedPortConfig(host string) (FixedPortConfig, error) {
	config := FixedPortConfig{
		Host:            host,
		FixedPort:       util.FixedPort,
		ServerPort:      util.ServerPort,
		Delay:           time.Second,
		ShutdownTimeout: 5 * time.Second,
	}
	if util.ServerURL == nil && config.FixedPort != "" && config.FixedPort != "0" && config.FixedPort != config.ServerPort {
		return FixedPortConfig{}, errors.New("fixed_port_target_unbound")
	}
	if util.ServerURL != nil {
		config.Target = *util.ServerURL
	}
	return config, nil
}

type FixedPortService struct {
	config  FixedPortConfig
	owner   *taskgroup.Group
	done    chan struct{}
	mu      sync.Mutex
	started bool
	err     error
}

func NewFixedPortService(config FixedPortConfig) *FixedPortService {
	if config.Delay < 0 {
		config.Delay = 0
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 5 * time.Second
	}
	return &FixedPortService{config: config, owner: taskgroup.Open(), done: make(chan struct{})}
}

func (s *FixedPortService) Start() error {
	if s == nil || s.owner == nil {
		return errFixedPortClosed
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()
	if !s.owner.Go(func(ctx context.Context) {
		err := s.run(ctx)
		if err != nil {
			logging.LogWarnf("fixed port service failed: %s", err)
		}
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	}) {
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
		return errFixedPortClosed
	}
	return nil
}

func (s *FixedPortService) run(ctx context.Context) error {
	if ctx == nil {
		return errFixedPortClosed
	}
	if s.config.Delay > 0 {
		timer := time.NewTimer(s.config.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
	}
	if s.config.FixedPort == "" || s.config.FixedPort == "0" || s.config.FixedPort == s.config.ServerPort {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(s.config.Host, s.config.FixedPort))
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(&s.config.Target)
	server := &http.Server{Handler: proxy}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	logging.LogInfof("fixed port service [%s] is running", listener.Addr())

	select {
	case err = <-serveDone:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.config.ShutdownTimeout)
		shutdownErr := server.Shutdown(shutdownCtx)
		cancel()
		if shutdownErr != nil {
			shutdownErr = errors.Join(shutdownErr, server.Close())
		}
		serveErr := <-serveDone
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		err = errors.Join(shutdownErr, serveErr)
	}
	logging.LogInfof("fixed port service [%s] is stopped", listener.Addr())
	return err
}

func (s *FixedPortService) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return errFixedPortClosed
	}
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.err
	}
}

func (s *FixedPortService) Close(ctx context.Context) error {
	if s == nil || s.owner == nil {
		return nil
	}
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if err := s.owner.Close(ctx); err != nil {
		return err
	}
	if !started {
		return nil
	}
	return s.Wait(ctx)
}

var legacyFixedPort struct {
	sync.Mutex
	service *FixedPortService
	config  FixedPortConfig
	closed  bool
}

func ConfigureFixedPortService(config FixedPortConfig) error {
	if err := model.RegisterLegacyServiceCloser("fixed-port", CloseFixedPortService); err != nil {
		return err
	}
	legacyFixedPort.Lock()
	defer legacyFixedPort.Unlock()
	if legacyFixedPort.closed {
		return errFixedPortClosed
	}
	if legacyFixedPort.service != nil {
		if sameFixedPortConfig(legacyFixedPort.config, config) {
			return nil
		}
		return errFixedPortRunning
	}
	service := NewFixedPortService(config)
	if err := service.Start(); err != nil {
		return err
	}
	legacyFixedPort.service = service
	legacyFixedPort.config = config
	return nil
}

func CloseFixedPortService(ctx context.Context) error {
	legacyFixedPort.Lock()
	legacyFixedPort.closed = true
	service := legacyFixedPort.service
	legacyFixedPort.Unlock()
	if service == nil {
		return nil
	}
	return service.Close(ctx)
}

func sameFixedPortConfig(a, b FixedPortConfig) bool {
	return a.Host == b.Host && a.FixedPort == b.FixedPort && a.ServerPort == b.ServerPort &&
		a.Target.String() == b.Target.String() && a.Delay == b.Delay && a.ShutdownTimeout == b.ShutdownTimeout
}

func InitFixedPortService(host string) {
	config, err := CaptureFixedPortConfig(host)
	if err == nil {
		err = ConfigureFixedPortService(config)
	}
	if err != nil {
		logging.LogWarnf("boot fixed port service failed: %s", err)
	}
}
