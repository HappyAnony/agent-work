// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	context "context"
	errors "errors"
	copier "github.com/jinzhu/copier"
	av "wiki-kernel/internal/avmodel"
)

func CollectRelatedAttributeViews(ctx context.Context, id string, sources func(string) ([]string, error), enqueue func([]string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ids, err := sources(id)
	if err != nil {
		return err
	}
	var related []string
	for _, source := range ids {
		if source != id {
			related = append(related, source)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = enqueue(related); err != nil {
		return err
	}
	return ctx.Err()
}

// Original finish order: read, regenerate, stage, reload. Scoped reload is a
// candidate event sink, not a WebSocket publication. Legacy keeps best effort.
func FlushRelatedAttributeViews(ctx context.Context, ids []string, read func(string) (*av.AttributeView, error), groups func(*av.AttributeView) error, stage func(*av.AttributeView) error, reload func(string) error, legacy bool) error {
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		a, err := read(id)
		if err != nil || a == nil {
			if legacy {
				continue
			}
			if err != nil {
				return err
			}
			return errors.New("av_cell_related_source_not_found")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = groups(a); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = stage(a); err != nil && !legacy {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = reload(id); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func CloneAttributeViewManagementView(v *av.View) (*av.View, error) {
	ret := &av.View{}
	err := copier.CopyWithOption(ret, v, copier.Option{DeepCopy: true})
	return ret, err
}

func SelectAttributeViewNearItem(c av.Collection, previous string) av.Item {
	if c == nil {
		return nil
	}
	items := c.GetItems()
	if previous == "" {
		if len(items) > 0 {
			return items[0]
		}
		return nil
	}
	for _, item := range items {
		if item.GetID() == previous {
			return item
		}
	}
	return nil
}

func CloneAttributeViewOrderKey(key *av.Key) (*av.Key, error) {
	copy := &av.Key{}
	if err := copier.CopyWithOption(copy, key, copier.Option{DeepCopy: true}); err != nil {
		return nil, err
	}
	return copy, nil
}
