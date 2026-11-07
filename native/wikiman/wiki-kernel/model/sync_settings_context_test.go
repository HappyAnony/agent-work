package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"wiki-kernel/conf"
)

func TestSyncSettingActualPersistenceFailureCancellationAndClose(t *testing.T) {
	c, file, _ := repoMetadataFixture(t)
	for _, tt := range []struct{ kind, raw string }{{"syncEnabled", "true"}, {"syncConflict", "true"}, {"syncInterval", "1"}, {"syncMode", "2"}, {"syncProvider", "4"}, {"syncPerception", "false"}, {"syncS3", `{"bucket":"test","endpoint":" example.test ","timeout":1}`}, {"syncWebDAV", `{"endpoint":" example.test "}`}} {
		if _, err := SetSyncSettingContext(context.Background(), tt.kind, []byte(tt.raw)); err != nil {
			t.Fatal(tt.kind, err)
		}
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var saved conf.AppData
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Sync.Enabled || !saved.Sync.GenerateConflictDoc || saved.Sync.Interval != 30 || saved.Sync.Provider != 4 || saved.Sync.S3.Endpoint != "http://example.test/" {
		t.Fatal(saved.Sync)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = SetSyncSettingContext(ctx, "syncEnabled", []byte("false")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.confPath = t.TempDir()
	if _, err = SetSyncSettingContext(context.Background(), "syncEnabled", []byte("false")); err == nil {
		t.Fatal("save failure hidden")
	}
	if !c.Sync.Enabled {
		t.Fatal("failed save published setting")
	}
	c.confPath = file
	c.persistenceReadOnly = true
	if _, err = SetSyncSettingContext(context.Background(), "syncEnabled", []byte("false")); !errors.Is(err, ErrSettingReadOnly) {
		t.Fatal(err)
	}
	c.persistenceReadOnly = false
	if err = c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = SetSyncSettingContext(context.Background(), "syncEnabled", []byte("false")); err == nil {
		t.Fatal("closed owner accepted setting")
	}
}
