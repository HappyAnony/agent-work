package riff

import (
	"errors"
	"io/fs"
)

// ErrStorageReadIncomplete distinguishes failed admission or cleanup from a
// successfully observed missing data file, while retaining underlying causes.
var ErrStorageReadIncomplete = errors.New("flashcard_storage_read_incomplete")

// Storage is explicitly supplied by the owner. Relative names retain the original
// MessagePack format; this package never resolves a workspace or touches a filesystem.
type Storage interface {
	Read(string) ([]byte, error)
	Write(string, []byte) error
}

// A missing file is a default only when every error cause is missing. A joined
// close/cancellation failure must never initialize an apparently healthy deck.
func isStorageMissing(err error) bool {
	if err == nil {
		return false
	}
	if group, ok := err.(interface{ Unwrap() []error }); ok {
		causes := group.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !isStorageMissing(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isStorageMissing(wrapped.Unwrap())
	}
	return errors.Is(err, fs.ErrNotExist)
}
