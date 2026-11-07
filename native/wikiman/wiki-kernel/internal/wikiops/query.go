// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/internal/workspacefile"
)

type QueryRequest struct {
	ProtocolVersion int             `json:"protocolVersion"`
	StoreID         string          `json:"storeId"`
	PageID          string          `json:"pageId"`
	Action          string          `json:"action"`
	Options         json.RawMessage `json:"options,omitempty"`
}
type QueryResult struct {
	StoreID    string `json:"storeId"`
	PageID     string `json:"pageId"`
	Action     string `json:"action"`
	SourceHash string `json:"sourceHash"`
	Value      any    `json:"value"`
}
type QueryAction struct {
	Action   string `json:"action"`
	Original string `json:"original"`
	Service  string `json:"service"`
}
type queryCall func(context.Context, *workspace.Lease) (any, error)
type queryEntry struct {
	QueryAction
	prepare func(json.RawMessage) (queryCall, error)
}

func DecodeQueryRequest(reader io.Reader) (QueryRequest, error) {
	var q QueryRequest
	if err := Decode(reader, &q); err != nil {
		return q, err
	}
	return q, q.Validate()
}
func (q QueryRequest) Validate() error { _, err := q.prepare(); return err }
func (q QueryRequest) prepare() (queryCall, error) {
	if q.ProtocolVersion != 1 || q.StoreID == "" || !pageIdentity.MatchString(q.PageID) || len(q.Options) > 1<<20 {
		return nil, ErrArgument
	}
	entry, ok := readQueries[q.Action]
	if !ok {
		return nil, ErrArgument
	}
	options := bytes.TrimSpace(q.Options)
	if len(options) == 0 {
		options = []byte("{}")
	}
	if options[0] != '{' {
		return nil, ErrArgument
	}
	return entry.prepare(options)
}

// QueryActions is the authoritative read registry and original-to-service map.
func QueryActions() []QueryAction {
	out := make([]QueryAction, 0, len(readQueries))
	for _, e := range readQueries {
		out = append(out, e.QueryAction)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}
func readQuery[T any](name, original, service string, run func(context.Context, *workspace.Lease, T) (any, error)) queryEntry {
	return queryEntry{QueryAction: QueryAction{name, original, service}, prepare: func(raw json.RawMessage) (queryCall, error) {
		var q T
		if err := Decode(bytes.NewReader(raw), &q); err != nil {
			return nil, err
		}
		if v, ok := any(q).(interface{ ValidateQuery() error }); ok {
			if e := v.ValidateQuery(); e != nil {
				return nil, e
			}
		}
		if !boundedReadOptions(reflect.ValueOf(q), "") {
			return nil, ErrArgument
		}
		return func(ctx context.Context, l *workspace.Lease) (any, error) { return run(ctx, l, q) }, nil
	}}
}
func boundedReadOptions(v reflect.Value, name string) bool {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !boundedReadOptions(v.Field(i), v.Type().Field(i).Name) {
				return false
			}
		}
	case reflect.String:
		if name == "AssetName" && !validAsset(v.String()) {
			return false
		}
		if v.Len() > 65536 {
			return false
		}
		if strings.EqualFold(name, "sql") && v.String() != "" {
			return indexdb.ValidateReadSQL(v.String()) == nil
		}
	case reflect.Slice:
		if v.Len() > 1000 {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !boundedReadOptions(v.Index(i), name) {
				return false
			}
		}
	case reflect.Map:
		if v.Len() > 1000 {
			return false
		}
		for _, key := range v.MapKeys() {
			if !boundedReadOptions(v.MapIndex(key), name) {
				return false
			}
		}
	case reflect.Int:
		if v.Int() > 1000000 || v.Int() < -1 {
			return false
		}
	}
	return true
}

// Query composes the inherited reads once. Both transports consume this exact
// versioned request/result. No source mutation, repair, listener or legacy boot.
func Query(ctx context.Context, m *workspace.WorkspaceManager, q QueryRequest) (out QueryResult, err error) {
	run, err := q.prepare()
	if err != nil {
		return out, err
	}
	if m == nil {
		return out, ErrArgument
	}
	err = m.WithReadContext(ctx, q.StoreID, func(c context.Context) error {
		l, e := m.Acquire(c, q.StoreID, q.PageID)
		if e != nil {
			return e
		}
		defer l.Release()
		var projection workspace.ReadProjection
		if queryNeedsIndex(q.Action) {
			projection, e = l.PrepareNativeReads(c, q.Action == "index.rebuild")
		} else {
			projection, e = l.NativeReadSourceState(c)
		}
		if e != nil {
			return e
		}
		if q.Action == "assets.search" || q.Action == "assets.content" {
			if e = l.PrepareNativeAssetReads(c, projection.SourceHash); e != nil {
				return e
			}
		}
		var value any
		if q.Action == "index.rebuild" {
			value = projection
		} else {
			value, e = run(c, l)
			if e != nil {
				return e
			}
		}
		if e = l.VerifyNativeReadSource(c, projection.SourceHash); e != nil {
			return e
		}
		out = QueryResult{q.StoreID, q.PageID, q.Action, projection.SourceHash, value}
		return nil
	})
	if err != nil {
		return QueryResult{}, err
	}
	return out, nil
}

type readID struct{ ID string }
type readIDs struct{ IDs []string }
type readKey struct{ Keyword string }
type readAV struct {
	ID, BlockID, ViewID, Query, Kind string
	IDs                              []string
	Page, PageSize                   int
	Groups                           map[string]workspace.NativeAVPaging
}
type readAsset struct {
	Keyword    string
	Extensions []string
}
type readGraph struct{ ID, Query string }
type readDOM struct {
	IDs   []string
	Embed bool
}
type readOutline struct {
	ID      string
	Preview bool
}
type readInfo struct {
	IDs                        []string
	References, AttributeViews bool
}
type readSearch struct {
	Request indexdb.BlockSearchRequest
	GroupBy int
}
type readContent struct {
	ID, Query string
	Method    int
}
type readTags struct{ IgnoreLimit bool }
type readSpans struct {
	Kind, Value string
	Limit       int
}
type readEmpty struct{}

var readQueries = map[string]queryEntry{
	"document.view": readQuery("document.view", "GetDoc", "Lease.NativeGetDoc", func(ctx context.Context, l *workspace.Lease, q docview.Request) (any, error) {
		return l.NativeGetDoc(ctx, q)
	}),
	"document.search": readQuery("document.search", "SearchDocs", "Lease.NativeSearchDocuments", func(ctx context.Context, l *workspace.Lease, q docquery.Request) (any, error) {
		return l.NativeSearchDocuments(ctx, q, nil)
	}),
	"document.tree": readQuery("document.tree", "ListDocsByPath", "Lease.NativeListDocumentTree", func(ctx context.Context, l *workspace.Lease, q doctree.Request) (any, error) {
		files, total, e := l.NativeListDocumentTree(ctx, q, nil)
		return struct {
			Files any `json:"files"`
			Total int `json:"total"`
		}{files, total}, e
	}),
	"document.paths": readQuery("document.paths", "GetHPathByPath/GetPathByID/GetIDsByHPath/IsDocPathExist", "Lease.NativeDocumentPaths", func(ctx context.Context, l *workspace.Lease, q docquery.PathRequest) (any, error) {
		return l.NativeDocumentPaths(ctx, q)
	}),
	"document.info": readQuery("document.info", "GetDocInfo", "Lease.NativeDocInfo", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeDocInfo(ctx, q.ID)
	}),
	"document.infos": readQuery("document.infos", "GetDocInfo/GetDocsInfo", "Lease.NativeDocsInfo", func(ctx context.Context, l *workspace.Lease, q readInfo) (any, error) {
		return l.NativeDocsInfo(ctx, q.IDs, q.References, q.AttributeViews)
	}),
	"document.statistics": readQuery("document.statistics", "GetTreeStat/GetBlocksWordCount/GetContentWordCount", "Lease.NativeDocumentStatistics", func(ctx context.Context, l *workspace.Lease, q docquery.StatRequest) (any, error) {
		return l.NativeDocumentStatistics(ctx, q)
	}),
	"block.read": readQuery("block.read", "ReadBlocksContext/GetChildBlocks/GetHeadingChildrenDOM/GetBlockKramdown", "Lease.NativeReadBlocks", func(ctx context.Context, l *workspace.Lease, q docquery.BlocksRequest) (any, error) {
		return l.NativeReadBlocks(ctx, q)
	}),
	"block.dom": readQuery("block.dom", "GetBlockDOM/GetBlockDOMs/GetBlockDOMWithEmbed", "Lease.NativeBlockDOMs", func(ctx context.Context, l *workspace.Lease, q readDOM) (any, error) {
		return l.NativeBlockDOMs(ctx, q.IDs, q.Embed)
	}),
	"block.position": readQuery("block.position", "GetBlockIndex/GetBlocksIndexes/GetBlockBreadcrumb/GetDOMText", "Lease.NativeBlockPosition", func(ctx context.Context, l *workspace.Lease, q docquery.PositionRequest) (any, error) {
		return l.NativeBlockPosition(ctx, q)
	}),
	"block.relations": readQuery("block.relations", "GetBlockTreeInfos/GetBlockSiblingID/GetBlockRelevantIDs", "Lease.NativeBlockRelations", func(ctx context.Context, l *workspace.Lease, q docquery.RelationRequest) (any, error) {
		return l.NativeBlockRelations(ctx, q)
	}),
	"block.fold": readQuery("block.fold", "GetUnfoldedParentID/IsBlockFolded", "Lease.NativeBlockFold", func(ctx context.Context, l *workspace.Lease, q docquery.FoldRequest) (any, error) {
		return l.NativeBlockFold(ctx, q)
	}),
	"block.location": readQuery("block.location", "GetBlockInfo/GetBlockDefIDByRefID", "Lease.NativeBlockLocation", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeBlockLocation(ctx, q.ID)
	}),
	"block.attributes": readQuery("block.attributes", "GetBlockAttrs", "Lease.ReadNativeBlockAttributes", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.ReadNativeBlockAttributes(ctx, q.ID)
	}),
	"outline": readQuery("outline", "Outline", "Lease.NativeOutline", func(ctx context.Context, l *workspace.Lease, q readOutline) (any, error) {
		return l.NativeOutline(ctx, q.ID, q.Preview)
	}),
	"references.search": readQuery("references.search", "SearchRefBlock", "Lease.SearchNativeReferences", func(ctx context.Context, l *workspace.Lease, q blockview.ReferenceSearchRequest) (any, error) {
		return l.SearchNativeReferences(ctx, q)
	}),
	"references.backlinks": readQuery("references.backlinks", "GetBacklink/GetBacklink2/GetBacklinkDoc/GetBackmentionDoc", "Lease.NativeBacklinks", func(ctx context.Context, l *workspace.Lease, q blockview.BacklinkRequest) (any, error) {
		return l.NativeBacklinks(ctx, q)
	}),
	"references.definitions": readQuery("references.definitions", "GetBlockRefsByDefID", "Lease.NativeRefsCacheByDefinition", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeRefsCacheByDefinition(ctx, q.ID)
	}),
	"references.text": readQuery("references.text", "GetRefText", "Lease.NativeRefText", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeRefText(ctx, q.ID)
	}),
	"references.check": readQuery("references.check", "CheckBlockRef", "Lease.NativeCheckBlockRef", func(ctx context.Context, l *workspace.Lease, q readIDs) (any, error) {
		return l.NativeCheckBlockRef(ctx, q.IDs)
	}),
	"references.embeds": readQuery("references.embeds", "GetBlockDOMWithEmbed/SQL embeds", "Lease.NativeEmbeds", func(ctx context.Context, l *workspace.Lease, q blockview.EmbedRequest) (any, error) {
		return l.NativeEmbeds(ctx, q)
	}),
	"graph": readQuery("graph", "BuildGraph/BuildTreeGraph", "Lease.NativeGraph", func(ctx context.Context, l *workspace.Lease, q readGraph) (any, error) {
		return l.NativeGraph(ctx, q.ID, q.Query)
	}),
	"search": readQuery("search", "FullTextSearchBlock/FullTextSearchRefBlock", "Lease.SearchNativeBlockView", func(ctx context.Context, l *workspace.Lease, q readSearch) (any, error) {
		return l.SearchNativeBlockView(ctx, q.Request, q.GroupBy)
	}),
	"sql": readQuery("sql", "QuerySQL", "Lease.NativeSQLQuery", func(ctx context.Context, l *workspace.Lease, q struct{ SQL string }) (any, error) {
		return l.NativeSQLQuery(ctx, q.SQL)
	}),
	"tags.search": readQuery("tags.search", "SearchTag", "Lease.SearchNativeTags", func(ctx context.Context, l *workspace.Lease, q readKey) (any, error) {
		return l.SearchNativeTags(ctx, q.Keyword)
	}),
	"tags.tree": readQuery("tags.tree", "BuildTags", "Lease.BuildNativeTags", func(ctx context.Context, l *workspace.Lease, q readTags) (any, error) {
		return l.BuildNativeTags(ctx, q.IgnoreLimit)
	}),
	"tags.blocks": readQuery("tags.blocks", "LabelBlocksByKeyword", "Lease.NativeTagBlocks", func(ctx context.Context, l *workspace.Lease, q readKey) (any, error) {
		return l.NativeTagBlocks(ctx, q.Keyword)
	}),
	"tags.spans": readQuery("tags.spans", "GetTagSpans", "Lease.NativeTagSpans", func(ctx context.Context, l *workspace.Lease, q readSpans) (any, error) {
		return l.NativeTagSpans(ctx, q.Kind, q.Value, q.Limit)
	}),
	"bookmarks": readQuery("bookmarks", "BuildBookmarks", "Lease.NativeBookmarks", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) { return l.NativeBookmarks(ctx) }),
	"av.get": readQuery("av.get", "GetAttributeView", "Lease.ReadNativeAttributeView", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.ReadNativeAttributeView(ctx, q.ID)
	}),
	"av.search": readQuery("av.search", "SearchAttributeView", "Lease.SearchNativeAttributeViews", func(ctx context.Context, l *workspace.Lease, q struct {
		Keyword    string
		ExcludeIDs []string
	}) (any, error) {
		return l.SearchNativeAttributeViews(ctx, q.Keyword, q.ExcludeIDs)
	}),
	"av.name": readQuery("av.name", "GetAttributeViewName", "Lease.NativeAttributeViewName", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeAttributeViewName(ctx, q.ID)
	}),
	"av.keys": readQuery("av.keys", "GetAttributeViewKeys/GetAttributeViewRelationKeys/GetAttributeViewRollupKeys", "Lease.NativeAttributeViewKeys", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewKeys(ctx, q.ID, q.Kind, q.Query, q.IDs)
	}),
	"av.primary": readQuery("av.primary", "GetAttributeViewPrimaryKeyValues", "Lease.NativeAttributeViewPrimaryValues", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewPrimaryValues(ctx, q.ID, q.Query, q.Page, q.PageSize)
	}),
	"av.view": readQuery("av.view", "GetAttributeViewFilterSort", "Lease.NativeAttributeViewView", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewView(ctx, q.ID, q.BlockID)
	}),
	"av.render": readQuery("av.render", "RenderAttributeView", "Lease.NativeAttributeViewRender", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewRender(ctx, q.ID, q.ViewID, q.Query, q.Page, q.PageSize, q.Groups)
	}),
	"av.items": readQuery("av.items", "GetAttributeViewItemIDs", "Lease.NativeAttributeViewItemIDs", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewItemIDs(ctx, q.ID, q.IDs)
	}),
	"av.bound": readQuery("av.bound", "GetAttributeViewBoundBlockIDs", "Lease.NativeAttributeViewBoundBlockIDs", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewBoundBlockIDs(ctx, q.ID, q.IDs)
	}),
	"assets.names": readQuery("assets.names", "SearchAssetsByName", "Lease.NativeSearchAssetNames", func(ctx context.Context, l *workspace.Lease, q readAsset) (any, error) {
		return l.NativeSearchAssetNames(ctx, q.Keyword, q.Extensions)
	}),
	"assets.references": readQuery("assets.references", "UnusedAssets/MissingAssets", "Lease.NativeAssetReferences", func(ctx context.Context, l *workspace.Lease, q struct{ Missing bool }) (any, error) {
		return l.NativeAssetReferences(ctx, q.Missing)
	}),
	"assets.search": readQuery("assets.search", "SearchAssetContent", "Lease.SearchNativeAssetContents", func(ctx context.Context, l *workspace.Lease, q assetquery.Request) (any, error) {
		return l.SearchNativeAssetContents(ctx, q)
	}),
	"assets.content": readQuery("assets.content", "GetAssetContent", "Lease.NativeAssetContent", func(ctx context.Context, l *workspace.Lease, q readContent) (any, error) {
		return l.NativeAssetContent(ctx, q.ID, q.Query, q.Method)
	}),
	"configuration": readQuery("configuration", "GetMaskedConf/GetConf", "Lease.ConfigurationPresentation", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) {
		return l.ConfigurationPresentation(ctx, true, false)
	}),
	"configuration.notebooks": readQuery("configuration.notebooks", "GetBoxes", "Lease.ConfigurationNotebooks", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) {
		return l.ConfigurationNotebooks(ctx)
	}),
	"configuration.notebook": readQuery("configuration.notebook", "GetNotebookConf", "Lease.NativeNotebookConfiguration", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeNotebookConfiguration(ctx, q.ID)
	}),
	"configuration.emojis": readQuery("configuration.emojis", "GetCustomEmojis", "Lease.CustomEmojis", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) { return l.CustomEmojis(ctx) }),
	"index.rebuild":        readQuery("index.rebuild", "FullReindex/checkIndex (derived reads only)", "Lease.PrepareNativeReads", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) { return nil, nil }),
	"document.ast": readQuery("document.ast", "LoadTreeByPath/LoadTreeByBlockID read-only bytes", "Lease.ReadDocument/ReadBlock", func(ctx context.Context, l *workspace.Lease, q struct{ BoxID, DocID, BlockID string }) (any, error) {
		if q.BlockID != "" {
			return l.ReadBlock(ctx, q.BoxID, q.DocID, q.BlockID)
		}
		return l.ReadDocument(ctx, q.BoxID, q.DocID)
	}),
	"asset.get": readQuery("asset.get", "GetFile/asset.get", "Lease.NativeFileRead", func(ctx context.Context, l *workspace.Lease, q struct{ AssetName string }) (any, error) {
		return l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: "assets/" + q.AssetName})
	}),
	"asset.list": readQuery("asset.list", "ReadDir/asset.list", "Lease.NativeFileRead", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) {
		return l.NativeFileRead(ctx, workspacefile.Request{Action: "list", Path: "assets"})
	}),
	"references.query": readQuery("references.query", "Original SQL reference/count/definition/annotation/recent queries", "Lease.NativeReferenceQuery", func(ctx context.Context, l *workspace.Lease, q workspace.NativeReferenceQuery) (any, error) {
		return l.NativeReferenceQuery(ctx, q)
	}),
	"references.invalid": readQuery("references.invalid", "GetInvalidBlockRefs", "Lease.NativeInvalidReferences", func(ctx context.Context, l *workspace.Lease, q struct{ Page, PageSize int }) (any, error) {
		return l.NativeInvalidReferences(ctx, q.Page, q.PageSize)
	}),
	"av.images": readQuery("av.images", "GetCurrentAttributeViewImages", "Lease.NativeAttributeViewImages", func(ctx context.Context, l *workspace.Lease, q readAV) (any, error) {
		return l.NativeAttributeViewImages(ctx, q.ID, q.ViewID, q.Query)
	}),
	"av.mirrors": readQuery("av.mirrors", "GetMirrorDatabaseBlocks", "Lease.NativeAttributeViewMirrors", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeAttributeViewMirrors(ctx, q.ID)
	}),
	"block.avkeys": readQuery("block.avkeys", "GetBlockAttributeViewKeys read projection without repair", "Lease.NativeBlockAttributeViewKeys", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeBlockAttributeViewKeys(ctx, q.ID)
	}),
	"block.attributes.batch": readQuery("block.attributes.batch", "BatchGetBlockAttrs", "Lease.ReadNativeBlockAttributes", func(ctx context.Context, l *workspace.Lease, q readIDs) (any, error) {
		out := map[string]map[string]string{}
		for _, id := range q.IDs {
			attrs, e := l.ReadNativeBlockAttributes(ctx, id)
			if e != nil {
				return nil, e
			}
			out[id] = attrs
		}
		return out, nil
	}),
	"block.get": readQuery("block.get", "GetBlock", "Lease.NativeGetBlock", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeGetBlock(ctx, q.ID)
	}),
	"blocks.recent": readQuery("blocks.recent", "RecentUpdatedBlocks", "Lease.NativeRecentUpdatedBlocks", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) {
		return l.NativeRecentUpdatedBlocks(ctx)
	}),
	"notebook.info": readQuery("notebook.info", "GetNotebookInfo", "Lease.NativeNotebookInfo", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeNotebookInfo(ctx, q.ID)
	}),
	"storage": readQuery("storage", "GetRecentDocs/GetCriteria/GetLocalStorage/GetOutlineStorage (read only)", "Lease.NativeReadStorage", func(ctx context.Context, l *workspace.Lease, q readStorage) (any, error) {
		return l.NativeReadStorage(ctx, q.Action, q.Key, q.SortBy)
	}),
	"document.assets": readQuery("document.assets", "DocAssets/DocImageAssets", "Lease.NativeDocumentAssets", func(ctx context.Context, l *workspace.Lease, q struct {
		ID     string
		Images bool
	}) (any, error) {
		return l.NativeDocumentAssets(ctx, q.ID, q.Images)
	}),
	"document.treeids": readQuery("document.treeids", "DocumentTreeIDs/listDocTree", "Lease.NativeDocumentTreeIDs", func(ctx context.Context, l *workspace.Lease, q struct{ Box, Path string }) (any, error) {
		return l.NativeDocumentTreeIDs(ctx, q.Box, q.Path)
	}),
	"references.block": readQuery("references.block", "GetBlockRefs/GetRefIDs", "Lease.NativeBlockReferences", func(ctx context.Context, l *workspace.Lease, q readID) (any, error) {
		return l.NativeBlockReferences(ctx, q.ID)
	}),
	"bookmarks.labels": readQuery("bookmarks.labels", "BookmarkLabels", "Lease.NativeBookmarkLabels", func(ctx context.Context, l *workspace.Lease, q readEmpty) (any, error) {
		return l.NativeBookmarkLabels(ctx)
	}),
	"file.read": readQuery("file.read", "GetFile (page data only)", "Lease.NativeFileRead", func(ctx context.Context, l *workspace.Lease, q struct{ Path string }) (any, error) {
		return l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: q.Path})
	}),
	"file.list": readQuery("file.list", "ReadDir (page data only)", "Lease.NativeFileRead", func(ctx context.Context, l *workspace.Lease, q struct{ Path string }) (any, error) {
		return l.NativeFileRead(ctx, workspacefile.Request{Action: "list", Path: q.Path})
	}),
	"templates.search": readQuery("templates.search", "SearchTemplate", "Lease.NativeSearchTemplates", func(ctx context.Context, l *workspace.Lease, q readKey) (any, error) {
		return l.NativeSearchTemplates(ctx, q.Keyword)
	}),
	"widgets.search": readQuery("widgets.search", "SearchWidget", "Lease.NativeSearchWidgets", func(ctx context.Context, l *workspace.Lease, q readKey) (any, error) {
		return l.NativeSearchWidgets(ctx, q.Keyword)
	}),
}

type readStorage struct{ Action, Key, SortBy string }

func (q readStorage) ValidateQuery() error {
	switch q.Action {
	case "recent.get", "criteria.get", "local.get", "outline.get":
	default:
		return ErrArgument
	}
	if q.Key != "" && q.Action != "outline.get" || q.SortBy != "" && q.Action != "recent.get" {
		return ErrArgument
	}
	return nil
}

func queryNeedsIndex(action string) bool {
	if strings.HasPrefix(action, "configuration") {
		return false
	}
	switch action {
	case "templates.search", "widgets.search", "file.read", "file.list", "document.treeids", "document.ast", "asset.get", "asset.list", "assets.names", "av.get", "av.name", "av.keys", "av.items", "av.bound":
		return false
	}
	return true
}
