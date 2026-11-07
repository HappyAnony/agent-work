//go:build fts5

package workspace

import (
	"context"
	"testing"
	"wiki-kernel/conf"
)

func TestNativeSyncSettingsUseOwnerConfigurationAndClassifiedPatches(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		s := conf.NewSync()
		if page == pageA {
			s.Mode = 2
		}
		return conf.AppData{Sync: s}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for l, mode := range map[*Lease]int{a: 2, b: 1} {
		p, err := l.PlanNativeSetting(context.Background(), "syncInterval", []byte("40"))
		if err != nil {
			t.Fatal(err)
		}
		if p.Setting.Data.Sync.Mode != mode || string(p.Patches[conf.StorePreference].Set["sync.interval"]) != "40" {
			t.Fatal(p)
		}
		s, err := l.Configuration(context.Background())
		if err != nil || s.Sync.Interval != 30 {
			t.Fatal(s, err)
		}
		p, err = l.PlanNativeSetting(context.Background(), "syncS3", []byte(`{"bucket":"native","secretKey":"private"}`))
		if err != nil {
			t.Fatal(err)
		}
		if string(p.Patches[conf.DevicePreference].Set["sync.s3.secretKey"]) != `"private"` {
			t.Fatal("credentials not device private")
		}
	}
	if digest(t, root) != before {
		t.Fatal("plan wrote source")
	}
}
