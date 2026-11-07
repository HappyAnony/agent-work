package model

import (
	"context"
	"errors"
	"sync"
	"testing"
	"wiki-kernel/conf"
)

func TestParserContextSettingsSnapshotAndCancellation(t *testing.T) {
	old := Conf
	c := NewAppConf()
	c.Editor = conf.NewEditor()
	c.Export = conf.NewExport()
	Conf = c
	defer func() { Conf = old }()
	ctx := context.Background()
	parser, e := NewLuteContext(ctx)
	if e != nil {
		t.Fatal(e)
	}
	original := parser.ParseOptions.InlineAsterisk
	c.m.Lock()
	c.Editor.Markdown.InlineAsterisk = !c.Editor.Markdown.InlineAsterisk
	c.m.Unlock()
	if parser.ParseOptions.InlineAsterisk != original {
		t.Fatal("borrowed parser options mutated")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 30 {
			c.m.Lock()
			c.Editor.Markdown.InlineAsterisk = !c.Editor.Markdown.InlineAsterisk
			c.m.Unlock()
		}
	}()
	for range 30 {
		if _, e = NewLuteContext(ctx); e != nil {
			t.Fatal(e)
		}
	}
	wg.Wait()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if p, e := NewLuteContext(canceled); !errors.Is(e, context.Canceled) || p != nil {
		t.Fatal(p, e)
	}
}
