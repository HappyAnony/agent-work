//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
)

func TestTransactionReferenceOwnerFourPaths(t *testing.T) {
	c, env, _, _, _ := commitFixture(t)
	paths := []struct {
		name     string
		schedule func(context.Context, ...string) error
	}{
		{"insert", env.insertReferences}, {"update", env.updateReferences}, {"move", env.moveReferences}, {"delete", env.deleteReferences},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			if err := path.schedule(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel lost: %v", err)
			}
			old := Conf
			Conf = &AppConf{}
			err := path.schedule(context.Background(), "foreign")
			Conf = old
			if err == nil {
				t.Fatal("replacement owner accepted")
			}
			if err = path.schedule(context.Background(), path.name); err != nil {
				t.Fatal(err)
			}
			c.repoMu.Lock()
			_, ok := c.repoReferenceCounts[path.name]
			c.repoMu.Unlock()
			if !ok {
				t.Fatal("work not registered to captured owner")
			}
		})
	}
	if err := c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if err := path.schedule(context.Background(), "late"); err == nil {
			t.Fatalf("%s accepted after close", path.name)
		}
	}
}

func TestTransactionReferenceOwnerRequiresBinding(t *testing.T) {
	if err := unboundTransactionReferenceCounts(context.Background(), "id"); err == nil {
		t.Fatal("unbound owner accepted")
	}
}
