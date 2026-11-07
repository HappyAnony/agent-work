package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const workspaceDocumentReadLimit = 16 << 20

type workspaceReadIO struct {
	directory workspaceDirectoryIO
	openFile  func(*WorkspaceContext, string) (*os.File, error)
	readFile  func(string, *os.File, []byte) (int, error)
	closeFile func(string, *os.File) error
}

func (op workspaceReadIO) open(w *WorkspaceContext, name string) (*os.File, error) {
	if op.openFile != nil {
		return op.openFile(w, name)
	}
	return openNativeFile(w, name)
}

func (op workspaceReadIO) read(name string, file *os.File, buffer []byte) (int, error) {
	if op.readFile != nil {
		return op.readFile(name, file, buffer)
	}
	return file.Read(buffer)
}

func (op workspaceReadIO) close(name string, file *os.File) error {
	if op.closeFile != nil {
		return op.closeFile(name, file)
	}
	return file.Close()
}

type workspaceDocumentReadManifest struct {
	root        string
	count       int
	directories []workspaceDirectoryManifest
	found       workspaceDirectoryEntry
}

func workspaceDocumentReadSourceManifest(ctx context.Context, w *WorkspaceContext, boxID, docID string, op workspaceDirectoryIO) (workspaceDocumentReadManifest, error) {
	manifest := workspaceDocumentReadManifest{root: boxID, count: 1}
	var scan func(string) error
	scan = func(dir string) error {
		listing, err := workspaceDirectorySourceManifest(ctx, w, dir, op)
		if err != nil {
			return err
		}
		manifest.directories = append(manifest.directories, listing)
		for _, entry := range listing.entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			manifest.count++
			if manifest.count > 100000 || entry.info.Mode()&fs.ModeSymlink != 0 {
				return ErrDocument
			}
			if entry.info.IsDir() {
				if strings.HasPrefix(path.Base(entry.name), ".") {
					continue
				}
				if err := scan(entry.name); err != nil {
					return err
				}
				continue
			}
			if path.Base(entry.name) != docID+".docj" {
				continue
			}
			if !entry.info.Mode().IsRegular() || manifest.found.name != "" {
				return ErrDocument
			}
			manifest.found = entry
		}
		return nil
	}
	if err := scan(boxID); err != nil {
		return workspaceDocumentReadManifest{}, err
	}
	if manifest.found.name == "" {
		return workspaceDocumentReadManifest{}, ErrDocument
	}
	return manifest, ctx.Err()
}

func sameWorkspaceDocumentReadManifest(a, b workspaceDocumentReadManifest) bool {
	if a.root != b.root || a.count != b.count || len(a.directories) != len(b.directories) || a.found.name != b.found.name || !sameWorkspaceDirectoryInfo(a.found.info, b.found.info) {
		return false
	}
	for i := range a.directories {
		if !sameWorkspaceDirectoryManifest(a.directories[i], b.directories[i]) {
			return false
		}
	}
	return true
}

func confirmedWorkspaceDocumentReadManifest(ctx context.Context, w *WorkspaceContext, boxID, docID string, op workspaceDirectoryIO) (workspaceDocumentReadManifest, error) {
	manifest, err := workspaceDocumentReadSourceManifest(ctx, w, boxID, docID, op)
	if err != nil {
		return workspaceDocumentReadManifest{}, err
	}
	confirmed, err := workspaceDocumentReadSourceManifest(ctx, w, boxID, docID, op)
	if err != nil {
		return workspaceDocumentReadManifest{}, err
	}
	if !sameWorkspaceDocumentReadManifest(manifest, confirmed) {
		return workspaceDocumentReadManifest{}, errWorkspaceDirectorySourceChanged
	}
	return manifest, nil
}

func readWorkspaceDocumentCandidate(ctx context.Context, w *WorkspaceContext, candidate workspaceDirectoryEntry, op workspaceReadIO) (data []byte, result error) {
	if candidate.name == "" || candidate.info == nil || !candidate.info.Mode().IsRegular() || candidate.info.Size() > workspaceDocumentReadLimit {
		return nil, ErrDocument
	}
	file, err := op.open(w, candidate.name)
	if err != nil {
		return nil, err
	}
	defer func() {
		result = errors.Join(result, op.close(candidate.name, file))
		current, statErr := workspaceDirectoryPathInfo(ctx, w, candidate.name, op.directory)
		if statErr != nil || !sameWorkspaceDirectoryInfo(current, candidate.info) {
			result = errors.Join(result, errWorkspaceDirectorySourceChanged, statErr)
		}
		if result != nil {
			data = nil
		}
	}()
	opened, err := file.Stat()
	if err != nil || !sameWorkspaceDirectoryInfo(opened, candidate.info) {
		return nil, errors.Join(errWorkspaceDirectorySourceChanged, err)
	}
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := op.read(candidate.name, file, buffer)
		if n < 0 || n > len(buffer) {
			return nil, ErrDocument
		}
		data = append(data, buffer[:n]...)
		if len(data) > workspaceDocumentReadLimit {
			return nil, ErrDocument
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	closedInfo, err := file.Stat()
	if err != nil || !sameWorkspaceDirectoryInfo(closedInfo, candidate.info) {
		return nil, errors.Join(errWorkspaceDirectorySourceChanged, err)
	}
	return data, ctx.Err()
}

func readSourceWithIO(ctx context.Context, w *WorkspaceContext, boxID, docID string, op workspaceReadIO) (json.RawMessage, string, error) {
	if w.data == nil || !documentID.MatchString(boxID) || !documentID.MatchString(docID) {
		return nil, "", ErrDocument
	}
	manifest, err := confirmedWorkspaceDocumentReadManifest(ctx, w, boxID, docID, op.directory)
	if err != nil {
		return nil, "", err
	}
	data, err := readWorkspaceDocumentCandidate(ctx, w, manifest.found, op)
	if err != nil {
		return nil, "", err
	}
	if !json.Valid(data) {
		return nil, "", ErrDocument
	}
	var identity struct{ ID string }
	if json.Unmarshal(data, &identity) != nil || identity.ID != docID {
		return nil, "", ErrDocument
	}
	final, err := workspaceDocumentReadSourceManifest(ctx, w, boxID, docID, op.directory)
	if err != nil {
		return nil, "", err
	}
	if !sameWorkspaceDocumentReadManifest(manifest, final) {
		return nil, "", errWorkspaceDirectorySourceChanged
	}
	return json.RawMessage(data), strings.TrimPrefix(manifest.found.name, boxID), ctx.Err()
}
