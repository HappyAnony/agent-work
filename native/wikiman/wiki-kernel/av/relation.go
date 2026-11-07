package av

import (
	"context"

	"os"
	"path/filepath"
	"sync"

	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"

	"wiki-kernel/internal/avmodel"
	"wiki-kernel/util"
)

var (
	attributeViewRelationsLock = sync.Mutex{}
)

func GetSrcAvIDs(destAvID string) []string {
	rows, e := ReadSourceAVIDs(context.Background(), destAvID, func() ([]byte, error) {
		return filelock.ReadFile(filepath.Join(util.DataDir, "storage", "av", "relations.msgpack"))
	})
	if e != nil {
		logging.LogErrorf("read attribute view relations failed: %s", e)
	}
	return rows
}

// ReadSourceAVIDs shares the original relation lock and projection; reads and
// cancellation belong to the caller, missing storage is an explicit empty set.
func ReadSourceAVIDs(ctx context.Context, id string, read func() ([]byte, error)) ([]string, error) {
	attributeViewRelationsLock.Lock()
	defer attributeViewRelationsLock.Unlock()
	return avmodel.ReadSourceAVIDs(ctx, id, read)
}

// MutateRelations retains the original ordered list/deduplication and empty-key
// rules under the original lock. The caller supplies source IO and publication.
func MutateRelations(ctx context.Context, src, dest string, add bool, read func() ([]byte, error), write func([]byte, []byte) error) error {
	attributeViewRelationsLock.Lock()
	defer attributeViewRelationsLock.Unlock()
	return avmodel.MutateRelations(ctx, src, dest, add, read, write)
}

func legacyMutateRelation(src, dest string, add bool) {
	file := filepath.Join(util.DataDir, "storage", "av", "relations.msgpack")
	e := MutateRelations(context.Background(), src, dest, add, func() ([]byte, error) { return filelock.ReadFile(file) }, func(_, after []byte) error {
		if add {
			if e := os.MkdirAll(filepath.Dir(file), 0755); e != nil {
				return e
			}
		}
		return filelock.WriteFile(file, after)
	})
	if e != nil {
		logging.LogErrorf("write attribute view relations failed: %s", e)
	}
}
func RemoveAvRel(src, dest string)     { legacyMutateRelation(src, dest, false) }
func UpsertAvBackRel(src, dest string) { legacyMutateRelation(src, dest, true) }
