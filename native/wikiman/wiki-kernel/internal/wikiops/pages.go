package wikiops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"wiki-kernel/internal/buildinfo"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

var ErrArgument = errors.New("invalid_argument")
var ErrUnauthorized = errors.New("unauthorized")
var ErrStaleSession = errors.New("stale_session")

// PageRequest is shared by argv/stdin and HTTP. apply accepts complete page
// records: creation uses revision 1 and an empty expectedHash; all later edits
// (rename/move/reorder/soft-delete/restore) use the previous hash and revision+1.
// Callers supply timestamps, so retry payloads never change with wall-clock time.
type PageRequest struct {
	ProtocolVersion    int          `json:"protocolVersion"`
	Action             string       `json:"action"`
	StoreID            string       `json:"storeId"`
	PageID             string       `json:"pageId,omitempty"`
	Query              string       `json:"query,omitempty"`
	IncludeDeleted     bool         `json:"includeDeleted,omitempty"`
	Limit              int          `json:"limit,omitempty"`
	Cursor             string       `json:"cursor,omitempty"`
	OperationID        string       `json:"operationId,omitempty"`
	ExpectedTreeDigest string       `json:"expectedTreeDigest,omitempty"`
	Updates            []PageUpdate `json:"updates,omitempty"`
	DryRun             bool         `json:"dryRun,omitempty"`
	Scope              string       `json:"scope,omitempty"`
}
type PageUpdate struct {
	Page              wikistore.Page `json:"page"`
	ExpectedHash      string         `json:"expectedHash"`
	EntryExpectedHash string         `json:"entryExpectedHash,omitempty"`
}
type PageView struct {
	Page      wikistore.Page `json:"page"`
	Hash      string         `json:"hash"`
	Path      string         `json:"path"`
	EntryHash string         `json:"entryHash,omitempty"`
}
type PageResult struct {
	StoreID          string                 `json:"storeId"`
	TreeSourceDigest string                 `json:"treeSourceDigest,omitempty"`
	Pages            []PageView             `json:"pages,omitempty"`
	Total            int                    `json:"total,omitempty"`
	NextCursor       string                 `json:"nextCursor,omitempty"`
	Receipt          *sourcetx.Receipt      `json:"receipt,omitempty"`
	Index            *sourcetx.IndexState   `json:"index,omitempty"`
	Diagnostics      []wikistore.Diagnostic `json:"diagnostics,omitempty"`
}
type pageCursor struct {
	Digest  string `json:"digest"`
	Query   string `json:"query"`
	Deleted bool   `json:"deleted"`
	Offset  int    `json:"offset"`
}

type externalBlockReference struct{ pageID, targetID string }
type blockTargetCount struct {
	documents int
	openable  bool
}
type storeReferenceCheck struct {
	targets  map[string]blockTargetCount
	external []externalBlockReference
}

func (c *storeReferenceCheck) addDocument(pageID string, spans []docast.Span, refs []docast.Reference, openable bool) {
	local := make(map[string]bool, len(spans))
	for _, span := range spans {
		local[span.ID] = true
		target := c.targets[span.ID]
		target.documents++
		target.openable = openable
		c.targets[span.ID] = target
	}
	for _, ref := range refs {
		if !local[ref.TargetID] {
			c.external = append(c.external, externalBlockReference{pageID, ref.TargetID})
		}
	}
}

// DecodePageRequest rejects unknown fields, duplicate keys, trailing input and
// oversized/deep payloads before a CLI handler opens any source/runtime path.
func DecodePageRequest(reader io.Reader) (PageRequest, error) {
	var request PageRequest
	if err := Decode(reader, &request); err != nil {
		return request, err
	}
	return request, request.Validate()
}

func Decode(reader io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return ErrArgument
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 512 {
			return ErrArgument
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
				name, ok := key.(string)
				if !ok || seen[name] {
					return ErrArgument
				}
				seen[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case json.Delim('['):
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case json.Delim('}'), json.Delim(']'):
			return ErrArgument
		}
		return nil
	}
	if err := walk(0); err != nil {
		return ErrArgument
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrArgument
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrArgument
	}
	return nil
}

func (r PageRequest) Validate() error {
	if r.ProtocolVersion != buildinfo.ProtocolVersion || r.StoreID == "" || r.Limit < 0 || r.Limit > 1000 || len(r.Query) > 4096 || len(r.Cursor) > 8192 {
		return ErrArgument
	}
	switch r.Action {
	case "get":
		if r.PageID == "" {
			return ErrArgument
		}
	case "list", "check":
	case "apply":
		if r.OperationID == "" || len(r.Updates) == 0 || len(r.Updates) > 1000 {
			return ErrArgument
		}
	case "index":
		if r.Scope != "all" && r.Scope != "tree" && r.Scope != "content" {
			return ErrArgument
		}
	default:
		return ErrArgument
	}
	// Reject irrelevant options instead of silently ignoring a mistaken write.
	if r.Action != "apply" && (len(r.Updates) != 0 || r.OperationID != "" || r.ExpectedTreeDigest != "" || r.DryRun) {
		return ErrArgument
	}
	if r.Action != "get" && r.PageID != "" {
		return ErrArgument
	}
	if r.Action != "list" && (r.Query != "" || r.IncludeDeleted || r.Limit != 0 || r.Cursor != "") {
		return ErrArgument
	}
	if r.Action != "index" && r.Scope != "" && !(r.Action == "check" && r.Scope == "content") {
		return ErrArgument
	}
	return nil
}

func Pages(ctx context.Context, manager *workspace.WorkspaceManager, req PageRequest) (PageResult, error) {
	result := PageResult{StoreID: req.StoreID}
	if err := req.Validate(); err != nil {
		return result, err
	}
	if req.Action == "index" {
		index, err := manager.Rebuild(ctx, req.StoreID, req.Scope)
		result.Index = &index
		return result, err
	}
	var transaction sourcetx.Request
	err := manager.WithReadContext(ctx, req.StoreID, func(ctx context.Context) error {
		tree, err := manager.Tree(ctx, req.StoreID)
		if err != nil {
			return err
		}
		result.TreeSourceDigest = tree.SourceDigest()
		view := func(page wikistore.Page) (PageView, error) {
			raw, err := manager.SourceFile(ctx, req.StoreID, "pages/"+page.PageID+"/page.json")
			if err != nil {
				return PageView{}, err
			}
			p, _ := tree.Path(page.PageID)
			item := PageView{Page: page, Hash: sourcetx.Hash(raw), Path: p}
			if page.Type == "docx" && page.EntryDocument != nil {
				_, raw, err := manager.DocumentSource(ctx, req.StoreID, page.PageID, page.EntryDocument.BoxID, page.EntryDocument.DocID)
				if err != nil {
					return PageView{}, err
				}
				item.EntryHash = sourcetx.Hash(raw)
			}
			return item, nil
		}
		switch req.Action {
		case "get":
			page, ok := tree.Get(req.PageID)
			if !ok {
				return os.ErrNotExist
			}
			item, err := view(page)
			if err != nil {
				return err
			}
			result.Pages = []PageView{item}
		case "list":
			var pages []wikistore.Page
			stack := tree.Children("", true)
			for len(stack) > 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
				id := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				page, _ := tree.Get(id)
				stack = append(stack, tree.Children(id, true)...)
				if (req.IncludeDeleted || page.DeletedAt == nil) && strings.Contains(strings.ToLower(page.Name), strings.ToLower(req.Query)) {
					pages = append(pages, page)
				}
			}
			sort.Slice(pages, func(i, j int) bool {
				a, _ := tree.Path(pages[i].PageID)
				b, _ := tree.Path(pages[j].PageID)
				if a == b {
					return pages[i].PageID < pages[j].PageID
				}
				return a < b
			})
			offset := 0
			if req.Cursor != "" {
				data, err := base64.RawURLEncoding.DecodeString(req.Cursor)
				if err != nil {
					return sourcetx.ErrCursor
				}
				var c pageCursor
				if json.Unmarshal(data, &c) != nil || c.Digest != tree.SourceDigest() || c.Query != req.Query || c.Deleted != req.IncludeDeleted || c.Offset < 0 || c.Offset > len(pages) {
					return sourcetx.ErrCursor
				}
				offset = c.Offset
			}
			limit := req.Limit
			if limit == 0 {
				limit = 50
			}
			end := min(offset+limit, len(pages))
			result.Total = len(pages)
			result.Pages = []PageView{}
			for _, page := range pages[offset:end] {
				item, err := view(page)
				if err != nil {
					return err
				}
				result.Pages = append(result.Pages, item)
			}
			if end < len(pages) {
				raw, _ := json.Marshal(pageCursor{tree.SourceDigest(), req.Query, req.IncludeDeleted, end})
				result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
			}
		case "check":
			result.Diagnostics = tree.Diagnostics()
			if req.Scope == "content" {
				invalid := false
				refs := storeReferenceCheck{targets: map[string]blockTargetCount{}}
				for stack := tree.Children("", true); len(stack) > 0; {
					pageID := stack[0]
					stack = append(stack[1:], tree.Children(pageID, true)...)
					page, _ := tree.Get(pageID)
					var content ContentResult
					r := ContentRequest{ProtocolVersion: 1, Action: "check", StoreID: req.StoreID, PageID: pageID}
					err := contentScan(ctx, manager, r, &content, page, &refs)
					if err != nil && !errors.Is(err, docast.ErrDocument) {
						return err
					}
					invalid = invalid || err != nil
					for _, diagnostic := range content.Diagnostics {
						result.Diagnostics = append(result.Diagnostics, wikistore.Diagnostic{Code: diagnostic, PageID: pageID, Severity: "error", RelativePath: "pages/" + pageID + "/data"})
					}
				}
				for _, ref := range refs.external {
					code := "unresolved_block_reference:"
					target := refs.targets[ref.targetID]
					switch target.documents {
					case 1:
						if target.openable {
							continue
						}
						code = "unsupported_block_reference_view:"
					case 0:
					default:
						code = "ambiguous_block_reference:"
					}
					invalid = true
					result.Diagnostics = append(result.Diagnostics, wikistore.Diagnostic{
						Code: code + ref.targetID, PageID: ref.pageID, Severity: "error",
						RelativePath: "pages/" + ref.pageID + "/data",
					})
				}
				if invalid {
					return docast.ErrDocument
				}
			}
		case "apply":
			transaction = sourcetx.Request{OperationID: req.OperationID, ExpectedTreeDigest: req.ExpectedTreeDigest}
			seen := map[string]bool{}
			payloadBytes := 0
			appendChange := func(change sourcetx.Change) error {
				payloadBytes += len(change.Data)
				if payloadBytes > 64<<20 {
					return ErrArgument
				}
				transaction.Changes = append(transaction.Changes, change)
				return nil
			}
			for _, update := range req.Updates {
				page := update.Page
				if seen[page.PageID] {
					return ErrArgument
				}
				seen[page.PageID] = true
				raw, err := json.Marshal(page)
				if err != nil {
					return err
				}
				if err := appendChange(sourcetx.Change{Path: "pages/" + page.PageID + "/page.json", ExpectedHash: update.ExpectedHash, Data: raw}); err != nil {
					return err
				}
				if page.Type == "docx" && page.EntryDocument != nil {
					if update.EntryExpectedHash == "" {
						return ErrArgument
					}
					name, raw, err := manager.DocumentSource(ctx, req.StoreID, page.PageID, page.EntryDocument.BoxID, page.EntryDocument.DocID)
					if err != nil {
						return err
					}
					var parser docast.Parser
					if _, _, err := parser.Parse(ctx, raw, page.EntryDocument.DocID); err != nil {
						return err
					}
					var ast map[string]json.RawMessage
					if json.Unmarshal(raw, &ast) != nil || ast == nil {
						return ErrArgument
					}
					var properties map[string]json.RawMessage
					if value, ok := ast["Properties"]; ok && json.Unmarshal(value, &properties) != nil {
						return ErrArgument
					}
					if properties == nil {
						properties = map[string]json.RawMessage{}
					}
					properties["title"], _ = json.Marshal(page.Name)
					ast["Properties"], _ = json.Marshal(properties)
					raw, err = json.Marshal(ast)
					if err != nil {
						return err
					}
					if err := appendChange(sourcetx.Change{Path: name, ExpectedHash: update.EntryExpectedHash, Data: raw}); err != nil {
						return err
					}
				} else if update.EntryExpectedHash != "" {
					return ErrArgument
				}
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if req.Action == "apply" {
		var receipt sourcetx.Receipt
		if req.DryRun {
			receipt, err = manager.Preview(ctx, req.StoreID, transaction)
		} else {
			receipt, err = manager.Apply(ctx, req.StoreID, transaction)
		}
		result.Receipt = &receipt
		result.TreeSourceDigest = receipt.TreeSourceDigest
	}
	return result, err
}

type ServiceError struct {
	Type      string `json:"type"`
	Hint      string `json:"hint"`
	Retryable bool   `json:"retryable"`
}

func ErrorInfo(err error) ServiceError {
	if errors.Is(err, sourcetx.ErrStoreMismatch) {
		err = workspace.ErrIdentity
	}
	if errors.Is(err, ErrConverterUnavailable) || errors.Is(err, ErrConversion) {
		kind := ErrConversion.Error()
		if errors.Is(err, ErrConverterUnavailable) {
			kind = ErrConverterUnavailable.Error()
		}
		return ServiceError{Type: kind, Hint: "Configure trusted local Pandoc for DOCX or the macOS wiki-pdf helper for PDF; native source is unchanged. No tool is downloaded automatically.", Retryable: false}
	}
	if errors.Is(err, ErrExportLimit) {
		return ServiceError{Type: ErrExportLimit.Error(), Hint: "Export exceeds bounded document/asset limits; reduce attachments or export native docj separately.", Retryable: false}
	}
	if errors.Is(err, ErrRichText) {
		return ServiceError{Type: ErrRichText.Error(), Hint: "This native operation cannot be represented losslessly; retain unsaved input and use a supported editor operation.", Retryable: false}
	}
	if errors.Is(err, ErrStaleSession) {
		return ServiceError{Type: "stale_session", Hint: "Refresh the explicit host session; old requests cannot be replayed after release/resume.", Retryable: false}
	}
	typeName := "operation_failed"
	for _, typed := range []error{ErrArgument, ErrUnauthorized, ErrUIEndpoint, docast.ErrDocument, sourcetx.ErrInvalid, sourcetx.ErrConflict, sourcetx.ErrBusy, sourcetx.ErrRecovery, sourcetx.ErrReadOnly, sourcetx.ErrIndex, sourcetx.ErrCursor, sourcetx.ErrIdentity, workspace.ErrIdentity, workspace.ErrClosed, workspace.ErrStale, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, typed) {
			typeName = typed.Error()
			break
		}
	}
	if typeName == "operation_failed" && errors.Is(err, os.ErrNotExist) {
		typeName = "not_found"
	}
	var scan *wikistore.ScanError
	if errors.As(err, &scan) {
		typeName = scan.Type
	}
	hint := "Read the current source version before retrying."
	switch typeName {
	case "invalid_argument", "invalid_transaction":
		hint = "Check the command help and the versioned request schema."
	case "store_busy":
		hint = "Wait for the source operation or save and release the active editor before retrying."
	case "source_conflict":
		hint = "Read the current source and hashes before retrying with a new operationId."
	case "recovery_required":
		hint = "Inspect status and preview recovery before applying it to this source."
	case "index_stale":
		hint = "Check any returned source receipt before rebuilding indexes. A source write may already be committed."
	}
	return ServiceError{Type: typeName, Hint: hint, Retryable: typeName == "store_busy"}
}
