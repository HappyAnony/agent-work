// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"net/http"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

func prepareRepoCheckout(ctx context.Context, v conf.AppData, id string, p metadataRepoPaths) (out cloudrepo.CheckoutPlan, err error) {
	if v.Repo == nil || v.System == nil || v.Sync == nil {
		return out, cloudrepo.ErrDependency
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, t := range transports {
			t.CloseIdleConnections()
		}
	}()
	repo, e := newMetadataRepository(ctx, v, p)
	if e != nil {
		return out, cloudrepo.ErrProvider
	}
	defer repo.CloseMetadata()
	plan, e := repo.PrepareCheckoutContext(ctx, id)
	if e != nil {
		return out, errors.Join(cloudrepo.ErrProvider, ctx.Err())
	}
	out.ID = plan.ID
	out.Bytes = plan.Bytes
	out.Files = []cloudrepo.CheckoutFile{}
	for _, f := range plan.Files {
		out.Files = append(out.Files, cloudrepo.CheckoutFile{ID: f.ID, Path: f.Path, SHA256: f.SHA256, Size: f.Size, Updated: f.Updated, Chunks: append([]string{}, f.Chunks...)})
	}
	return out, nil
}

func PrepareRepoCheckoutContext(ctx context.Context, id string) (cloudrepo.CheckoutPlan, error) {
	owner := Conf
	s, e := owner.CloudRepoService()
	if e != nil {
		return cloudrepo.CheckoutPlan{}, e
	}
	out, e := s.PrepareCheckout(ctx, id)
	if errors.Is(e, cloudrepo.ErrKey) {
		e = directoryBusinessError{e, owner.Language(26)}
	}
	return out, e
}
