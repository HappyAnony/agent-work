package sourcetx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"wiki-kernel/conf"
)

// StorePreferencesPath is shared source, unlike device-local preferences.
const StorePreferencesPath = ".wiki/preferences.json"

const devicePreferencesPath = ".local/conf/device-preferences.json"

func (s *Store) DevicePreferences(ctx context.Context) (out conf.PreferenceDocument, hash string, err error) {
	out = conf.PreferenceDocument{Version: 1, Scope: conf.DevicePreference, Values: map[string]json.RawMessage{}}
	err = s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		raw, exists, err := readFile(root, devicePreferencesPath, 1<<20)
		if err != nil || !exists {
			return err
		}
		out, err = conf.DecodePreferences(raw, conf.DevicePreference)
		if err == nil {
			hash = Hash(raw)
		}
		return err
	})
	return
}

func WorkspacePreferencesPath(page string) (string, error) {
	if !pageID.MatchString(page) {
		return "", ErrInvalid
	}
	return "pages/" + page + "/data/.wiki/preferences.json", nil
}

// Configuration overlays persisted preferences on an explicitly supplied full
// original base while holding one source admission. Missing preferences never
// create a file, synthesize defaults, or import runtime paths/authority.
func (s *Store) Configuration(ctx context.Context, page string, base conf.AppData) (conf.AppData, error) {
	name, err := WorkspacePreferencesPath(page)
	if err != nil {
		return conf.AppData{}, err
	}
	ret, err := base.Clone()
	if err != nil {
		return ret, err
	}
	err = s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		for _, item := range []struct {
			name  string
			scope conf.Scope
		}{{StorePreferencesPath, conf.StorePreference}, {name, conf.WorkspacePreference}, {devicePreferencesPath, conf.DevicePreference}} {
			raw, exists, err := readFile(root, item.name, 1<<20)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}
			doc, err := conf.DecodePreferences(raw, item.scope)
			if err != nil {
				return err
			}
			ret, err = ret.ApplyPreferences(doc)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return ret, err
}

// SaveDevicePreferences is a local single-file CAS under the existing writer's
// session/source locks. It is not a source receipt and is excluded from backups.
func (w *Writer) SaveDevicePreferences(ctx context.Context, expected string, doc conf.PreferenceDocument) (hash string, err error) {
	if doc.Validate(conf.DevicePreference) != nil {
		return "", conf.ErrPreferences
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", ErrInvalid
	}
	err = w.exclusive(ctx, false, func(root *os.Root) error {
		old, exists, err := readFile(root, devicePreferencesPath, 1<<20)
		if err != nil {
			return err
		}
		actual := ""
		if exists {
			actual = Hash(old)
		}
		if actual != expected {
			return ErrConflict
		}
		if err = safePath(root, ".local/conf"); err != nil {
			return err
		}
		if err = root.MkdirAll(".local/conf", 0700); err != nil {
			return writeError(err)
		}
		if err = syncDir(root, ".local"); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = atomicFile(root, devicePreferencesPath, raw); err != nil {
			return err
		}
		stored, exists, err := readFile(root, devicePreferencesPath, 1<<20)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("preferences_missing_after_write")
		}
		hash = Hash(stored)
		return nil
	})
	return
}

func WorkspacePreferencesRequest(page, operation, expected string, doc conf.PreferenceDocument) (Request, error) {
	name, err := WorkspacePreferencesPath(page)
	if err != nil {
		return Request{}, err
	}
	if err = doc.Validate(conf.WorkspacePreference); err != nil {
		return Request{}, err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Request{}, err
	}
	return Request{OperationID: operation, Changes: []Change{{Path: name, ExpectedHash: expected, Data: raw}}}, nil
}

func StorePreferencesRequest(operation, expected string, doc conf.PreferenceDocument) (Request, error) {
	if err := doc.Validate(conf.StorePreference); err != nil {
		return Request{}, err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Request{}, err
	}
	return Request{OperationID: operation, Changes: []Change{{Path: StorePreferencesPath, ExpectedHash: expected, Data: raw}}}, nil
}
