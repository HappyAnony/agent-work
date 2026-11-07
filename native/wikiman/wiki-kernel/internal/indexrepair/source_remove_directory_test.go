package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"testing"
)

func TestSourceRemoveDirectoryUsesSelectedBaseAndRejectsRoot(t *testing.T) {
	s := sourceFixture(t)
	if e := s.Data.MkdirAll("chosen/sub", 0700); e != nil {
		t.Fatal(e)
	}
	if e := s.Data.WriteFile("chosen/sub/file", []byte("data"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.Data.WriteFile("keep", []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.RemoveDirectory(context.Background(), "chosen"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Data.Stat("chosen"); !errors.Is(e, fs.ErrNotExist) {
		t.Fatal(e)
	}
	rewriteExpectBytes(t, s.Data, "keep", "keep")
	for _, name := range []string{".", "../outside", "chosen/../keep", "/absolute"} {
		if e := s.RemoveDirectory(context.Background(), name); e == nil {
			t.Fatal(name)
		}
	}
}
