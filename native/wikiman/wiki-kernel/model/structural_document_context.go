package model

import (
	"context"
	"errors"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

type structuralDocumentOwner struct {
	owner     *AppConf
	service   *cloudrepo.Service
	data      string
	workspace string
}

func captureStructuralDocumentOwner(ctx context.Context) (*structuralDocumentOwner, error) {
	if ctx == nil {
		return nil, errors.New("structural_document_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := Conf
	if owner == nil {
		return nil, errors.New("structural_document_owner_unbound")
	}
	service, err := owner.CloudRepoService()
	if err != nil {
		return nil, err
	}
	return &structuralDocumentOwner{owner: owner, service: service, data: util.DataDir, workspace: util.WorkspaceDir}, ctx.Err()
}

func (o *structuralDocumentOwner) scheduleReferenceRefresh(ids ...string) error {
	return o.scheduleReferenceRefreshAfter(0, ids...)
}

func (o *structuralDocumentOwner) scheduleReferenceRefreshAfter(delay time.Duration, ids ...string) error {
	if o == nil || o.owner == nil || o.service == nil {
		return errors.New("structural_document_owner_unbound")
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed {
		return errors.New("structural_document_owner_closed")
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	if !o.owner.repoTasks.Go(func(life context.Context) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-life.Done():
				return
			case <-timer.C:
			}
		}
		for _, id := range ids {
			if err := o.service.RefreshReferences(life, id); err != nil {
				if life.Err() == nil {
					logging.LogErrorf("structural document reference refresh failed [id=%s]: %s", id, err)
				}
			}
		}
		if life.Err() == nil && o.owner == Conf && o.data == util.DataDir && o.workspace == util.WorkspaceDir {
			if err := ResetVirtualBlockRefCacheContext(life); err != nil && life.Err() == nil {
				logging.LogErrorf("structural document virtual-reference refresh failed: %s", err)
			}
		}
	}) {
		return errors.New("structural_document_owner_closed")
	}
	return nil
}
