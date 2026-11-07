// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"
)

var legacyServices struct {
	sync.Mutex
	closed  bool
	closers map[string]func(context.Context) error
}

// Explicit compat boot registration only; importing a package starts no service.
func RegisterLegacyServiceCloser(name string, fn func(context.Context) error) error {
	if name == "" || fn == nil {
		return errors.New("legacy_service_unbound")
	}
	legacyServices.Lock()
	defer legacyServices.Unlock()
	if legacyServices.closed {
		return errors.New("legacy_services_closed")
	}
	if legacyServices.closers == nil {
		legacyServices.closers = map[string]func(context.Context) error{}
	}
	if legacyServices.closers[name] == nil {
		legacyServices.closers[name] = fn
	}
	return nil
}
func StopLegacyServices(ctx context.Context) error {
	legacyServices.Lock()
	legacyServices.closed = true
	var fns []func(context.Context) error
	for _, fn := range legacyServices.closers {
		fns = append(fns, fn)
	}
	legacyServices.Unlock()
	var result error
	for _, fn := range fns {
		result = errors.Join(result, fn(ctx))
	}
	return result
}

// Explicitly closed owners must not stay retained by the process registry after
// workspace switching. StopLegacyServices snapshots callbacks before invoking
// them, so removing a finished registration is safe during shutdown too.
func unregisterLegacyServiceCloser(name string) {
	legacyServices.Lock()
	delete(legacyServices.closers, name)
	legacyServices.Unlock()
}
