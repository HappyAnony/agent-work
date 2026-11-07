// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package aiflow preserves original non-streaming AI orchestration with owned
// history, immutable settings and explicit provider/source/UI capabilities.
package aiflow

import (
	"context"
	"errors"
	"math"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/nativeparser"
)

var ErrDependency = errors.New("ai_dependencies_unbound")
var ErrConfig = errors.New("ai_configuration_invalid")
var ErrProvider = errors.New("ai_provider_failed")
var ErrClosed = errors.New("ai_closed")

type Settings struct {
	API      conf.OpenAI
	Markdown *nativeparser.Markdown
}
type Part struct {
	Text string
	Stop bool
}
type Client interface {
	Chat(context.Context, string, []string) (Part, error)
	Close()
}
type Event string

const (
	MissingKey Event = "missing-key"
	Requesting Event = "requesting"
	Continuing Event = "continuing"
	Failed     Event = "failed"
	Clear      Event = "clear"
)

type Options struct {
	Load  func(context.Context) (Settings, error)
	Open  func(conf.OpenAI) (Client, error)
	Event func(context.Context, Event)
	// A trusted device owner may retain conversation across source commits.
	Conversation *Conversation
}
type identity struct{ Key, Proxy, Base, Agent, Provider, Version, Model string }
type Conversation struct {
	serial  chan struct{}
	history []string
	key     identity
}

func NewConversation() *Conversation {
	return &Conversation{serial: make(chan struct{}, 1)}
}

type Service struct {
	o      Options
	gate   chan struct{}
	life   context.Context
	cancel context.CancelFunc
	*Conversation
}

func New(o Options) (*Service, error) {
	if o.Load == nil || o.Open == nil || o.Event == nil {
		return nil, ErrDependency
	}
	c, cancel := context.WithCancel(context.Background())
	conversation := o.Conversation
	if conversation == nil {
		conversation = NewConversation()
	}
	return &Service{o: o, gate: make(chan struct{}, 1), life: c, cancel: cancel, Conversation: conversation}, nil
}
func (s *Service) enter(ctx context.Context) (context.Context, func(), error) {
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	clean := func() { stop(); cancel() }
	select {
	case s.gate <- struct{}{}:
		if s.life.Err() != nil {
			<-s.gate
			clean()
			return nil, nil, ErrClosed
		}
		if e := c.Err(); e != nil {
			<-s.gate
			clean()
			return nil, nil, e
		}
		return c, func() { <-s.gate; clean() }, nil
	case <-c.Done():
		clean()
		return nil, nil, c.Err()
	}
}
func (s *Service) Close(ctx context.Context) error {
	s.cancel()
	select {
	case s.gate <- struct{}{}:
		if s.o.Conversation == nil {
			s.history = nil
			s.key = identity{}
		}
		<-s.gate
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) Chat(ctx context.Context, msg string) (string, error) {
	return s.run(ctx, msg, nil, "", nil)
}

type Extract func(context.Context, []string, Settings) (string, error)

func (s *Service) Action(ctx context.Context, ids []string, action string, extract Extract) (string, error) {
	if extract == nil {
		return "", ErrDependency
	}
	return s.run(ctx, "", append([]string(nil), ids...), action, extract)
}
func (s *Service) run(ctx context.Context, msg string, ids []string, action string, extract Extract) (string, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return "", e
	}
	defer done()
	select {
	case s.serial <- struct{}{}:
		defer func() { <-s.serial }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	cfg, e := s.o.Load(ctx)
	if e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	if cfg.Markdown != nil {
		md := *cfg.Markdown
		cfg.Markdown = &md
	}
	v := cfg.API
	k := identity{v.APIKey, v.APIProxy, v.APIBaseURL, v.APIUserAgent, v.APIProvider, v.APIVersion, v.APIModel}
	if k != s.key {
		s.history = nil
		s.key = k
	}
	if v.APIKey == "" {
		s.o.Event(ctx, MissingKey)
		return "", nil
	}
	if v.APIMaxContexts < 1 || v.APIMaxContexts > 64 || v.APITimeout < 1 || v.APITimeout > 600 || v.APIMaxTokens < 0 || v.APITemperature <= 0 || v.APITemperature > 2 || math.IsNaN(v.APITemperature) || math.IsInf(v.APITemperature, 0) {
		return "", ErrConfig
	}
	// The plain message trims its clear command; the action compares it exactly,
	// as before. Action requests otherwise do not read/write dialogue history.
	if extract == nil && strings.TrimSpace(msg) == "Clear context" || extract != nil && action == "Clear context" {
		s.history = nil
		return "", nil
	}
	if extract != nil {
		msg, e = extract(ctx, ids, cfg)
		if e != nil {
			return "", e
		}
		a := strings.TrimSpace(action)
		if a != "" {
			msg = a + ":\n\n" + msg
		}
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	s.o.Event(ctx, Requesting)
	defer s.o.Event(context.WithoutCancel(ctx), Clear)
	if e = ctx.Err(); e != nil {
		return "", e
	}
	client, e := s.o.Open(v)
	if e != nil {
		s.o.Event(ctx, Failed)
		return "", ErrConfig
	}
	if client == nil {
		s.o.Event(ctx, Failed)
		return "", ErrDependency
	}
	defer client.Close()
	var history []string
	if extract == nil {
		history = append([]string(nil), s.history...)
		if len(history) > v.APIMaxContexts {
			history = history[len(history)-v.APIMaxContexts:]
		}
	}
	var buf strings.Builder
	for i := 0; i < v.APIMaxContexts; i++ {
		if e = ctx.Err(); e != nil {
			return strings.TrimSpace(buf.String()), e
		}
		part, err := client.Chat(ctx, msg, append([]string(nil), history...))
		buf.WriteString(part.Text)
		if ctx.Err() != nil {
			return strings.TrimSpace(buf.String()), ctx.Err()
		}
		if err != nil {
			s.o.Event(ctx, Failed)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return strings.TrimSpace(buf.String()), err
			}
			return strings.TrimSpace(buf.String()), ErrProvider
		}
		if part.Stop {
			break
		}
		s.o.Event(ctx, Continuing)
	}
	out := strings.TrimSpace(buf.String())
	if e = ctx.Err(); e != nil {
		return out, e
	}
	if extract == nil && out != "" {
		s.history = append(s.history, msg, out)
		if len(s.history) > 64 {
			s.history = append([]string(nil), s.history[len(s.history)-64:]...)
		}
	}
	return out, nil
}
