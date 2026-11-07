// Package wikistore reads the portable Wiki source without booting a server,
// modifying documents, or importing the legacy global workspace runtime.
package wikistore

import (
	"encoding/json"
	"fmt"
)

type Store struct {
	SchemaVersion  int    `json:"schemaVersion"`
	StoreID        string `json:"storeId"`
	Name           string `json:"name"`
	DocumentFormat string `json:"documentFormat"`
	LayoutVersion  int    `json:"layoutVersion"`
}

type EntryDocument struct {
	BoxID string `json:"boxId"`
	DocID string `json:"docId"`
}

type Page struct {
	SchemaVersion int             `json:"schemaVersion"`
	PageID        string          `json:"pageId"`
	Name          string          `json:"name"`
	ParentID      *string         `json:"parentId"`
	Rank          string          `json:"rank"`
	Type          string          `json:"type"`
	Revision      int64           `json:"revision"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
	DeletedAt     *string         `json:"deletedAt"`
	EntryDocument *EntryDocument  `json:"entryDocument"`
	Ext           json.RawMessage `json:"ext"`
}

type Diagnostic struct {
	Code           string   `json:"code"`
	Severity       string   `json:"severity"`
	PageID         string   `json:"pageId,omitempty"`
	RelativePath   string   `json:"relativePath"`
	Field          string   `json:"field,omitempty"`
	RelatedPageIDs []string `json:"relatedPageIds,omitempty"`
}

type ScanError struct {
	Type        string       `json:"type"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("%s: %d source diagnostics", e.Type, len(e.Diagnostics))
}

type TreeSnapshot struct {
	root        string
	store       Store
	digest      string
	storeDigest string
	pages       map[string]Page
	children    map[string][]string
	paths       map[string]string
	diagnostics []Diagnostic
}

func (s *TreeSnapshot) Root() string         { return s.root }
func (s *TreeSnapshot) Store() Store         { return s.store }
func (s *TreeSnapshot) SourceDigest() string { return s.digest }
func (s *TreeSnapshot) StoreDigest() string  { return s.storeDigest }

// Get returns a detached record; callers cannot mutate the published snapshot.
func (s *TreeSnapshot) Get(id string) (Page, bool) {
	p, ok := s.pages[id]
	return clonePage(p), ok
}

func (s *TreeSnapshot) Children(parentID string, includeDeleted bool) []string {
	result := []string{}
	for _, id := range s.children[parentID] {
		if includeDeleted || s.pages[id].DeletedAt == nil {
			result = append(result, id)
		}
	}
	return result
}

func (s *TreeSnapshot) Path(id string) (string, bool) {
	p, ok := s.paths[id]
	return p, ok
}

func (s *TreeSnapshot) Diagnostics() []Diagnostic {
	result := make([]Diagnostic, len(s.diagnostics))
	for i, d := range s.diagnostics {
		result[i] = d
		result[i].RelatedPageIDs = append([]string(nil), d.RelatedPageIDs...)
	}
	return result
}

func clonePage(p Page) Page {
	if p.ParentID != nil {
		value := *p.ParentID
		p.ParentID = &value
	}
	if p.DeletedAt != nil {
		value := *p.DeletedAt
		p.DeletedAt = &value
	}
	if p.EntryDocument != nil {
		value := *p.EntryDocument
		p.EntryDocument = &value
	}
	p.Ext = append(json.RawMessage(nil), p.Ext...)
	return p
}
