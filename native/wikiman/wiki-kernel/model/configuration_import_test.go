package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestConfigurationImportOwnerSaveFailureAndReadOnly(t *testing.T) {
	c, file := cloudFixture(t)
	c.Search = conf.NewSearch()
	c.Lang = "owner"
	c.UserData = "private"
	c.System = conf.NewSystemForDevice("owner", "device", "v")
	c.System.DataDir = "/owner"
	other := Conf
	Conf = &AppConf{AppData: conf.AppData{Lang: "foreign"}}
	defer func() { Conf = other }()
	raw := []byte(`{"search":{"limit":13},"system":{"dataDir":"/foreign","networkProxy":{}},"userData":"foreign","readonly":false}`)
	if e := c.ImportConfiguration(context.Background(), "conf.json", bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	if c.Search.Limit != 13 || c.System.DataDir != "/owner" || c.UserData != "private" || Conf.Lang != "foreign" {
		t.Fatal("borrowed authority")
	}
	before, _ := os.ReadFile(file)
	c.confPath = filepath.Join(file, "impossible")
	if e := c.ImportConfiguration(context.Background(), "f.json", bytes.NewBufferString(`{"search":{"limit":99}}`)); e == nil || c.Search.Limit != 13 {
		t.Fatal(e)
	}
	c.confPath = file
	c.persistenceReadOnly = true
	if e := c.ImportConfiguration(context.Background(), "f.json", bytes.NewReader(raw)); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed import wrote config")
	}
}
func TestConfigurationImportCancellationAndInvalidGroupsRetainState(t *testing.T) {
	c, file := cloudFixture(t)
	c.Search = conf.NewSearch()
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	for _, raw := range []string{`{"editor":null}`, `{"system":{"networkProxy":null}}`, `{"search":{},"search":{}}`} {
		if e := c.ImportConfiguration(context.Background(), "f.json", bytes.NewBufferString(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- c.WithSettingSnapshot(context.Background(), func(conf.AppData) error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	e := c.ImportConfiguration(ctx, "f.json", bytes.NewBufferString(`{"search":{"limit":99}}`))
	close(release)
	if wait := <-done; wait != nil {
		t.Fatal(wait)
	}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled import wrote config")
	}
}
func TestNotebookRepairAuthorityReadonlyAndFailure(t *testing.T) {
	c, _ := cloudFixture(t)
	c.FileTree = conf.NewFileTree()
	dir := t.TempDir()
	root, e := os.OpenRoot(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	id := "20261027093000-miss001"
	os.Mkdir(filepath.Join(dir, id), 0700)
	c.persistenceReadOnly = true
	items, e := c.listNotebooksAt(context.Background(), root, nil, nil)
	if e != nil || len(items) != 1 || !items[0].Closed {
		t.Fatal(items, e)
	}
	target := filepath.Join(dir, id, ".siyuan/conf.json")
	if _, e = os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("readonly repaired source")
	}
	c.persistenceReadOnly = false
	sentinel := errors.New("owner changed")
	if _, e = c.listNotebooksAt(context.Background(), root, func(context.Context) error { return sentinel }, func(context.Context, string) error { return nil }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if _, e = os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("failed guard wrote source")
	}
	count := 0
	items, e = c.listNotebooksAt(context.Background(), root, func(context.Context) error { return nil }, func(ctx context.Context, got string) error {
		if got != id {
			t.Fatal(got)
		}
		if _, e := os.Stat(target); e != nil {
			t.Fatal("effect preceded save", e)
		}
		count++
		return nil
	})
	if e != nil || len(items) != 1 || count != 1 {
		t.Fatal(items, e, count)
	}
}
