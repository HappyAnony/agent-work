// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
)

type attributeViewIdentityEnvironment struct {
	ctx  context.Context
	read func(string) (*av.AttributeView, error)
}

func (tx *Transaction) attributeViewIdentityEnvironment() *attributeViewIdentityEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avCell == nil {
			return nil
		}
		return &attributeViewIdentityEnvironment{ctx: tx.environment.ctx, read: tx.environment.avCell.read}
	}
	ctx := transactionAVContext(tx)
	b := readonlyAVBinding(tx, ctx)
	return &attributeViewIdentityEnvironment{ctx: ctx, read: b.readCandidate}
}
func (tx *Transaction) attributeViewItemIDs(id string, ids []string) (map[string]string, error) {
	return attributeViewIdentitiesWithEnvironment(id, ids, true, tx.attributeViewIdentityEnvironment())
}
func (tx *Transaction) attributeViewBoundBlockIDs(id string, ids []string) (map[string]string, error) {
	return attributeViewIdentitiesWithEnvironment(id, ids, false, tx.attributeViewIdentityEnvironment())
}

func attributeViewIdentitiesWithEnvironment(id string, ids []string, toItems bool, e *attributeViewIdentityEnvironment) (map[string]string, error) {
	if e == nil || e.ctx == nil || e.read == nil {
		return nil, errors.New("av_identity_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, errors.New("invalid_av_identity_id")
	}
	a, err := e.read(id)
	if err != nil {
		return nil, err
	}
	if err = e.ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.ID != id {
		return nil, errors.New("invalid_av_identity_source")
	}
	return a.IdentityMap(e.ctx, ids, toItems)
}

func legacyAttributeViewIdentities(id string, ids []string, toItems bool, contexts ...context.Context) map[string]string {
	ctx := legacyAVContext(contexts)
	b := readonlyAVBinding(nil, ctx)
	ret, err := attributeViewIdentitiesWithEnvironment(id, ids, toItems, &attributeViewIdentityEnvironment{ctx: ctx, read: b.readCandidate})
	if err != nil {
		logging.LogErrorf("get AV [%s] identities failed: %s", id, err)
		return avquery.EmptyIdentityMap(ids)
	}
	return ret
}
