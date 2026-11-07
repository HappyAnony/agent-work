package wikiops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

var nativeID = regexp.MustCompile(`^\d{14}-[a-z0-9]{7}$`)
var pageIdentity = regexp.MustCompile(`^\d{19}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ContentRequest struct {
	ExpectedAVHashes  map[string]string `json:"expectedAVHashes,omitempty"`
	LegacyHistoryPath string            `json:"legacyHistoryPath,omitempty"`
	VersionHash       string            `json:"versionHash,omitempty"`
	Title             string            `json:"title,omitempty"`
	Markdown          string            `json:"markdown,omitempty"`
	DOM               string            `json:"dom,omitempty"`
	CreationMode      string            `json:"creationMode,omitempty"`
	HPath             string            `json:"hPath,omitempty"`
	Tags              string            `json:"tags,omitempty"`
	ClippingHref      string            `json:"clippingHref,omitempty"`
	WithMath          bool              `json:"withMath,omitempty"`
	ToBoxID           string            `json:"toBoxId,omitempty"`
	Order             []string          `json:"order,omitempty"`
	VersionID         string            `json:"versionId,omitempty"`
	ExpectedPageHash  string            `json:"expectedPageHash,omitempty"`
	NewAssetName      string            `json:"newAssetName,omitempty"`

	ProtocolVersion    int                `json:"protocolVersion"`
	Action             string             `json:"action"`
	StoreID            string             `json:"storeId"`
	PageID             string             `json:"pageId"`
	BoxID              string             `json:"boxId,omitempty"`
	DocID              string             `json:"docId,omitempty"`
	BlockID            string             `json:"blockId,omitempty"`
	ParentID           string             `json:"parentId,omitempty"`
	BeforeID           string             `json:"beforeId,omitempty"`
	ParentDocID        string             `json:"parentDocId,omitempty"`
	TargetDocID        string             `json:"targetDocId,omitempty"`
	TargetExpectedHash string             `json:"targetExpectedHash,omitempty"`
	AssetName          string             `json:"assetName,omitempty"`
	Data               []byte             `json:"data,omitempty"`
	AST                json.RawMessage    `json:"ast,omitempty"`
	ExpectedHash       string             `json:"expectedHash,omitempty"`
	ExpectedTreeDigest string             `json:"expectedTreeDigest,omitempty"`
	OperationID        string             `json:"operationId,omitempty"`
	DryRun             bool               `json:"dryRun,omitempty"`
	Cascade            bool               `json:"cascade,omitempty"`
	PageUpdate         *PageUpdate        `json:"pageUpdate,omitempty"`
	NotebookName       string             `json:"notebookName,omitempty"`
	Format             string             `json:"format,omitempty"`
	Query              string             `json:"query,omitempty"`
	Limit              int                `json:"limit,omitempty"`
	Cursor             string             `json:"cursor,omitempty"`
	Operations         []ProtyleOperation `json:"operations,omitempty"`
	ClipboardAssets    map[string][]byte  `json:"clipboardAssets,omitempty"`
	NativeHistory      bool               `json:"nativeHistory,omitempty"`
	HistoryID          string             `json:"historyId,omitempty"`
	Redo               bool               `json:"redo,omitempty"`
}
type ContentItem struct {
	LegacyHistoryPath string `json:"legacyHistoryPath,omitempty"`
	VersionID         string `json:"versionId,omitempty"`
	Title             string `json:"title,omitempty"`
	DocID             string `json:"docId,omitempty"`
	BoxID             string `json:"boxId,omitempty"`
	Path              string `json:"path"`
	Hash              string `json:"hash"`
	Size              int    `json:"size"`
}
type ContentResult struct {
	VersionAVHashes map[string]string            `json:"versionAVHashes,omitempty"`
	DocID           string                       `json:"docId,omitempty"`
	Path            string                       `json:"path,omitempty"`
	AssetName       string                       `json:"assetName,omitempty"`
	Tombstones      []sourcetx.DocumentTombstone `json:"tombstones,omitempty"`

	StoreID            string             `json:"storeId"`
	PageID             string             `json:"pageId"`
	Hash               string             `json:"hash,omitempty"`
	AST                json.RawMessage    `json:"ast,omitempty"`
	Text               string             `json:"text,omitempty"`
	Data               []byte             `json:"data,omitempty"`
	MediaType          string             `json:"mediaType,omitempty"`
	Items              []ContentItem      `json:"items,omitempty"`
	Total              int                `json:"total"`
	NextCursor         string             `json:"nextCursor,omitempty"`
	Receipt            *sourcetx.Receipt  `json:"receipt,omitempty"`
	HTML               string             `json:"html,omitempty"`
	References         []docast.Reference `json:"references,omitempty"`
	Diagnostics        []string           `json:"diagnostics,omitempty"`
	HistoryID          string             `json:"historyId,omitempty"`
	OriginalIndexFresh bool               `json:"originalIndexFresh,omitempty"`
}

func DecodeContentRequest(reader io.Reader) (ContentRequest, error) {
	var r ContentRequest
	if err := Decode(reader, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}
func (r ContentRequest) Validate() error {
	if r.ProtocolVersion != 1 || r.StoreID == "" || !pageIdentity.MatchString(r.PageID) || r.Limit < 0 || r.Limit > 1000 || len(r.Query) > 4096 || len(r.Cursor) > 8192 || len(r.Data) > 8<<20 || len(r.AST) > 16<<20 {
		return ErrArgument
	}
	allowed := map[string]bool{"protocolVersion": true, "action": true, "storeId": true, "pageId": true}
	add := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	mutating := false
	switch r.Action {
	case "document.get":
		add("boxId", "docId", "format")
	case "document.render", "document.export":
		add("boxId", "docId")
		if r.Action == "document.export" {
			add("format")
			if r.Format != "" && r.Format != "html" && r.Format != "htmlmd" && r.Format != "docx" && r.Format != "pdf" {
				return ErrArgument
			}
		}
	case "document.transactions":
		add("boxId", "docId", "operations", "nativeHistory", "clipboardAssets")
		mutating = true
		if len(r.Operations) == 0 || len(r.Operations) > 1000 {
			return ErrArgument
		}
		if _, e := portableEditOperations(r.Operations); e != nil {
			return e
		}
		if len(r.ClipboardAssets) != 0 && (!r.NativeHistory || !validClipboardAssets(r.ClipboardAssets)) {
			return ErrArgument
		}
	case "document.history":
		add("boxId", "docId", "historyId", "redo")
		mutating = true
		if !editOperationID.MatchString(r.HistoryID) {
			return ErrArgument
		}
	case "block.get":
		add("boxId", "docId", "blockId", "format")
	case "document.list", "search":
		add("query", "limit", "cursor")
	case "check":
	case "asset.list":
		add("query", "limit", "cursor")
	case "asset.get":
		add("assetName")
	case "asset.rename":
		add("assetName", "newAssetName")
		mutating = true
		if r.NewAssetName == "" || len(r.NewAssetName) > 255 {
			return ErrArgument
		}
	case "asset.put", "asset.delete":
		add("assetName")
		if r.Action == "asset.put" {
			add("data")
		}
		mutating = true
	case "document.import":
		add("boxId", "parentDocId", "data")
		mutating = true
		if len(r.Data) == 0 || r.ExpectedHash != "" {
			return ErrArgument
		}
	case "document.put":
		add("boxId", "docId", "ast", "parentDocId", "pageUpdate", "notebookName")
		mutating = true
	case "document.restore":
		add("boxId", "docId", "data")
		mutating = true
		if len(r.Data) == 0 || len(r.Data) > 4<<20 {
			return ErrArgument
		}
	case "document.create":
		add("boxId", "docId", "title", "markdown", "dom", "creationMode", "hPath", "parentDocId")
		mutating = true
		if len(r.Markdown) > 8<<20 || len(r.DOM) > 8<<20 || len(r.HPath) > 4096 || r.Markdown != "" && r.DOM != "" {
			return ErrArgument
		}
		switch r.CreationMode {
		case "":
			if !nativeID.MatchString(r.DocID) {
				return ErrArgument
			}
		case "markdown", "hpath", "daily", "duplicate":
			if r.CreationMode == "markdown" {
				add("tags", "clippingHref", "withMath")
				if len(r.Tags) > 4096 || len(r.ClippingHref) > 4096 {
					return ErrArgument
				}
			}
			if r.CreationMode == "duplicate" && !nativeID.MatchString(r.DocID) {
				return ErrArgument
			}
			if r.CreationMode != "duplicate" && r.DocID != "" && !nativeID.MatchString(r.DocID) {
				return ErrArgument
			}
		default:
			return ErrArgument
		}
	case "document.rename":
		add("boxId", "docId", "title", "expectedPageHash")
		mutating = true
	case "document.delete":
		add("boxId", "docId", "cascade")
		mutating = true
	case "document.move":
		add("boxId", "docId", "parentDocId", "toBoxId", "expectedPageHash")
		mutating = true
	case "document.sort":
		add("boxId", "order")
		mutating = true
		if len(r.Order) == 0 || len(r.Order) > 1000 {
			return ErrArgument
		}
	case "document.deleted":
		add("boxId", "limit", "cursor")
	case "document.recover", "document.version.save", "document.version.apply":
		add("boxId", "docId")
		mutating = true
		if r.Action == "document.version.apply" {
			add("versionId", "expectedPageHash", "legacyHistoryPath", "versionHash", "parentDocId", "expectedAVHashes")
		}
	case "document.version.list":
		add("boxId", "docId", "limit", "cursor")
	case "document.version.get":
		add("boxId", "docId", "versionId", "legacyHistoryPath")
	case "block.create", "block.replace", "block.delete", "block.move":
		add("boxId", "docId", "blockId")
		mutating = true
		if r.Action == "block.create" || r.Action == "block.replace" {
			add("ast")
		}
		if r.Action == "block.create" || r.Action == "block.move" {
			add("parentId", "beforeId")
			if !nativeID.MatchString(r.ParentID) {
				return ErrArgument
			}
		}
		if r.Action == "block.move" {
			add("targetDocId", "targetExpectedHash")
		}
	default:
		return ErrArgument
	}
	if mutating {
		add("expectedHash", "expectedTreeDigest", "operationId", "dryRun")
		if !editOperationID.MatchString(r.OperationID) {
			return ErrArgument
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return ErrArgument
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for key := range fields {
		if !allowed[key] {
			return ErrArgument
		}
	}
	if allowed["boxId"] && !nativeID.MatchString(r.BoxID) || allowed["docId"] && r.Action != "document.create" && !nativeID.MatchString(r.DocID) {
		return ErrArgument
	}
	if allowed["blockId"] && !nativeID.MatchString(r.BlockID) {
		return ErrArgument
	}
	if r.ParentDocID != "" && !nativeID.MatchString(r.ParentDocID) {
		return ErrArgument
	}
	if r.BeforeID != "" && !nativeID.MatchString(r.BeforeID) {
		return ErrArgument
	}
	if r.TargetDocID != "" {
		if !nativeID.MatchString(r.TargetDocID) || r.TargetDocID == r.DocID || !hashPattern.MatchString(r.TargetExpectedHash) {
			return ErrArgument
		}
	} else if r.TargetExpectedHash != "" {
		return ErrArgument
	}
	if r.Action != "document.export" && r.Format != "" && r.Format != "ast" && r.Format != "text" {
		return ErrArgument
	}
	if r.ExpectedHash != "" && !hashPattern.MatchString(r.ExpectedHash) {
		return ErrArgument
	}
	if r.ExpectedTreeDigest != "" && !hashPattern.MatchString(r.ExpectedTreeDigest) {
		return ErrArgument
	}
	if r.ToBoxID != "" && !nativeID.MatchString(r.ToBoxID) || r.ExpectedPageHash != "" && !hashPattern.MatchString(r.ExpectedPageHash) || len(r.Title) > 4096 {
		return ErrArgument
	}
	if r.Action == "document.version.get" || r.Action == "document.version.apply" {
		if (r.VersionID == "") == (r.LegacyHistoryPath == "") {
			return ErrArgument
		}
		if r.VersionID != "" && !editOperationID.MatchString(r.VersionID) {
			return ErrArgument
		}
		if len(r.LegacyHistoryPath) > 4096 || r.LegacyHistoryPath != "" && !fs.ValidPath(r.LegacyHistoryPath) {
			return ErrArgument
		}
		if r.VersionHash != "" && !hashPattern.MatchString(r.VersionHash) || r.Action == "document.version.apply" && r.LegacyHistoryPath != "" && r.VersionHash == "" {
			return ErrArgument
		}
	}
	if len(r.ExpectedAVHashes) > 1000 {
		return ErrArgument
	}
	for id, hash := range r.ExpectedAVHashes {
		if !nativeID.MatchString(id) || !hashPattern.MatchString(hash) {
			return ErrArgument
		}
	}
	if mutating && r.Action != "document.put" && r.Action != "asset.put" && r.Action != "document.create" && r.Action != "document.import" && r.Action != "document.sort" && r.Action != "document.version.apply" && r.ExpectedHash == "" {
		return ErrArgument
	}
	if (r.Action == "document.put" || r.Action == "block.create" || r.Action == "block.replace") && len(r.AST) == 0 {
		return ErrArgument
	}
	if allowed["assetName"] && !validAsset(r.AssetName) {
		return ErrArgument
	}
	return nil
}
func validAsset(name string) bool {
	if !fs.ValidPath(name) || name == "." || len(name) > 1024 || strings.ContainsAny(name, "\\\x00") || strings.HasSuffix(name, ".docj") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

func Content(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest) (ContentResult, error) {
	result := ContentResult{StoreID: r.StoreID, PageID: r.PageID}
	if err := r.Validate(); err != nil {
		return result, err
	}
	if r.Action == "document.transactions" || r.Action == "document.history" {
		return contentOriginalEdit(ctx, m, r)
	}
	if lifecycleAction(r.Action) {
		return contentLifecycle(ctx, m, r)
	}
	// dryRun is not part of the committed operation identity.
	copyRequest := r
	copyRequest.DryRun = false
	canonical, _ := json.Marshal(copyRequest)
	clientHash := sourcetx.Hash(canonical)
	if r.OperationID != "" {
		previous, found, err := m.LookupReceipt(ctx, r.StoreID, r.OperationID, clientHash)
		if err != nil {
			return result, err
		}
		if found {
			if r.DryRun {
				previous.Status = "dry_run_replay"
			}
			result.Receipt = &previous
			return result, nil
		}
	}
	tx := sourcetx.Request{OperationID: r.OperationID, ExpectedTreeDigest: r.ExpectedTreeDigest, ClientHash: clientHash}
	add := func(change sourcetx.Change) error {
		size := len(change.Data)
		for _, c := range tx.Changes {
			size += len(c.Data)
		}
		if size > 64<<20 || len(tx.Changes) >= 10000 {
			return ErrArgument
		}
		tx.Changes = append(tx.Changes, change)
		return nil
	}
	err := m.WithReadContext(ctx, r.StoreID, func(ctx context.Context) error {
		tree, err := m.Tree(ctx, r.StoreID)
		if err != nil {
			return err
		}
		page, exists := tree.Get(r.PageID)
		if !exists && !(r.Action == "document.put" && r.PageUpdate != nil && r.PageUpdate.Page.PageID == r.PageID && r.PageUpdate.ExpectedHash == "") {
			return os.ErrNotExist
		}
		if page.DeletedAt != nil && r.OperationID != "" {
			return ErrArgument
		}
		if r.Action == "document.list" || r.Action == "asset.list" || r.Action == "search" || r.Action == "check" {
			return contentScan(ctx, m, r, &result, page, nil)
		}
		if strings.HasPrefix(r.Action, "asset.") {
			name := "pages/" + r.PageID + "/data/assets/" + r.AssetName
			if r.Action == "asset.get" {
				raw, err := m.SourceFile(ctx, r.StoreID, name)
				result.Data = raw
				result.Hash = sourcetx.Hash(raw)
				return err
			}
			return add(sourcetx.Change{Path: name, ExpectedHash: r.ExpectedHash, Data: r.Data, Delete: r.Action == "asset.delete"})
		}
		name, raw, err := m.DocumentSource(ctx, r.StoreID, r.PageID, r.BoxID, r.DocID)
		if err != nil && !(r.Action == "document.put" && r.ExpectedHash == "" && errors.Is(err, os.ErrNotExist)) {
			return err
		}
		if name == "" {
			name = "pages/" + r.PageID + "/data/" + r.BoxID + "/" + r.DocID + ".docj"
			if r.ParentDocID != "" {
				parent, bytes, err := m.DocumentSource(ctx, r.StoreID, r.PageID, r.BoxID, r.ParentDocID)
				if err != nil {
					return err
				}
				name = strings.TrimSuffix(parent, ".docj") + "/" + r.DocID + ".docj"
				if err := add(sourcetx.Change{Path: parent, ExpectedHash: sourcetx.Hash(bytes), Data: bytes}); err != nil {
					return err
				}
			}
		} else if r.Action == "document.put" && r.ParentDocID != "" {
			return ErrArgument
		}
		if r.Action == "document.get" || r.Action == "block.get" {
			result.Hash = sourcetx.Hash(raw)
			parser := docast.Parser{}
			spans, refs, err := parser.Parse(ctx, raw, r.DocID)
			if err != nil {
				return err
			}
			result.References = refs
			if r.Action == "block.get" {
				found := false
				for _, span := range spans {
					if span.ID == r.BlockID {
						owners := map[string]bool{}
						for _, child := range spans {
							if child.Start >= span.Start && child.End <= span.End {
								owners[child.ID] = true
							}
						}
						result.References = nil
						for _, ref := range refs {
							if owners[ref.SourceID] {
								result.References = append(result.References, ref)
							}
						}
						raw = raw[span.Start:span.End]
						found = true
						break
					}
				}
				if !found {
					return os.ErrNotExist
				}
			}
			if r.Format == "text" {
				result.Text, err = PlainText(ctx, raw)
			} else {
				result.AST = raw
			}
			return err
		}
		if r.Action == "document.render" {
			result.Path = strings.TrimPrefix(name, "pages/"+r.PageID+"/data/")
			result.Hash = sourcetx.Hash(raw)
			result.HTML, err = RenderDocument(ctx, raw, r.DocID)
			return err
		}
		if r.Action == "document.export" {
			result.Hash = sourcetx.Hash(raw)
			result.HTML, err = exportDocument(ctx, m, r, name, raw)
			return err
		}
		if strings.HasPrefix(r.Action, "block.") {
			if r.Action == "block.move" && r.TargetDocID != "" {
				destination, bytes, err := m.DocumentSource(ctx, r.StoreID, r.PageID, r.BoxID, r.TargetDocID)
				if err != nil {
					return err
				}
				sourceData, targetData, err := moveBlockAcrossDocuments(ctx, raw, bytes, r)
				if err != nil {
					return err
				}
				if err := add(sourcetx.Change{Path: name, ExpectedHash: r.ExpectedHash, Data: sourceData}); err != nil {
					return err
				}
				return add(sourcetx.Change{Path: destination, ExpectedHash: r.TargetExpectedHash, Data: targetData})
			}
			next, err := mutateBlock(ctx, raw, r)
			if err != nil {
				return err
			}
			return add(sourcetx.Change{Path: name, ExpectedHash: r.ExpectedHash, Data: next})
		}
		switch r.Action {
		case "document.put", "document.restore":
			if r.Action == "document.restore" {
				r.AST = r.Data
			}
			parser := docast.Parser{}
			if _, _, err := parser.Parse(ctx, r.AST, r.DocID); err != nil {
				return err
			}
			var ast struct {
				Type       string
				Properties map[string]json.RawMessage
			}
			if json.Unmarshal(r.AST, &ast) != nil || ast.Type != "NodeDocument" {
				return ErrArgument
			}
			if r.PageUpdate != nil {
				update := r.PageUpdate
				if update.Page.PageID != r.PageID || update.EntryExpectedHash != "" {
					return ErrArgument
				}
				if update.Page.Type == "docx" && (update.Page.EntryDocument == nil || update.Page.EntryDocument.BoxID != r.BoxID || update.Page.EntryDocument.DocID != r.DocID) {
					return ErrArgument
				}
				page = update.Page
				bytes, _ := json.Marshal(page)
				if err := add(sourcetx.Change{Path: "pages/" + r.PageID + "/page.json", ExpectedHash: update.ExpectedHash, Data: bytes}); err != nil {
					return err
				}
			}
			if page.Type == "docx" && page.EntryDocument != nil && page.EntryDocument.BoxID == r.BoxID && page.EntryDocument.DocID == r.DocID {
				var title string
				if json.Unmarshal(ast.Properties["title"], &title) != nil || title != page.Name {
					return ErrArgument
				}
			}
			if r.NotebookName != "" {
				if len(r.NotebookName) > 1024 {
					return ErrArgument
				}
				conf, _ := json.Marshal(map[string]any{"name": r.NotebookName, "sort": 0, "closed": false})
				if err := add(sourcetx.Change{Path: "pages/" + r.PageID + "/data/" + r.BoxID + "/.wiki/conf.json", Data: conf}); err != nil {
					return err
				}
			}
			return add(sourcetx.Change{Path: name, ExpectedHash: r.ExpectedHash, Data: r.AST})

		}
		return ErrArgument
	})
	if err != nil {
		if r.OperationID != "" {
			previous, found, lookupErr := m.LookupReceipt(ctx, r.StoreID, r.OperationID, clientHash)
			if lookupErr == nil && found {
				if r.DryRun {
					previous.Status = "dry_run_replay"
				}
				result.Receipt = &previous
				return result, nil
			}
		}
		return result, err
	}
	if r.Action == "document.export" && (r.Format == "docx" || r.Format == "pdf") {
		// Conversion uses the captured inert HTML, outside the publication read
		// barrier. It neither re-reads mutable source nor adopts a new save hash.
		html := result.HTML
		result.HTML = ""
		if r.Format == "pdf" {
			result.Data, err = convertPDF(ctx, html)
		} else {
			result.Data, err = convertDocx(ctx, html)
		}
		if err == nil {
			result.MediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
			if r.Format == "pdf" {
				result.MediaType = "application/pdf"
			}
		}
		return result, err
	}
	if r.OperationID != "" {
		var receipt sourcetx.Receipt
		if r.DryRun {
			receipt, err = m.Preview(ctx, r.StoreID, tx)
		} else {
			receipt, err = m.Apply(ctx, r.StoreID, tx)
		}
		result.Receipt = &receipt
	}
	return result, err
}

func contentScan(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest, result *ContentResult, page wikistore.Page, storeRefs *storeReferenceCheck) error {
	live := page.DeletedAt == nil
	files, err := m.SourceFiles(ctx, r.StoreID, r.PageID)
	if err != nil {
		return err
	}
	identity, _ := json.Marshal(struct {
		Files               []sourcetx.FileInfo
		Action, Query, Page string
	}{files, r.Action, r.Query, r.PageID})
	digest := sourcetx.Hash(identity)
	items := []ContentItem{}
	ids := map[string]int{}
	refs := []docast.Reference{}
	for _, file := range files {
		relative := strings.TrimPrefix(file.Path, "pages/"+r.PageID+"/data/")
		asset := strings.HasPrefix(relative, "assets/")
		if r.Action == "asset.list" {
			if asset && strings.Contains(strings.ToLower(relative), strings.ToLower(r.Query)) {
				items = append(items, ContentItem{Path: strings.TrimPrefix(relative, "assets/"), Hash: file.Hash, Size: file.Size})
			}
			continue
		}
		if asset || !strings.HasSuffix(relative, ".docj") {
			continue
		}
		parts := strings.Split(relative, "/")
		if len(parts) < 2 || !nativeID.MatchString(parts[0]) {
			return ErrArgument
		}
		item := ContentItem{DocID: strings.TrimSuffix(path.Base(relative), ".docj"), BoxID: parts[0], Path: relative, Hash: file.Hash, Size: file.Size}
		if r.Action == "document.list" {
			raw, err := m.SourceFile(ctx, r.StoreID, file.Path)
			if err != nil {
				return err
			}
			var doc struct{ Properties map[string]json.RawMessage }
			if json.Unmarshal(raw, &doc) == nil {
				_ = json.Unmarshal(doc.Properties["title"], &item.Title)
			}
		}
		if r.Action == "search" || r.Action == "check" {
			raw, err := m.SourceFile(ctx, r.StoreID, file.Path)
			if err != nil {
				return err
			}
			if sourcetx.Hash(raw) != file.Hash {
				return sourcetx.ErrConflict
			}
			parser := docast.Parser{}
			spans, references, err := parser.Parse(ctx, raw, item.DocID)
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, "invalid_ast:"+relative)
				continue
			}
			for _, span := range spans {
				ids[span.ID]++
			}
			if storeRefs != nil && live {
				openable := page.Type == "wiki" || page.Type == "docx" && page.EntryDocument != nil &&
					page.EntryDocument.BoxID == item.BoxID && page.EntryDocument.DocID == item.DocID
				storeRefs.addDocument(r.PageID, spans, references, openable)
			}
			refs = append(refs, references...)
			if r.Action == "search" {
				text, err := PlainText(ctx, raw)
				if err != nil {
					return err
				}
				var doc struct{ Properties map[string]json.RawMessage }
				_ = json.Unmarshal(raw, &doc)
				var title string
				_ = json.Unmarshal(doc.Properties["title"], &title)
				if !strings.Contains(strings.ToLower(text+"\n"+title), strings.ToLower(r.Query)) {
					continue
				}
			}
		} else if !strings.Contains(strings.ToLower(relative), strings.ToLower(r.Query)) {
			continue
		}
		items = append(items, item)
	}
	if r.Action == "check" {
		for id, count := range ids {
			if count > 1 {
				result.Diagnostics = append(result.Diagnostics, "ambiguous_block_id:"+id)
			}
		}
		if storeRefs == nil && live {
			for _, ref := range refs {
				if ids[ref.TargetID] == 0 {
					result.Diagnostics = append(result.Diagnostics, "unresolved_block_reference:"+ref.TargetID)
				}
			}
		}
		if err := checkLifecycleContent(ctx, m, r, files, result, live); err != nil {
			return err
		}
		sort.Strings(result.Diagnostics)
		result.References = refs
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	offset := 0
	if r.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(r.Cursor)
		if err != nil {
			return sourcetx.ErrCursor
		}
		var c pageCursor
		if json.Unmarshal(raw, &c) != nil || c.Digest != digest || c.Offset < 0 || c.Offset > len(items) {
			return sourcetx.ErrCursor
		}
		offset = c.Offset
	}
	limit := r.Limit
	if limit == 0 {
		limit = 50
	}
	end := min(offset+limit, len(items))
	result.Total = len(items)
	result.Items = items[offset:end]
	if end < len(items) {
		raw, _ := json.Marshal(pageCursor{Digest: digest, Offset: end})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if r.Action == "check" && len(result.Diagnostics) > 0 {
		return docast.ErrDocument
	}
	return nil
}
