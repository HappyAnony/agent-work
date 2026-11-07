// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/internal/doclife"
)

type documentCreationEnvironment struct {
	gate     *attributeMutationGate
	services doclife.CreationServices
}

func (tx *Transaction) documentCreation(q doclife.CreationRequest) (ret doclife.CreationResult, err error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.documentCreation
		if e == nil || e.gate == nil || tx.environment.ctx == nil {
			return ret, doclife.ErrDependency
		}
		err = e.gate.with(tx.environment.ctx, func() error {
			var err error
			ret, err = doclife.CreatePeripheral(tx.environment.ctx, q, e.services)
			return err
		})
		if err != nil {
			return doclife.CreationResult{}, err
		}
		return
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return runOwnedDocumentCreation(ctx, q)
}
