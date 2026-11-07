package util

import (
	"context"
	"errors"
	"testing"
)

func TestCapturePandocInitConfigIsImmutable(t *testing.T) {
	originalContainer, originalTemp := Container, TempDir
	originalConf, originalWorking, originalMode := ConfDir, WorkingDir, Mode
	t.Cleanup(func() {
		Container, TempDir = originalContainer, originalTemp
		ConfDir, WorkingDir, Mode = originalConf, originalWorking, originalMode
	})
	Container, TempDir, ConfDir, WorkingDir, Mode = "std", "temp-a", "conf-a", "working-a", "prod"
	captured := CapturePandocInitConfig()
	TempDir, ConfDir, WorkingDir, Mode = "temp-b", "conf-b", "working-b", "dev"
	if captured.TempDir != "temp-a" || captured.ConfDir != "conf-a" || captured.WorkingDir != "working-a" || captured.Mode != "prod" {
		t.Fatalf("captured config changed: %#v", captured)
	}
}

func TestInitPandocContextRejectsCanceledWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := InitPandocContext(ctx, PandocInitConfig{Container: ContainerStd})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if result.Notify {
		t.Fatal("canceled initialization requested a completion notification")
	}
}
