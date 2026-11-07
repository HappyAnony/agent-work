// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import "sync"

const legacySQLEventOwnerKey = "wiki-kernel.sql-event-owner"

var legacySQLEventOwnerProvider = struct {
	sync.RWMutex
	fn func() interface{}
}{}

// SetLegacySQLEventOwnerProvider binds compatibility queue admission to an
// opaque workspace token without making sql depend on model. Passing nil
// removes the model reference during runtime teardown.
func SetLegacySQLEventOwnerProvider(fn func() interface{}) {
	legacySQLEventOwnerProvider.Lock()
	legacySQLEventOwnerProvider.fn = fn
	legacySQLEventOwnerProvider.Unlock()
}

func captureLegacySQLEventOwner() interface{} {
	legacySQLEventOwnerProvider.RLock()
	fn := legacySQLEventOwnerProvider.fn
	legacySQLEventOwnerProvider.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// LegacySQLEventContext attaches the opaque token captured at queue admission.
// Existing event fields and routing values are retained.
func LegacySQLEventContext(owner interface{}, values map[string]interface{}) map[string]interface{} {
	if values == nil {
		values = map[string]interface{}{}
	}
	if owner == nil {
		delete(values, legacySQLEventOwnerKey)
	} else {
		values[legacySQLEventOwnerKey] = owner
	}
	return values
}

// LegacySQLEventOwner returns the token for model-side validation.
func LegacySQLEventOwner(values map[string]interface{}) interface{} {
	if values == nil {
		return nil
	}
	return values[legacySQLEventOwnerKey]
}
