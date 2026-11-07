//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/internal/blockview"
)

func TestNativeBacklinksActualOwnersSourceAndNoPublication(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		for _, mode := range []string{"links", "links2", "document", "mention-document"} {
			out, e := l.NativeBacklinks(ctx, blockview.BacklinkRequest{Mode: mode, ID: blockID, RefTreeID: docID, BeforeLen: 12, Highlight: true})
			if e != nil {
				t.Fatal(l.PageID(), mode, e)
			}
			if mode == "links" || mode == "links2" {
				if out.ReferenceCount != 1 || len(out.Links) != 1 || out.Box != boxID {
					t.Fatal(l.PageID(), mode, out)
				}
			}
			if mode == "document" {
				if len(out.Documents) != 1 || !strings.Contains(out.Documents[0].DOM, positionRef) {
					t.Fatal(l.PageID(), mode, out)
				}
			}
		}
		mentions, e := l.NativeBacklinks(ctx, blockview.BacklinkRequest{Mode: "mention-document", ID: getDocDefinition, RefTreeID: docID, BeforeLen: 12, Highlight: true})
		if e != nil || len(mentions.Documents) != 1 || len(mentions.Keywords) == 0 || !strings.Contains(mentions.Documents[0].DOM, "search-mark") {
			t.Fatal("actual mention", l.PageID(), mentions, e)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := l.NativeBacklinks(canceled, blockview.BacklinkRequest{Mode: "links", ID: blockID}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("backlink read published source")
	}
}
