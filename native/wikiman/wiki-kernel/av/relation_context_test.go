// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package av

import (
	"context"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"reflect"
	"testing"
)

func TestAVRelationMutationPreservesOriginalOrderAndEmptyKeys(t *testing.T) {
	rows := map[string][]string{"dest": {"a", "b", "a"}, "other": {"keep"}}
	raw, _ := msgpack.Marshal(rows)
	writes := 0
	read := func() ([]byte, error) { return raw, nil }
	write := func(before, after []byte) error {
		if !reflect.DeepEqual(before, raw) {
			t.Fatal("read version not passed")
		}
		raw = after
		writes++
		return nil
	}
	if e := MutateRelations(context.Background(), "b", "dest", true, read, write); e != nil {
		t.Fatal(e)
	}
	msgpack.Unmarshal(raw, &rows)
	if !reflect.DeepEqual(rows["dest"], []string{"a", "b"}) || !reflect.DeepEqual(rows["other"], []string{"keep"}) {
		t.Fatal(rows)
	}
	for _, id := range []string{"a", "b"} {
		if e := MutateRelations(context.Background(), id, "dest", false, read, write); e != nil {
			t.Fatal(e)
		}
	}
	msgpack.Unmarshal(raw, &rows)
	if value, exists := rows["dest"]; !exists || value != nil {
		t.Fatal("original empty key lost", rows)
	}
	count := writes
	MutateRelations(context.Background(), "b", "dest", false, read, write)
	MutateRelations(context.Background(), "b", "unknown", false, read, write)
	if writes != count {
		t.Fatal("nil/absent relation no-op changed")
	}
}
func TestAVRelationMutationMissingCorruptAndWriteFailures(t *testing.T) {
	sentinel := errors.New("fixture IO failure")
	for _, mode := range []string{"missing-remove", "missing-add", "bad", "null", "read-error", "write-error", "canceled-read"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writes := 0
			read := func() ([]byte, error) {
				switch mode {
				case "missing-remove", "missing-add":
					return nil, os.ErrNotExist
				case "bad":
					return []byte("bad msgpack"), nil
				case "null":
					return []byte{0xc0}, nil
				case "read-error":
					return nil, sentinel
				case "canceled-read":
					cancel()
				}
				return msgpack.Marshal(map[string][]string{})
			}
			write := func(before, after []byte) error {
				writes++
				if mode == "missing-add" && before != nil {
					t.Fatal("missing add was not create-only")
				}
				if mode == "write-error" {
					return sentinel
				}
				return nil
			}
			err := MutateRelations(ctx, "src", "dest", mode != "missing-remove", read, write)
			if mode == "missing-remove" {
				if err != nil || writes != 0 {
					t.Fatal(err, writes)
				}
			} else if mode == "missing-add" {
				if err != nil || writes != 1 {
					t.Fatal(err, writes)
				}
			} else {
				if err == nil {
					t.Fatal("failure swallowed")
				}
				if mode != "write-error" && writes != 0 {
					t.Fatal("bad/canceled read wrote data")
				}
			}
		})
	}
}
