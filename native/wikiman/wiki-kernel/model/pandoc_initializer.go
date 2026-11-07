// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"

	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

var errPandocInitializerClosed = errors.New("pandoc_initializer_closed")

type pandocInitFunc func(context.Context, util.PandocInitConfig) (util.PandocInitResult, error)

// pandocInitializer owns the process-once extraction and executable probes.
// It intentionally survives workspace task-owner replacement during hot switch.
type pandocInitializer struct {
	mu      sync.Mutex
	owner   *taskgroup.Group
	run     pandocInitFunc
	started bool
	closed  bool
}

func newPandocInitializer(run pandocInitFunc) *pandocInitializer {
	return &pandocInitializer{run: run}
}

func (p *pandocInitializer) Start(config util.PandocInitConfig, apply func(util.PandocInitResult)) error {
	if p == nil || p.run == nil {
		return errPandocInitializerClosed
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPandocInitializerClosed
	}
	if p.started {
		return nil
	}
	owner := taskgroup.Open()
	if !owner.Go(func(ctx context.Context) {
		result, err := p.run(ctx, config)
		if err != nil && !errors.Is(err, context.Canceled) {
			logging.LogWarnf("initialize pandoc failed: %s", err)
		}
		// A canceled initializer must not publish a stale workspace result.
		if err == nil && ctx.Err() == nil && result.Notify && apply != nil {
			apply(result)
		}
	}) {
		return errPandocInitializerClosed
	}
	p.owner = owner
	p.started = true
	return nil
}

func (p *pandocInitializer) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.closed = true
	owner := p.owner
	p.mu.Unlock()
	if owner == nil {
		return nil
	}
	return owner.Close(ctx)
}

var legacyPandocInitializer = newPandocInitializer(util.InitPandocContext)

func startLegacyPandocInitialization(conf *AppConf) error {
	if conf == nil {
		return errors.New("pandoc_configuration_unbound")
	}
	if err := RegisterLegacyServiceCloser("pandoc-initializer", legacyPandocInitializer.Close); err != nil {
		return err
	}
	config := util.CapturePandocInitConfig()
	return legacyPandocInitializer.Start(config, func(result util.PandocInitResult) {
		logging.LogInfof("pandoc initialized, set pandoc bin to [%s]", result.BinPath)
		conf.m.Lock()
		conf.Export.PandocBin = result.BinPath
		err := conf.saveDataLocked(conf.AppData)
		conf.m.Unlock()
		if err != nil {
			logging.LogErrorf("write pandoc configuration failed: %s", err)
		}
	})
}
