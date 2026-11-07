// SPDX-License-Identifier: AGPL-3.0-or-later
package sourcetx

import (
	"encoding/json"
	"path"
	"slices"
	"strings"
	"time"
)

type DeletedFile struct {
	Path string `json:"path"` // relative to page data
	Hash string `json:"hash"`
}

type DeletedEffect struct {
	Path       string `json:"path"`
	BeforeHash string `json:"beforeHash"`
	AfterHash  string `json:"afterHash"`
}

type DocumentTombstone struct {
	Version      int             `json:"version"`
	PageID       string          `json:"pageId"`
	BoxID        string          `json:"boxId"`
	DocID        string          `json:"docId"`
	OriginalPath string          `json:"originalPath"`
	DeletedAt    string          `json:"deletedAt"`
	Files        []DeletedFile   `json:"files"`
	Ranks        map[string]int  `json:"ranks,omitempty"`
	Effects      []DeletedEffect `json:"effects,omitempty"`
	// Linked contains sorted box/document identities for an atomic recovery.
	Linked []string `json:"linked,omitempty"`
}

func (t DocumentTombstone) RecordPath() string {
	return "tombstones/documents/" + t.PageID + "/" + t.BoxID + "/" + t.DocID + ".json"
}

func (t DocumentTombstone) ArchivePath(rel string) string {
	return "deleted/documents/" + t.PageID + "/" + t.BoxID + "/" + t.DocID + "/" + strings.TrimPrefix(rel, path.Dir(t.OriginalPath)+"/")
}

func (t DocumentTombstone) EffectPath(rel string) string {
	return "deleted/documents/" + t.PageID + "/" + t.BoxID + "/" + t.DocID + "/effects/" + Hash([]byte(rel))
}

func DecodeDocumentTombstone(raw []byte, name string) (DocumentTombstone, error) {
	var t DocumentTombstone
	if len(raw) > maxJournal || strict(raw, &t) != nil || t.Version != 1 || !sourcePath(name) || t.RecordPath() != name || !strings.HasPrefix(t.OriginalPath, t.BoxID+"/") || path.Base(t.OriginalPath) != t.DocID+".docj" || len(t.Files) == 0 || len(t.Files) > 4999 {
		return DocumentTombstone{}, ErrInvalid
	}
	when, err := time.Parse(time.RFC3339, t.DeletedAt)
	if err != nil || when.UTC().Format(time.RFC3339) != t.DeletedAt {
		return DocumentTombstone{}, ErrInvalid
	}
	for _, part := range strings.Split(path.Dir(t.OriginalPath), "/") {
		if !docID.MatchString(part) {
			return DocumentTombstone{}, ErrInvalid
		}
	}
	prefix := strings.TrimSuffix(t.OriginalPath, ".docj") + "/"
	root := false
	last := ""
	for _, f := range t.Files {
		if f.Path <= last || !hashID.MatchString(f.Hash) ||
			f.Path != t.OriginalPath && !strings.HasPrefix(f.Path, prefix) ||
			!sourcePath("pages/"+t.PageID+"/data/"+f.Path) || !sourcePath(t.ArchivePath(f.Path)) {
			return DocumentTombstone{}, ErrInvalid
		}
		root = root || f.Path == t.OriginalPath
		last = f.Path
	}
	if !root {
		return DocumentTombstone{}, ErrInvalid
	}
	if len(t.Linked) > 0 {
		if len(t.Linked) < 2 || len(t.Linked) > 1000 {
			return DocumentTombstone{}, ErrInvalid
		}
		owner, previous := false, ""
		for _, key := range t.Linked {
			box, doc, ok := strings.Cut(key, "/")
			if !ok || !docID.MatchString(box) || !docID.MatchString(doc) || key <= previous {
				return DocumentTombstone{}, ErrInvalid
			}
			owner = owner || box == t.BoxID && doc == t.DocID
			previous = key
		}
		if !owner || len(t.Effects) > 0 && t.Linked[0] != t.BoxID+"/"+t.DocID {
			return DocumentTombstone{}, ErrInvalid
		}
	}
	for id := range t.Ranks {
		if !docID.MatchString(id) {
			return DocumentTombstone{}, ErrInvalid
		}
	}
	last = ""
	for _, effect := range t.Effects {
		if !validDeletedEffect(t.PageID, last, effect) || effect.Path == t.OriginalPath || strings.HasPrefix(effect.Path, prefix) {
			return DocumentTombstone{}, ErrInvalid
		}
		last = effect.Path
	}
	return t, nil
}

func validDeletedEffect(page, last string, effect DeletedEffect) bool {
	first, _, _ := strings.Cut(effect.Path, "/")
	doc := docID.MatchString(first) && strings.HasSuffix(effect.Path, ".docj")
	av := effect.Path == "storage/av/blocks.msgpack" || strings.HasPrefix(effect.Path, "storage/av/") && strings.Count(effect.Path, "/") == 2 && strings.HasSuffix(effect.Path, ".json") && docID.MatchString(strings.TrimSuffix(path.Base(effect.Path), ".json"))
	return (doc || av) && effect.Path > last && sourcePath("pages/"+page+"/data/"+effect.Path) &&
		hashID.MatchString(effect.BeforeHash) && hashID.MatchString(effect.AfterHash)
}

func validArchiveEffect(live, saved Change, effect DeletedEffect, restoring bool) bool {
	if live.Delete {
		return false
	}
	if restoring {
		return live.ExpectedHash == effect.AfterHash && Hash(live.Data) == effect.BeforeHash &&
			saved.Delete && saved.ExpectedHash == effect.BeforeHash
	}
	return live.ExpectedHash == effect.BeforeHash && Hash(live.Data) == effect.AfterHash &&
		!saved.Delete && saved.ExpectedHash == "" && Hash(saved.Data) == effect.BeforeHash
}

// Archive records and exact file moves form a closed set in a single commit.
func verifyDocumentArchives(request Request, before [][]byte, changes map[string]Change) error {
	paired := map[string]bool{}
	records := make(map[string]DocumentTombstone)
	removing := make(map[string]bool)
	for i, c := range request.Changes {
		if !strings.HasPrefix(c.Path, "tombstones/documents/") {
			continue
		}
		raw := c.Data
		if c.Delete {
			raw = before[i]
		} else if c.ExpectedHash != "" {
			return ErrConflict
		}
		t, e := DecodeDocumentTombstone(raw, c.Path)
		if e != nil {
			return e
		}
		records[c.Path], removing[c.Path] = t, c.Delete
		for _, f := range t.Files {
			name := "pages/" + t.PageID + "/data/" + f.Path
			archive := t.ArchivePath(f.Path)
			active, ok := changes[name]
			saved, found := changes[archive]
			if !ok || !found || paired[archive] {
				return ErrInvalid
			}
			paired[archive] = true
			if c.Delete {
				active, saved = saved, active
			}
			if !active.Delete || active.ExpectedHash != f.Hash || saved.Delete || saved.ExpectedHash != "" || Hash(saved.Data) != f.Hash {
				return ErrInvalid
			}
		}
		for _, effect := range t.Effects {
			name := "pages/" + t.PageID + "/data/" + effect.Path
			archived := t.EffectPath(effect.Path)
			live, ok := changes[name]
			saved, found := changes[archived]
			if !ok || !found || paired[archived] || !validArchiveEffect(live, saved, effect, c.Delete) {
				return ErrInvalid
			}
			paired[archived] = true
		}

	}
	for name, record := range records {
		for _, key := range record.Linked {
			peer := "tombstones/documents/" + record.PageID + "/" + key + ".json"
			tomb, ok := records[peer]
			if !ok || removing[name] != removing[peer] || !slices.Equal(record.Linked, tomb.Linked) {
				return ErrInvalid
			}
		}
	}
	for _, c := range request.Changes {
		if strings.HasPrefix(c.Path, "deleted/documents/") && !paired[c.Path] {
			return ErrInvalid
		}
		if strings.HasPrefix(c.Path, "versions/documents/") {
			if c.Delete || c.ExpectedHash != "" {
				return ErrInvalid
			}
			parts := strings.Split(c.Path, "/")
			if len(parts) == 8 {
				companion := strings.Join(parts[:6], "/") + "/" + parts[4] + ".docj"
				if _, ok := changes[companion]; !ok {
					return ErrInvalid
				}
				var av struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(c.Data, &av) != nil || av.ID+".json" != parts[7] {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}
