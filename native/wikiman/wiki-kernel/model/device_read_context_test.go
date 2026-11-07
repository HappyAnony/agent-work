//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func TestDeviceReadsOwnChangelogStateAndAdvisoryFilename(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	old := util.WorkingDir
	util.WorkingDir = t.TempDir()
	defer func() { util.WorkingDir = old }()
	base := filepath.Join(util.WorkingDir, "changelogs", "v"+util.Ver)
	os.MkdirAll(base, 0700)
	file := filepath.Join(base, "v"+util.Ver+"_en_US.md")
	os.WriteFile(file, []byte("# Owned changelog"), 0600)
	c.r.owner.ShowChangelog = true
	out, e := ChangelogContext(context.Background())
	if e != nil || out["show"] != true || c.r.owner.ShowChangelog {
		t.Fatal(out, e)
	}
	out, e = ChangelogContext(context.Background())
	if e != nil || out["show"] != false {
		t.Fatal(out, e)
	}
	name := filepath.Join(t.TempDir(), "selected.txt")
	os.WriteFile(name, []byte("existing"), 0600)
	selected, e := UniqueHostFilenameContext(context.Background(), name)
	if e != nil || selected != filepath.Join(filepath.Dir(name), "selected (1).txt") {
		t.Fatal(selected, e)
	}
	if _, e = os.Stat(selected); !os.IsNotExist(e) {
		t.Fatal("advisory probe created file", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = ChangelogContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
