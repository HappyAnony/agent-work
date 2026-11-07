package indexrepair

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestSourceCopyDirectoryCompleteInventoryBeforeWritesAndSuccessPrefix(t *testing.T) {
	s := sourceFixture(t)
	ctx := context.Background()
	if e := s.Data.MkdirAll("input/sub", 0700); e != nil {
		t.Fatal(e)
	}
	for name, body := range map[string]string{"input/a": "first", "input/b": "second", "input/sub/c": "nested"} {
		if e := s.Data.WriteFile(name, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	sentinel := errors.New("late selected read failed")
	e := s.copyDirectory(ctx, s, "input", "target", sourceCopyIO{read: func(c context.Context, s Source, name string) ([]byte, error) {
		if name == "input/b" {
			return nil, sentinel
		}
		return s.Read(c, name)
	}})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	rewriteExpectBytes(t, s.Data, "target/a", "first")
	if _, e = s.Data.Stat("target/b"); !os.IsNotExist(e) {
		t.Fatal("failed file published", e)
	}
	if e = s.CopyDirectoryFrom(ctx, s, "input", "target"); e != nil {
		t.Fatal(e)
	}
	rewriteExpectBytes(t, s.Data, "target/sub/c", "nested")
	if e = s.Data.Symlink("../a", "input/sub/link"); e != nil {
		t.Fatal(e)
	}
	if e = s.CopyDirectoryFrom(ctx, s, "input", "new-target"); e == nil {
		t.Fatal("linked source accepted")
	}
	if _, e = s.Data.Stat("new-target"); !os.IsNotExist(e) {
		t.Fatal("preflight failure wrote target", e)
	}
}
