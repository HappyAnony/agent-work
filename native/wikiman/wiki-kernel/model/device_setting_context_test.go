//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/conf"
)

func TestDeviceSettingActualOwnerPersistenceAndRefusal(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	c.r.owner.Api = conf.NewAPI()
	c.r.owner.System = conf.NewSystem()
	if _, err := SetDeviceConfigurationContext(context.Background(), "apiToken", []byte(`{"token":"owned-token"}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(c.r.owner.confPath)
	if err != nil || !bytes.Contains(raw, []byte("owned-token")) {
		t.Fatal(err, string(raw))
	}
	c.r.owner.persistenceReadOnly = true
	if _, err = SetDeviceConfigurationContext(context.Background(), "apiToken", []byte(`{"token":"forbidden"}`)); !errors.Is(err, ErrSettingReadOnly) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, err := SetDeviceConfigurationContext(ctx, "apiToken", []byte(`{"token":"cancelled"}`)); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal(err, string(out))
	}
	after, _ := os.ReadFile(c.r.owner.confPath)
	if !bytes.Equal(raw, after) || c.r.owner.Api.Token != "owned-token" {
		t.Fatal("failed setting changed owner")
	}
	if err = c.r.owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = ConfigurationPresentationContext(context.Background(), false, false); err == nil {
		t.Fatal("closed owner admitted")
	}
}
