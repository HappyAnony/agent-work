package model

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"wiki-kernel/util"
)

func TestAppConfSavesReceiverNotGlobal(t *testing.T) {
	root := t.TempDir()
	a, err := NewAppConfAt(filepath.Join(root, "a.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewAppConfAt(filepath.Join(root, "b.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	oldConf, oldDir, oldReadOnly := Conf, util.ConfDir, util.ReadOnly
	defer func() { Conf, util.ConfDir, util.ReadOnly = oldConf, oldDir, oldReadOnly }()
	Conf, util.ConfDir, util.ReadOnly = b, filepath.Join(root, "never-created"), true
	a.Lang, b.Lang = "owner-a", "owner-b"
	var wg sync.WaitGroup
	for _, c := range []*AppConf{a, b} {
		wg.Add(1)
		go func(c *AppConf) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				if err := c.SaveError(); err != nil {
					t.Error(err)
				}
			}
		}(c)
	}
	wg.Wait()
	for name, expected := range map[string]string{"a.json": "owner-a", "b.json": "owner-b"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result["lang"] != expected {
			t.Fatal(result["lang"])
		}
		if _, leaked := result["confPath"]; leaked {
			t.Fatal("binding serialized")
		}
	}
	if _, err := os.Stat(util.ConfDir); !os.IsNotExist(err) {
		t.Fatal("global directory touched", err)
	}
}

func TestAppConfDetachedReadOnlyAndWriteFailure(t *testing.T) {
	if _, err := NewAppConfAt("", false); err == nil {
		t.Fatal("empty binding")
	}
	if _, err := NewAppConfAt("relative/conf.json", false); err == nil {
		t.Fatal("relative binding")
	}
	var detached AppConf
	if err := detached.SaveError(); !errors.Is(err, ErrUnboundAppConf) {
		t.Fatal(err)
	}
	root := t.TempDir()
	ro, _ := NewAppConfAt(filepath.Join(root, "ro.json"), true)
	// Imported presentation values cannot acquire persistence authority.
	if err := json.Unmarshal([]byte(`{"readonly":false,"confPath":"/invalid","lang":"imported"}`), ro); err != nil {
		t.Fatal(err)
	}
	if err := ro.SaveError(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "ro.json")); !os.IsNotExist(err) {
		t.Fatal("read-only write", err)
	}
	bad, _ := NewAppConfAt(filepath.Join(root, "missing", "conf.json"), false)
	if err := bad.SaveError(); err == nil {
		t.Fatal("write error swallowed")
	}
}
