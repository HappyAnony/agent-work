package sourcetx

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

const maxFile = 64 << 20
const maxJournal = 8 << 20

var operationID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var pageID = regexp.MustCompile(`^\d{19}$`)
var docID = regexp.MustCompile(`^\d{14}-[a-z0-9]{7}$`)
var hashID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func sourcePath(name string) bool {
	if path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") || len(name) > 4096 || strings.Count(name, "/") > 64 || strings.HasPrefix(path.Base(name), ".wiki-") {
		return false
	}
	if name == StorePreferencesPath {
		return true
	}
	parts := strings.Split(name, "/")
	if len(parts) >= 5 && parts[1] == "documents" && pageID.MatchString(parts[2]) && docID.MatchString(parts[3]) {
		switch parts[0] {
		case "tombstones":
			return len(parts) == 5 && strings.HasSuffix(parts[4], ".json") && docID.MatchString(strings.TrimSuffix(parts[4], ".json"))
		case "deleted":
			return len(parts) >= 6 && docID.MatchString(parts[4]) &&
				(len(parts) == 6 && parts[5] == parts[4]+".docj" || len(parts) >= 7 && parts[5] == parts[4] ||
					len(parts) == 7 && parts[5] == "effects" && hashID.MatchString(parts[6]))
		case "versions":
			return len(parts) >= 7 && docID.MatchString(parts[4]) && operationID.MatchString(parts[5]) &&
				(len(parts) == 7 && parts[6] == parts[4]+".docj" || len(parts) == 8 && parts[6] == "av" && strings.HasSuffix(parts[7], ".json") && docID.MatchString(strings.TrimSuffix(parts[7], ".json")))
		}
	}
	if len(parts) >= 4 && parts[1] == "notebooks" && pageID.MatchString(parts[2]) {
		switch parts[0] {
		case "tombstones":
			return len(parts) == 4 && strings.HasSuffix(parts[3], ".json") && docID.MatchString(strings.TrimSuffix(parts[3], ".json"))
		case "deleted":
			return len(parts) >= 5 && docID.MatchString(parts[3]) && fs.ValidPath(strings.Join(parts[4:], "/"))
		}
	}
	if len(parts) < 3 || parts[0] != "pages" || !pageID.MatchString(parts[1]) {
		return false
	}
	if len(parts) == 3 {
		return parts[2] == "page.json"
	}
	return parts[2] == "data"
}

func sourcePage(name string) string {
	parts := strings.Split(name, "/")
	if parts[0] == "pages" {
		return parts[1]
	}
	return parts[2] // validated archive path
}

func pageMetadata(name string) bool {
	return strings.HasPrefix(name, "pages/") && strings.Count(name, "/") == 2 && path.Base(name) == "page.json"
}

func readFile(root *os.Root, name string, limit int64) ([]byte, bool, error) {
	if err := safePath(root, name); err != nil {
		return nil, false, err
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, false, ErrInvalid
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, false, ErrInvalid
	}
	return data, true, err
}

func syncDir(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func atomicFile(root *os.Root, name string, data []byte) error {
	if err := safePath(root, name); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(name), ".wiki-tmp-"+rand.Text())
	return atomicAt(root, name, tmp, data)
}

func atomicAt(root *os.Root, name, tmp string, data []byte) error {
	return atomicMode(root, name, tmp, data, 0600)
}

func atomicMode(root *os.Root, name, tmp string, data []byte, mode os.FileMode) error {
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return writeError(err)
	}
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return writeError(err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return writeError(err)
	}
	return syncDir(root, path.Dir(name))
}

func jsonFile(root *os.Root, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return atomicFile(root, name, raw)
}

// strict also rejects duplicate keys; recovery logs are executable input.
func strict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 64 {
			return ErrInvalid
		}
		t, err := decoder.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return ErrInvalid
				}
				seen[s] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			t, err = decoder.Token()
			if err != nil || t != json.Delim('}') {
				return ErrInvalid
			}
		case json.Delim('['):
			for decoder.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			t, err = decoder.Token()
			if err != nil || t != json.Delim(']') {
				return ErrInvalid
			}
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
