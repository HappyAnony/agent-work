// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"github.com/vmihailenco/msgpack/v5"
	context "context"
	errors "errors"
	os "os"
)

// ReadSourceAVIDs shares the original relation lock and projection; reads and
// cancellation belong to the caller, missing storage is an explicit empty set.
func ReadSourceAVIDs(ctx context.Context, id string, read func() ([]byte, error)) ([]string, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	raw, e := read()
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	rows := map[string][]string{}
	if e = msgpack.Unmarshal(raw, &rows); e != nil {
		return nil, e
	}
	return append([]string(nil), rows[id]...), ctx.Err()
}

// MutateRelations retains the original ordered list/deduplication and empty-key
// rules under the original lock. The caller supplies source IO and publication.
func MutateRelations(ctx context.Context, src, dest string, add bool, read func() ([]byte, error), write func([]byte, []byte) error) error {
	if ctx == nil || read == nil || write == nil {
		return errors.New("AV relation dependencies unbound")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	before, e := read()
	missing := os.IsNotExist(e)
	if e != nil && !missing {
		return e
	}
	if missing && !add {
		return nil
	}
	rows := map[string][]string{}
	if !missing {
		if e = msgpack.Unmarshal(before, &rows); e != nil {
			return e
		}
	}
	if rows == nil {
		return errors.New("invalid AV relation map")
	}
	values, exists := rows[dest]
	if !add && (!exists || values == nil) {
		return nil
	}
	var next []string
	if add {
		seen := map[string]bool{}
		for _, id := range append(append([]string(nil), values...), src) {
			if !seen[id] {
				next = append(next, id)
				seen[id] = true
			}
		}
	} else {
		for _, id := range values {
			if id != src {
				next = append(next, id)
			}
		}
	}
	rows[dest] = next
	after, e := msgpack.Marshal(rows)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if missing {
		before = nil
	}
	return write(before, after)
}
