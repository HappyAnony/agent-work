package wikistore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

var pageIDPattern = regexp.MustCompile(`^[0-9]{19}$`)
var documentIDPattern = regexp.MustCompile(`^[0-9]{14}-[a-z0-9]{7}$`)
var storeIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

var storeFields = []string{"schemaVersion", "storeId", "name", "documentFormat", "layoutVersion"}
var pageFields = []string{"schemaVersion", "pageId", "name", "parentId", "rank", "type", "revision", "createdAt", "updatedAt", "deletedAt", "entryDocument", "ext"}

type Scanner struct {
	MaxPages              int
	MaxMetadataBytes      int64
	MaxTotalMetadataBytes int64
	MaxDepth              int
}

// Scan performs stable, read-only source loading. The caller must eventually
// wrap it in StoreContext's shared source lease (sourcetx). Double collection detects
// instability but is not a lock against uncooperative external writers.
func (scanner Scanner) Scan(ctx context.Context, root string) (*TreeSnapshot, error) {
	scanner = scanner.defaults()
	return scanner.scan(ctx, root)
}

func (scanner Scanner) defaults() Scanner {
	if scanner.MaxPages <= 0 {
		scanner.MaxPages = 100000
	}
	if scanner.MaxMetadataBytes <= 0 {
		scanner.MaxMetadataBytes = 1 << 20
	}
	if scanner.MaxDepth <= 0 {
		scanner.MaxDepth = 10000
	}
	if scanner.MaxTotalMetadataBytes <= 0 {
		scanner.MaxTotalMetadataBytes = 64 << 20
	}
	return scanner
}

func (scanner Scanner) scan(ctx context.Context, root string) (*TreeSnapshot, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, scanFailure("source_unreadable", "store.json", "")
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		before, err := scanner.collect(ctx, realRoot)
		if err != nil {
			return nil, err
		}
		result, validation := scanner.build(ctx, realRoot, before, checkEntry)
		after, err := scanner.collect(ctx, realRoot)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(before, after) {
			continue
		}
		if validation != nil {
			return nil, validation
		}
		return result, nil
	}
	return nil, &ScanError{Type: "source_unstable", Diagnostics: []Diagnostic{{Code: "source_unstable", Severity: "error", RelativePath: "pages"}}}
}

func (scanner Scanner) collect(ctx context.Context, root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	store, err := scanner.readMetadata(root, "store.json")
	if err != nil {
		return nil, err
	}
	files["store.json"] = store
	totalBytes := int64(len(store))
	if totalBytes > scanner.MaxTotalMetadataBytes {
		return nil, scanFailure("metadata_total_size_limit", "store.json", "")
	}
	info, err := os.Lstat(filepath.Join(root, "pages"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, scanFailure("invalid_pages_directory", "pages", "")
	}
	entries, err := os.ReadDir(filepath.Join(root, "pages"))
	if err != nil {
		return nil, scanFailure("source_unreadable", "pages", "")
	}
	if len(entries) > scanner.MaxPages {
		return nil, scanFailure("page_count_limit", "pages", "")
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel := "pages/" + entry.Name()
		if !pageIDPattern.MatchString(entry.Name()) || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, scanFailure("invalid_page_directory", rel, "pageId")
		}
		content, err := scanner.readMetadata(root, rel+"/page.json")
		if err != nil {
			return nil, err
		}
		files[rel+"/page.json"] = content
		totalBytes += int64(len(content))
		if totalBytes > scanner.MaxTotalMetadataBytes {
			return nil, scanFailure("metadata_total_size_limit", "pages", "")
		}
	}
	return files, nil
}

func (scanner Scanner) readMetadata(root, rel string) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return nil, scanFailure("missing_or_unsafe_metadata", rel, "")
	}
	file, err := os.Open(p)
	if err != nil {
		return nil, scanFailure("source_unreadable", rel, "")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, scanner.MaxMetadataBytes+1))
	if err != nil {
		return nil, scanFailure("source_unreadable", rel, "")
	}
	if int64(len(data)) > scanner.MaxMetadataBytes {
		return nil, scanFailure("metadata_size_limit", rel, "")
	}
	return data, nil
}

// Candidate validates a full metadata graph before source publication. The
// transaction owner supplies entry validation against its file overlay, not just
// today's disk. Overrides are metadata only; physical page deletion is forbidden.
func (scanner Scanner) Candidate(ctx context.Context, root string, overrides map[string][]byte, entry func(context.Context, string, Page) error) (*TreeSnapshot, error) {
	scanner = scanner.defaults()
	if entry == nil {
		return nil, errors.New("candidate requires overlay entry validation")
	}
	files, err := scanner.collect(ctx, root)
	if err != nil {
		return nil, err
	}
	for key, data := range overrides {
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[0] != "pages" || !pageIDPattern.MatchString(parts[1]) || parts[2] != "page.json" || len(data) == 0 || int64(len(data)) > scanner.MaxMetadataBytes {
			return nil, scanFailure("invalid_candidate_metadata", key, "")
		}
		files[key] = data
	}
	if len(files)-1 > scanner.MaxPages {
		return nil, scanFailure("page_count_limit", "pages", "")
	}
	var total int64
	for _, data := range files {
		total += int64(len(data))
	}
	if total > scanner.MaxTotalMetadataBytes {
		return nil, scanFailure("metadata_total_size_limit", "pages", "")
	}
	return scanner.build(ctx, root, files, entry)
}

func (scanner Scanner) build(ctx context.Context, root string, files map[string][]byte, entry func(context.Context, string, Page) error) (*TreeSnapshot, error) {
	result := &TreeSnapshot{root: root, pages: map[string]Page{}, children: map[string][]string{}, paths: map[string]string{}}
	var diagnostics []Diagnostic
	add := func(code, rel, field, id string) {
		diagnostics = append(diagnostics, Diagnostic{Code: code, Severity: "error", RelativePath: rel, Field: field, PageID: id})
	}
	if err := decodeMetadata(files["store.json"], &result.store, storeFields); err != nil {
		add("invalid_metadata", "store.json", "", "")
	}
	if result.store.SchemaVersion != 1 || result.store.LayoutVersion != 1 || result.store.DocumentFormat != "docj" {
		add("unsupported_schema", "store.json", "schemaVersion", "")
	}
	if !storeIDPattern.MatchString(result.store.StoreID) || strings.TrimSpace(result.store.Name) == "" {
		add("invalid_store_identity", "store.json", "storeId", "")
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		if key != "store.json" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var p Page
		if err := decodeMetadata(files[key], &p, pageFields); err != nil {
			add("invalid_metadata", key, "", "")
			continue
		}
		if p.SchemaVersion != 1 {
			add("unsupported_schema", key, "schemaVersion", p.PageID)
		}
		if !pageIDPattern.MatchString(p.PageID) || strings.Split(key, "/")[1] != p.PageID {
			add("page_identity_mismatch", key, "pageId", p.PageID)
		}
		if _, exists := result.pages[p.PageID]; exists {
			add("duplicate_page_id", key, "pageId", p.PageID)
		}
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Rank) == "" {
			add("invalid_name_or_rank", key, "rank", p.PageID)
		}
		if p.Type != "wiki" && p.Type != "docx" && p.Type != "table" {
			add("invalid_page_type", key, "type", p.PageID)
		}
		if p.Revision < 1 || p.Revision > 9007199254740991 {
			add("invalid_revision", key, "revision", p.PageID)
		}
		if !utcTimestamp(p.CreatedAt) || !utcTimestamp(p.UpdatedAt) || (p.DeletedAt != nil && !utcTimestamp(*p.DeletedAt)) {
			add("invalid_timestamp", key, "createdAt/updatedAt/deletedAt", p.PageID)
		}
		var ext map[string]json.RawMessage
		if err := json.Unmarshal(p.Ext, &ext); err != nil || ext == nil {
			add("invalid_ext", key, "ext", p.PageID)
		}
		if p.ParentID != nil && !pageIDPattern.MatchString(*p.ParentID) {
			add("invalid_parent_id", key, "parentId", p.PageID)
		}
		if p.EntryDocument != nil {
			if err := entry(ctx, root, p); err != nil {
				add("invalid_entry_document", key, "entryDocument", p.PageID)
			}
		}
		result.pages[p.PageID] = p
	}
	for _, key := range keys {
		id := strings.Split(key, "/")[1]
		p, exists := result.pages[id]
		if !exists {
			continue
		}
		parent := ""
		if p.ParentID != nil {
			parent = *p.ParentID
			ancestor, exists := result.pages[parent]
			if !exists {
				add("missing_parent", key, "parentId", id)
			}
			if exists && p.DeletedAt == nil && ancestor.DeletedAt != nil {
				add("deleted_parent", key, "parentId", id)
			}
		}
		result.children[parent] = append(result.children[parent], id)
	}
	parentKeys := make([]string, 0, len(result.children))
	for parent := range result.children {
		parentKeys = append(parentKeys, parent)
	}
	sort.Strings(parentKeys)
	for _, parent := range parentKeys {
		ids := result.children[parent]
		sort.Slice(ids, func(i, j int) bool {
			a, b := result.pages[ids[i]], result.pages[ids[j]]
			if a.Rank == b.Rank {
				return a.PageID < b.PageID
			}
			return a.Rank < b.Rank
		})
		for i := 1; i < len(ids); i++ {
			if result.pages[ids[i-1]].Rank == result.pages[ids[i]].Rank {
				diagnostics = append(diagnostics, Diagnostic{Code: "duplicate_rank", Severity: "warning", RelativePath: "pages/" + ids[i] + "/page.json", PageID: ids[i], RelatedPageIDs: []string{ids[i-1]}})
			}
		}
	}
	// Follow parent links iteratively; this checks disconnected cycles too.
	finished := map[string]bool{}
	for _, key := range keys {
		id := strings.Split(key, "/")[1]
		chain, seen := []string{}, map[string]bool{}
		for id != "" && !finished[id] {
			if seen[id] {
				add("page_cycle", "pages/"+id+"/page.json", "parentId", id)
				break
			}
			if len(chain) >= scanner.MaxDepth {
				add("tree_depth_limit", key, "parentId", id)
				break
			}
			p, ok := result.pages[id]
			if !ok {
				break
			}
			seen[id] = true
			chain = append(chain, id)
			if p.ParentID == nil {
				break
			}
			id = *p.ParentID
		}
		for _, member := range chain {
			finished[member] = true
		}
	}
	for _, d := range diagnostics {
		if d.Severity == "error" {
			return nil, &ScanError{Type: "invalid_store", Diagnostics: diagnostics}
		}
	}
	queue := append([]string(nil), result.children[""]...)
	depths := map[string]int{}
	for i := 0; i < len(queue); i++ {
		id := queue[i]
		p := result.pages[id]
		prefix := ""
		if p.ParentID != nil {
			prefix = result.paths[*p.ParentID]
			depths[id] = depths[*p.ParentID] + 1
		}
		if depths[id] >= scanner.MaxDepth {
			return nil, scanFailure("tree_depth_limit", "pages/"+id+"/page.json", "parentId")
		}
		result.paths[id] = prefix + "/" + p.Name
		queue = append(queue, result.children[id]...)
	}
	hash := sha256.New()
	for _, key := range keys {
		fileHash := sha256.Sum256(files[key])
		for _, value := range [][]byte{[]byte(key), fileHash[:]} {
			_ = binary.Write(hash, binary.BigEndian, uint64(len(value)))
			_, _ = hash.Write(value)
		}
	}
	result.digest = hex.EncodeToString(hash.Sum(nil))
	storeHash := sha256.Sum256(files["store.json"])
	result.storeDigest = hex.EncodeToString(storeHash[:])
	result.diagnostics = diagnostics
	return result, nil
}

func utcTimestamp(value string) bool {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return false
	}
	_, offset := t.Zone()
	return offset == 0
}

func checkEntry(ctx context.Context, root string, page Page) error {
	entry := page.EntryDocument
	if !documentIDPattern.MatchString(entry.BoxID) || !documentIDPattern.MatchString(entry.DocID) {
		return errors.New("invalid document identity")
	}
	base := filepath.Join(root, "pages", page.PageID, "data")
	for _, p := range []string{base, filepath.Join(base, entry.BoxID)} {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("missing or unsafe data directory")
		}
	}
	matches := 0
	err := filepath.WalkDir(filepath.Join(base, entry.BoxID), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe entry path")
		}
		if d.Name() == entry.DocID+".docj" {
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("unsafe document")
			}
			matches++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if matches != 1 {
		return fmt.Errorf("entry document count %d", matches)
	}
	return nil
}

func scanFailure(code, path, field string) error {
	return &ScanError{Type: "invalid_store", Diagnostics: []Diagnostic{{Code: code, Severity: "error", RelativePath: path, Field: field}}}
}
