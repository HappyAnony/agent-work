// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestSourceReadLimitedBoundsAndEmpty(t *testing.T) {
	s := readFixture(t)
	s.Data.WriteFile("box/empty", []byte{}, 0600)
	for _, q := range []struct {
		name  string
		limit int64
		want  []byte
		fail  bool
	}{{"box/doc", -1, nil, true}, {"box/doc", 0, nil, true}, {"box/doc", 13, nil, true}, {"box/doc", 14, []byte("original bytes"), false}, {"box/doc", 15, []byte("original bytes"), false}, {"box/empty", 0, []byte{}, false}} {
		raw, err := s.ReadLimited(context.Background(), q.name, q.limit)
		if q.fail {
			if raw != nil || !errors.Is(err, ErrSourceReadLimit) {
				t.Fatal(q, raw, err)
			}
		} else if err != nil || raw == nil || !bytes.Equal(raw, q.want) {
			t.Fatal(q, raw, err)
		}
	}
}

func TestSourceReadLimitedPreflightAndLimitPlusOne(t *testing.T) {
	s := readFixture(t)
	opens := 0
	_, err := s.read(context.Background(), "box/doc", sourceReadIO{limited: true, maxBytes: 13, open: func(*os.Root, string) (sourceReadFile, error) { opens++; return nil, errors.New("unexpected open") }})
	if !errors.Is(err, ErrSourceReadLimit) || opens != 0 {
		t.Fatal(err, opens)
	}
	closeErr := errors.New("file close")
	var file *sourceReadFaultFile
	maxChunk := 0
	raw, err := s.read(context.Background(), "box/doc", sourceReadIO{limited: true, maxBytes: 14, open: func(root *os.Root, name string) (sourceReadFile, error) {
		f, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		file = &sourceReadFaultFile{File: f, closeErr: closeErr, read: func(b []byte) (int, error) {
			maxChunk = len(b)
			for i := range b {
				b[i] = 'x'
			}
			return len(b), nil
		}}
		return file, nil
	}})
	if raw != nil || !errors.Is(err, ErrSourceReadLimit) || !errors.Is(err, closeErr) || maxChunk != 15 || file.closes != 1 {
		t.Fatal(raw, err, maxChunk, file)
	}
}
