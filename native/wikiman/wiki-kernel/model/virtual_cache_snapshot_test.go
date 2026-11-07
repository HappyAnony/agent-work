//go:build fts5

package model

import (
	"context"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestVirtualCacheReusesHeldSettingSnapshot(t *testing.T) {
	c, _, _, _, _ := conflictFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		held := context.WithValue(ctx, virtualCacheSnapshotKey{}, virtualCacheSnapshot{owner: c, data: data})
		e, err := captureLegacyVirtualCache(held)
		if err != nil {
			return err
		}
		return e.rebuild(held)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVirtualCacheSnapshotRejectsReplacement(t *testing.T) {
	c, _, _, _, _ := conflictFixture(t)
	ctx := context.WithValue(context.Background(), virtualCacheSnapshotKey{}, virtualCacheSnapshot{owner: c, data: c.AppData})
	old := Conf
	Conf = &AppConf{}
	defer func() { Conf = old }()
	if _, err := captureLegacyVirtualCache(ctx); err == nil {
		t.Fatal("snapshot recaptured replacement")
	}
}
