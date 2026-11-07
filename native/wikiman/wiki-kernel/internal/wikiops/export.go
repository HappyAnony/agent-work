package wikiops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io/fs"
	"net/url"
	"path"
	"strings"

	nethtml "golang.org/x/net/html"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var ErrExportLimit = errors.New("export_size_limit")

// Called ONLY within Content's read admission: the document and every asset
// belong to one publication snapshot. No HTTP/renderer global state is used.
func exportDocument(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest, source string, raw []byte) (string, error) {
	mode := "html"
	if r.Format == "docx" {
		mode = "docx"
	} else if r.Format == "htmlmd" {
		mode = "htmlmd"
	}
	body, err := renderDocumentProjection(ctx, raw, r.DocID, mode)
	if err != nil {
		return "", err
	}
	title := r.DocID
	var metadata struct{ Properties map[string]json.RawMessage }
	if json.Unmarshal(raw, &metadata) == nil {
		var name string
		if json.Unmarshal(metadata.Properties["title"], &name) == nil && name != "" && len(name) <= 4096 {
			title = name
		}
	}
	return staticExport(ctx, body, title, func(name string) ([]byte, error) {
		return readOwnedDocumentAsset(ctx, m, r.StoreID, r.PageID, r.BoxID, source, name)
	})
}

// DocumentAsset binds the resource lookup to one existing document and the
// same source admission used for reading its page and notebook attachments.
func DocumentAsset(ctx context.Context, m *workspace.WorkspaceManager, store, page, box, doc, name string, entryOnly bool) (out ContentResult, err error) {
	if !pageIdentity.MatchString(page) || !nativeID.MatchString(box) || !nativeID.MatchString(doc) || !validAsset(name) {
		return out, ErrArgument
	}
	err = m.WithReadContext(ctx, store, func(c context.Context) error {
		tree, e := m.Tree(c, store)
		if e != nil {
			return e
		}
		record, ok := tree.Get(page)
		if !ok || record.DeletedAt != nil {
			return fs.ErrNotExist
		}
		if entryOnly && (record.EntryDocument == nil || record.EntryDocument.BoxID != box || record.EntryDocument.DocID != doc) {
			return ErrStaleSession
		}
		source, _, e := m.DocumentSource(c, store, page, box, doc)
		if e != nil {
			return e
		}
		data, e := readOwnedDocumentAsset(c, m, store, page, box, source, name)
		if e != nil {
			return e
		}
		if len(data) > 8<<20 {
			return ErrExportLimit
		}
		out = ContentResult{StoreID: store, PageID: page, Data: data, Hash: sourcetx.Hash(data)}
		return nil
	})
	return out, err
}

func readOwnedDocumentAsset(ctx context.Context, m *workspace.WorkspaceManager, store, page, box, source, name string) ([]byte, error) {
	prefix := "pages/" + page + "/data/"
	candidates := []string{prefix + "assets/" + name, strings.TrimSuffix(source, ".docj") + "/assets/" + name, path.Dir(source) + "/assets/" + name, prefix + box + "/assets/" + name}
	for _, candidate := range candidates {
		data, err := m.SourceFile(ctx, store, candidate)
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			return data, err
		}
	}
	return nil, fs.ErrNotExist
}

// No uploaded HTML/SVG, remote embed, script, event attribute, CSS URL or
// arbitrary protocol may become active in the exported file. Attachments are
// opaque downloads; only a fixed raster/audio/video set can be previewed.
func staticExport(ctx context.Context, body, title string, read func(string) ([]byte, error)) (string, error) {
	if len(body) > 4<<20 {
		return "", ErrExportLimit
	}
	root, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		return "", err
	}
	allowed := strings.Fields("div span p h1 h2 h3 h4 h5 h6 pre code blockquote ul ol li table thead tbody tfoot tr th td hr br strong b em i u s del mark sub sup kbd a img audio video source details summary caption colgroup col")
	tags := map[string]bool{}
	for _, tag := range allowed {
		tags[tag] = true
	}
	drop := map[string]bool{}
	for _, tag := range strings.Fields("script style iframe object embed link meta base form input button textarea select svg math template") {
		drop[tag] = true
	}
	types := map[string]string{"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp", "avif": "image/avif", "mp3": "audio/mpeg", "ogg": "audio/ogg", "wav": "audio/wav", "mp4": "video/mp4", "webm": "video/webm"}
	cache := map[string][]byte{}
	total := 0
	encodedTotal := 0
	asset := func(value string, preview bool) (string, error) {
		name := ""
		switch {
		case strings.HasPrefix(value, "assets/"):
			name = value[7:]
		case strings.HasPrefix(value, "/assets/"):
			name = value[8:]
		default:
			return "", nil
		}
		decoded, e := url.PathUnescape(name)
		if e != nil || !validAsset(decoded) {
			return "", ErrArgument
		}
		data, ok := cache[decoded]
		if !ok {
			if len(cache) >= 128 {
				return "", ErrExportLimit
			}
			data, e = read(decoded)
			if e != nil {
				return "", e
			}
			total += len(data)
			if len(data) > 8<<20 || total > 8<<20 {
				return "", ErrExportLimit
			}
			cache[decoded] = data
		}
		mime := "application/octet-stream"
		if preview {
			mime = types[strings.TrimPrefix(strings.ToLower(path.Ext(decoded)), ".")]
			if mime == "" {
				return "", nil
			}
		}
		encodedTotal += base64.StdEncoding.EncodedLen(len(data)) + len(mime) + 14
		if encodedTotal > 12<<20 {
			return "", ErrExportLimit
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	var clean func(*nethtml.Node) error
	clean = func(parent *nethtml.Node) error {
		for node := parent.FirstChild; node != nil; {
			next := node.NextSibling
			if err := ctx.Err(); err != nil {
				return err
			}
			if node.Type == nethtml.CommentNode || node.Type == nethtml.ElementNode && drop[node.Data] {
				parent.RemoveChild(node)
				node = next
				continue
			}
			if err := clean(node); err != nil {
				return err
			}
			if node.Type == nethtml.ElementNode {
				if !tags[node.Data] {
					for node.FirstChild != nil {
						child := node.FirstChild
						node.RemoveChild(child)
						parent.InsertBefore(child, node)
					}
					parent.RemoveChild(node)
					node = next
					continue
				}
				attrs := []nethtml.Attribute{}
				link := ""
				for _, attr := range node.Attr {
					if attr.Namespace != "" {
						continue
					}
					switch attr.Key {
					case "href", "data-href":
						if node.Data == "a" || node.Data == "span" {
							link = attr.Val
						}
					case "src", "poster":
						if node.Data == "img" || node.Data == "audio" || node.Data == "video" || node.Data == "source" {
							value, e := asset(attr.Val, true)
							if e != nil {
								return e
							}
							if value != "" {
								attrs = append(attrs, nethtml.Attribute{Key: attr.Key, Val: value})
							}
						}
					case "alt", "title", "colspan", "rowspan", "data-type":
						attrs = append(attrs, attr)
					}
				}
				if link != "" {
					value, e := asset(link, false)
					if e != nil {
						return e
					}
					if value != "" {
						node.Data = "a"
						attrs = append(attrs, nethtml.Attribute{Key: "href", Val: value}, nethtml.Attribute{Key: "download", Val: path.Base(link)})
					} else if target, e := url.Parse(link); e == nil && (target.Scheme == "http" || target.Scheme == "https") && target.Host != "" {
						node.Data = "a"
						attrs = append(attrs, nethtml.Attribute{Key: "href", Val: link}, nethtml.Attribute{Key: "rel", Val: "noopener noreferrer"})
					}
				}
				if node.Data == "audio" || node.Data == "video" {
					attrs = append(attrs, nethtml.Attribute{Key: "controls"}, nethtml.Attribute{Key: "preload", Val: "none"})
				}
				node.Attr = attrs
			}
			node = next
		}
		return nil
	}
	if err = clean(root); err != nil {
		return "", err
	}
	var output strings.Builder
	// Export is deliberately self-contained and inert. External links require a
	// user click; remote subresources and scripts remain denied when opened.
	output.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; media-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>` + html.EscapeString(title) + `</title><style>body{max-width:960px;margin:2rem auto;padding:0 1rem;font:16px/1.7 system-ui;overflow-wrap:anywhere}img,video{max-width:100%}pre{white-space:pre-wrap;background:#f4f4f4;padding:1rem}table{border-collapse:collapse}td,th{border:1px solid #bbb;padding:.4rem}[data-type=NodeHeading]{font-size:1.5em;font-weight:600}@media print{body{max-width:none;margin:0}pre{break-inside:avoid}}</style></head><body>`)
	if err = nethtml.Render(&output, root); err != nil {
		return "", err
	}
	output.WriteString("</body></html>")
	value := output.String()
	if len(value) > 16<<20 {
		return "", ErrExportLimit
	}
	encoded, _ := json.Marshal(value)
	if len(encoded) > 24<<20 {
		return "", ErrExportLimit
	}
	return value, nil
}
