// SPDX-License-Identifier: AGPL-3.0-or-later
package sourcetx

import (
	"io/fs"
	"strings"
	"time"
)

type NotebookArchive struct {
	Version   int             `json:"version"`
	PageID    string          `json:"pageId"`
	BoxID     string          `json:"boxId"`
	DeletedAt string          `json:"deletedAt"`
	Files     []DeletedFile   `json:"files"` // relative to notebook
	Effects   []DeletedEffect `json:"effects,omitempty"`
}

func (a NotebookArchive) RecordPath() string {
	return "tombstones/notebooks/" + a.PageID + "/" + a.BoxID + ".json"
}

func (a NotebookArchive) ActivePrefix() string {
	return "pages/" + a.PageID + "/data/" + a.BoxID + "/"
}

func (a NotebookArchive) ArchivePath(name string) string {
	return "deleted/notebooks/" + a.PageID + "/" + a.BoxID + "/" + name
}

func (a NotebookArchive) EffectPath(name string) string {
	return a.ArchivePath("effects/" + Hash([]byte(name)))
}

func DecodeNotebookArchive(raw []byte, name string) (NotebookArchive, error) {
	var a NotebookArchive
	if len(raw) > maxJournal || strict(raw, &a) != nil || a.Version != 1 || a.RecordPath() != name ||
		!sourcePath(name) || len(a.Files) == 0 || len(a.Files) > 4999 {
		return NotebookArchive{}, ErrInvalid
	}
	when, err := time.Parse(time.RFC3339, a.DeletedAt)
	if err != nil || when.UTC().Format(time.RFC3339) != a.DeletedAt {
		return NotebookArchive{}, ErrInvalid
	}
	var last string
	configuration := false
	for _, file := range a.Files {
		if file.Path <= last || !fs.ValidPath(file.Path) || !hashID.MatchString(file.Hash) ||
			!sourcePath(a.ActivePrefix()+file.Path) || !sourcePath(a.ArchivePath(file.Path)) {
			return NotebookArchive{}, ErrInvalid
		}
		configuration = configuration || file.Path == ".wiki/conf.json" || file.Path == ".siyuan/conf.json"
		last = file.Path
	}
	if !configuration {
		return NotebookArchive{}, ErrInvalid
	}
	last = ""
	for _, effect := range a.Effects {
		if !validDeletedEffect(a.PageID, last, effect) || strings.HasPrefix(effect.Path, a.BoxID+"/") {
			return NotebookArchive{}, ErrInvalid
		}
		last = effect.Path
	}
	return a, nil
}

func verifyNotebookArchives(request Request, before [][]byte, changes map[string]Change) error {
	paired := map[string]bool{}
	for i, c := range request.Changes {
		if !strings.HasPrefix(c.Path, "tombstones/notebooks/") {
			continue
		}
		raw := c.Data
		if c.Delete {
			raw = before[i]
		} else if c.ExpectedHash != "" {
			return ErrConflict
		}
		a, err := DecodeNotebookArchive(raw, c.Path)
		if err != nil {
			return err
		}
		if request.ReadGuards == nil {
			return ErrInvalid
		}
		activeFiles := map[string]bool{}
		for _, file := range a.Files {
			active, activeFound := changes[a.ActivePrefix()+file.Path]
			saved, savedFound := changes[a.ArchivePath(file.Path)]
			if !activeFound || !savedFound || paired[saved.Path] || active.Delete == c.Delete || saved.Delete != c.Delete {
				return ErrInvalid
			}
			deleted, created := active, saved
			if c.Delete {
				deleted, created = saved, active
			}
			if deleted.ExpectedHash != file.Hash || created.ExpectedHash != "" || Hash(created.Data) != file.Hash {
				return ErrConflict
			}
			activeFiles[active.Path], paired[saved.Path] = true, true
		}
		for _, effect := range a.Effects {
			live, ok := changes["pages/"+a.PageID+"/data/"+effect.Path]
			archived := a.EffectPath(effect.Path)
			saved, found := changes[archived]
			if !ok || !found || paired[archived] || !validArchiveEffect(live, saved, effect, c.Delete) {
				return ErrInvalid
			}
			paired[archived] = true
		}
		for name := range request.ReadGuards.Files {
			if strings.HasPrefix(name, a.ActivePrefix()) && (c.Delete || !activeFiles[name]) {
				return ErrConflict
			}
		}
	}
	for name := range changes {
		if strings.HasPrefix(name, "deleted/notebooks/") && !paired[name] {
			return ErrInvalid
		}
	}
	return nil
}
